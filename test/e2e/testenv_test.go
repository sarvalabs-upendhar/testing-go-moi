package e2e

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/battleground/server/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	bg "github.com/sarvalabs/battleground"
	client "github.com/sarvalabs/battleground/client/types"
	"github.com/sarvalabs/battleground/server/warzone/infrastructure"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
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
	DefaultBGStartTime      = 10 * time.Minute
	DefaultShutdownTimeout  = 10 * time.Minute
	DefaultConfirmIxTimeout = 1 * time.Minute
	DefaultAccountCount     = 2
	InitialKMOITokens       = 25000
	DefaultValidatorCount   = 100
)

var (
	bgURL = "http://85.239.245.54:7000/api"

	DefaulFuelPrice  = big.NewInt(1)
	DefaultFuelLimit = uint64(10000)
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
	bgConfig    BattleGroundConfig
	bgClient    bg.Client
	jsonRPCUrls []string
	moiClient   *moiclient.Client
	moiClients  []*moiclient.Client
	accounts    []tests.AccountWithMnemonic
	logger      hclog.Logger
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

	bgConfig := types.DefaultCloudConfig()

	// initialize bg client
	te.bgClient = bg.NewBGClient(&client.Config{
		CloudCfg:    bgConfig,
		Network:     client.Cloud,
		EndPoint:    te.bgConfig.rpcEndPoint,
		DialTimeout: 10 * time.Second,
	})

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

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

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
			continue
		}

		clients = append(clients, client)
	}

	return clients
}

func (te *TestEnvironment) initializeMOIClient() {
	clients := getMoiClients(te.T(), te.jsonRPCUrls)
	if len(clients) == 0 {
		commonError("unable to initialize moi client")
	}

	te.logger.Info("JSON RPC url used is ", "url", clients[0].URL())

	te.moiClient = clients[0]
	te.moiClients = clients
}

func (te *TestEnvironment) SetupSuite() {
	te.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString("DEBUG"),
	})

	var registeredAcc []tests.AccountWithMnemonic

	err := te.configureBattleGround()
	te.Suite.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	bgStatus, err := te.bgClient.NetworkStatus(ctx)

	cancel()
	te.Suite.NoError(err)

	switch bgStatus {
	case infrastructure.Inactive: // if battleground is already running, then don't start it
		te.logger.Info("starting battle ground")

		ctx, cancel = context.WithTimeout(context.Background(), DefaultBGStartTime)
		_, err = te.bgClient.StartNetwork(ctx)

		cancel()
		te.Suite.NoError(err)

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		registeredAcc, err = te.bgClient.Accounts(ctx)

		cancel()
		te.Suite.NoError(err)
	case infrastructure.Active:
		te.logger.Info("battle ground is already running")

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		registeredAcc, err = te.bgClient.Accounts(ctx)

		cancel()
		te.Suite.NoError(err)
	case infrastructure.Pending:
		te.Suite.FailNow("wait for some time battle ground is getting provisioned")
	case infrastructure.Failed:
		te.Suite.FailNow("battle ground status is Failed")
	}

	require.GreaterOrEqual(te.T(), len(registeredAcc), 1)

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	te.jsonRPCUrls, err = te.bgClient.JsonRpcUrls(ctx)
	te.Suite.NoError(err)

	cancel()
	te.Suite.NotEqual(len(te.jsonRPCUrls), 0)

	te.logger.Info("json urls ", "urls", te.jsonRPCUrls)
	// choose url that works
	te.initializeMOIClient()

	// Generate accounts and register them on chain
	te.accounts, err = cmdcommon.GetAccountsWithMnemonic(DefaultAccountCount)
	te.Suite.NoError(err)

	te.logger.Info("registering accounts on chain", te.accounts)

	KMOIAssetID := common.AssetID("000000004cd973c4eb83cdb8870c0de209736270491b7acc99873da1eddced5826c3b548")

	for _, account := range te.accounts {
		te.logger.Info("sending Fuel token ", "KMOI ", InitialKMOITokens)
		transferAsset(te, registeredAcc[0], account.Addr, map[common.AssetID]*big.Int{
			KMOIAssetID: big.NewInt(InitialKMOITokens),
		})
	}
}

func (te *TestEnvironment) TearDownSuite() {
	te.logger.Info("tear down suite called")

	ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	err := te.bgClient.DestroyNetwork(ctx, false)
	te.Suite.NoError(err)
}

func TestInteractions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test")
	}

	bgToken := os.Getenv("BG_TOKEN")
	if bgToken == "" {
		fmt.Println("can not run E2E tests")
		fmt.Println("set BG_TOKEN environment variable")

		return
	}

	suite.Run(t, new(TestEnvironment))
}
