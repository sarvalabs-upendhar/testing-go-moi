package e2e

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common"

	bgCommon "github.com/sarvalabs/battleground/common"
	"github.com/sarvalabs/battleground/server/types"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	bg "github.com/sarvalabs/battleground"
	client "github.com/sarvalabs/battleground/client/types"
	"github.com/sarvalabs/battleground/server/warzone/infrastructure"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

type ClusterType byte

const (
	Unknown ClusterType = iota
	StandAlone
	PreExisting

	InitialSyncTime         = 120 * time.Second
	DefaultQueryTime        = 10 * time.Second
	DefaultNodeStartTime    = 30 * time.Second
	DefaultNodeStopTime     = 30 * time.Second
	DefaultBGStartTime      = 25 * time.Minute
	DefaultShutdownTimeout  = 10 * time.Minute
	DefaultConfirmIxTimeout = 1 * time.Minute
	DefaultAccountCount     = 2 // 2 accounts are enough to fire ixns in debug mode
	InitialKMOITokens       = 25000
	DefaultJSONRPCPort      = 29000
)

var (
	bgURL = "http://85.239.245.54:7000/api"

	DefaultFuelPrice = big.NewInt(1)
	DefaultFuelLimit = uint64(10000)
	local            = "local"
	cloud            = "cloud"
	networkType      = flag.String("network", local, "enter the network type to use local or cloud")
	commitHash       = flag.String("commit-hash", "", "enter the commit hash of the repo to be deployed")
	logLevel         = flag.String("log-level", "TRACE", "enter the log level")
)

type BattleGroundConfig struct {
	clusterType ClusterType
	logLevel    string
	rpcEndPoint string
}

func newBattleGroundConfig(
	clusterType ClusterType,
	logLevel string,
	rpcEndPoint string,
) BattleGroundConfig {
	return BattleGroundConfig{
		clusterType: clusterType,
		logLevel:    logLevel,
		rpcEndPoint: rpcEndPoint,
	}
}

type TestEnvironment struct {
	suite.Suite
	bgConfig       BattleGroundConfig
	bgClient       bg.Client
	jsonRPCUrls    []string
	validatorCount int
	moiClient      *moiclient.Client
	moiClients     []*moiclient.Client
	accounts       []tests.AccountWithMnemonic
	instances      []common.Instance
	logger         hclog.Logger
	suiteSetupDone bool
}

func (te *TestEnvironment) chooseRandomAccount() tests.AccountWithMnemonic {
	randomNum, err := rand.Int(rand.Reader, big.NewInt(int64(len(te.accounts))))
	te.Suite.NoError(err)

	return te.accounts[int(randomNum.Int64())]
}

func (te *TestEnvironment) chooseRandomUniqueAccounts(count int) ([]tests.AccountWithMnemonic, error) {
	if count > len(te.accounts) {
		return nil, errors.New("insufficient accounts")
	}

	remainingAccs := make([]tests.AccountWithMnemonic, len(te.accounts))
	copy(remainingAccs, te.accounts)

	chosenAccounts := make([]tests.AccountWithMnemonic, 0, count)

	for len(chosenAccounts) < count {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(remainingAccs))))
		te.Suite.NoError(err)

		chosenAccounts = append(chosenAccounts, remainingAccs[randomIndex.Int64()])
		remainingAccs = append(remainingAccs[:randomIndex.Int64()], remainingAccs[randomIndex.Int64()+1:]...)
	}

	return chosenAccounts, nil
}

func (te *TestEnvironment) configureBattleGround() error {
	te.bgConfig = newBattleGroundConfig(StandAlone, "TRACE", bgURL)

	// initialize bg client
	if *networkType == cloud {
		bgConfig := types.DefaultCloudConfig()
		bgConfig.MoichainSourceRef = *commitHash
		// TODO optimize the battleground to generate 15 accounts in genesis, instead of creating accounts through ixns
		bgConfig.NoOfInstances = 15 // 150 moipods are required to execute these tests

		te.bgClient = bg.NewBGClient(&client.Config{
			CloudCfg:    bgConfig,
			Network:     client.Cloud,
			EndPoint:    te.bgConfig.rpcEndPoint,
			DialTimeout: 10 * time.Second,
		})

		te.validatorCount = bgConfig.NoOfInstances * bgConfig.NoOfPodsPerInstance
	} else {
		d := client.DefaultClusterConfig()
		d.WithLogs = true
		d.WithStdout = false
		d.LogLevel = "TRACE"
		d.BootNodePort = 27000
		d.Libp2pPort = 28000
		d.JsonRPCPort = DefaultJSONRPCPort
		d.GuardianPath = "../../moiclient/"

		te.bgClient = bg.NewBGClient(&client.Config{
			ClusterConfig: d,
			Network:       client.Local,
		})

		te.validatorCount = d.ValidatorCount
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := te.bgClient.ServerStatus(ctx); err != nil {
		return err
	}

	return nil
}

func getMoiClient(t *testing.T, url string) (*moiclient.Client, error) {
	t.Helper()

	client, err := moiclient.NewClient(url)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	// check if node is up
	_, err = client.Inspect(ctx, &rpcargs.InspectArgs{})
	if err != nil {
		cancel()

		return nil, err
	}

	cancel()

	return client, nil
}

func getMoiClients(t *testing.T, urls []string) []*moiclient.Client {
	t.Helper()

	clients := make([]*moiclient.Client, 0)

	for _, url := range urls {
		client, err := getMoiClient(t, url)
		if err != nil {
			t.Log("unable to create moi client for ", url, "err", err)

			continue
		}

		clients = append(clients, client)
	}

	return clients
}

func (te *TestEnvironment) initializeMOIClient() {
	clients := getMoiClients(te.T(), te.jsonRPCUrls)
	if len(clients) == 0 {
		te.FailNow("unable to initialize moi clients")
	}

	te.moiClients = clients

	if *networkType == local {
		// use operator for firing ixns and querying until observer node wait time issue is fixed
		// https://github.com/sarvalabs/go-moi/issues/678
		c, err := getMoiClient(te.T(), fmt.Sprintf("http://localhost:%d", DefaultJSONRPCPort))
		if err != nil {
			te.FailNow("unable to initialize moi clients")
		}

		te.logger.Debug("JSON RPC url used is ", "url", c.URL())

		te.moiClient = c

		return
	}

	te.moiClient = clients[0]
	te.logger.Debug("JSON RPC url used is ", "url", te.moiClient.URL())
}

func (te *TestEnvironment) runCriticallyNecessaryTearDown() {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	err := te.bgClient.DestroyNetwork(ctx, true)
	te.Suite.NoError(err)
}

func (te *TestEnvironment) initLogger() {
	te.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString(*logLevel),
	})
}

// fetch json urls and check if initial sync is done in all nodes
func (te *TestEnvironment) getUrlsAndCheckInitialSync() {
	te.jsonRPCUrls = moiclient.GetJSONRPCUrls(te.Suite.T(), te.bgClient, te.validatorCount)

	te.logger.Debug("e2e json urls ", "network", *networkType, "urls", te.jsonRPCUrls)

	moiclient.CheckIfNodesInitialSyncDone(te.Suite.T(), te.validatorCount, te.jsonRPCUrls)
}

func (te *TestEnvironment) SetupSuite() {
	defer func() {
		// make sure to delete directories in case of setup suite failure
		// make sure to destroy battleground if setup suite fails
		if !te.suiteSetupDone {
			te.logger.Error("Setup suite failed")
			te.runCriticallyNecessaryTearDown()
		}
	}()

	te.initLogger()

	var registeredAcc []bgCommon.AccountWithMnemonic

	err := te.configureBattleGround()
	te.Suite.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)
	bgStatus, err := te.bgClient.NetworkStatus(ctx)

	cancel()
	te.Suite.NoError(err)

	switch bgStatus {
	case infrastructure.Inactive: // if battleground is already running, then don't start it
		te.logger.Debug("starting battle ground")

		ctx, cancel = context.WithTimeout(context.Background(), DefaultBGStartTime)
		_, err = te.bgClient.StartNetwork(ctx)

		cancel()
		te.Suite.NoError(err)

	case infrastructure.Active:
		te.logger.Debug("battle ground is already running")

	case infrastructure.Pending:
		te.Suite.FailNow("wait for some time battle ground is getting provisioned")
	case infrastructure.Failed:
		te.Suite.FailNow("battle ground status is Failed")
	default:
		te.Suite.FailNow("unknown network status", bgStatus)
	}

	// if network type is cloud then update nodes with given commit hash
	if *networkType == cloud {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)
		bgStatus, err := te.bgClient.NetworkStatus(ctx)

		cancel()
		te.Suite.NoError(err)

		if bgStatus != infrastructure.Active {
			te.logger.Error("battle ground is not active even after 10 minute wait time")
		}

		te.logger.Debug("updating battle ground")

		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Minute)

		err = te.bgClient.UpdateNetwork(ctx, *commitHash)

		cancel()
		te.Suite.NoError(err)
	}

	te.getUrlsAndCheckInitialSync()

	if *networkType == cloud {
		time.Sleep(30 * time.Second)
	}

	ctx, cancel = context.WithTimeout(context.Background(), DefaultQueryTime)
	registeredAcc, err = te.bgClient.Accounts(ctx)

	cancel()
	te.Suite.NoError(err)

	// make sure at least one account is registered on chain to fire interactions
	require.GreaterOrEqual(te.T(), len(registeredAcc), 1)

	// choose url that works
	te.initializeMOIClient()

	if *networkType == local {
		te.instances, err = common.ReadInstancesFile("./tmp/instances.json")
		te.Suite.NoError(err)
	}

	// Generate accounts and register them on chain
	te.accounts, err = cmdcommon.GetAccountsWithMnemonic(DefaultAccountCount)
	te.Suite.NoError(err)

	te.logger.Debug("registering accounts on chain", te.accounts)

	KMOIAssetID := identifiers.AssetID("000000004cd973c4eb83cdb8870c0de209736270491b7acc99873da1eddced5826c3b548")

	for _, account := range te.accounts {
		te.logger.Debug("sending Fuel token ", "KMOI ", InitialKMOITokens)
		transferAsset(te, tests.AccountWithMnemonic(registeredAcc[0]), account.Addr, map[identifiers.AssetID]*big.Int{
			KMOIAssetID: big.NewInt(InitialKMOITokens),
		})
	}

	te.suiteSetupDone = true
}

func (te *TestEnvironment) TearDownSuite() {
	te.logger.Debug("tear down suite called")

	te.runCriticallyNecessaryTearDown()
}

func validateFlags() error {
	if *networkType != cloud && *networkType != local {
		return errors.New("network type should be either local or cloud")
	}

	if *networkType == cloud && *commitHash == "" {
		return errors.New("commit hash cannot be empty")
	}

	return nil
}

func TestInteractions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test")
	}

	t.Log("network type  ", *networkType)

	if err := validateFlags(); err != nil {
		t.Logf("can not run E2E tests %s", err.Error())

		return
	}

	// make sure bg token is set if network type is cloud
	if *networkType == cloud {
		bgToken := os.Getenv("BG_TOKEN")

		if bgToken == "" {
			t.Log("can not run E2E tests")
			t.Log("set BG_TOKEN environment variable")

			return
		}
	}

	suite.Run(t, new(TestEnvironment))
}
