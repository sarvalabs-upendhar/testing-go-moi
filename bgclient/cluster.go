package bgclient

import "C"
import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"

	bgcommon "github.com/sarvalabs/battleground/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

const (
	assetArtifactFile = "./../common/artifacts/assetartifacts.json"
	accountsFile      = "accounts.json"
	instancesFile     = "instances.json"
	artifactFile      = "artifact.json"
	genesisFile       = "genesis.json"
	bootnodeFileKey   = "file.key"

	flipperManifest = "./../compute/exlogics/flipper/flipper.yaml"
)

var (
	FlipperLogicID = common.CreateLogicIDFromString(
		"flipper-logic",
		0,
		identifiers.Systemic,
		identifiers.LogicIntrinsic,
		identifiers.LogicExtrinsic,
	)
	FlipperAccountID = FlipperLogicID.AsIdentifier()
)

var startTime int64

type Instance struct {
	KramaID      string `json:"krama_id"`
	RPCUrl       string `json:"rpc_url"`
	ConsensusKey string `json:"consensus_key"`
}

func readInstancesFile(path string) ([]Instance, error) {
	instances := make([]Instance, 0)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading instances file")
	}

	if err = json.Unmarshal(file, &instances); err != nil {
		return nil, errors.New("error reading instances file")
	}

	return instances, nil
}

type Cluster struct {
	Config        *ClusterConfig
	serverConfigs []*ServerConfig
	BootNode      *Node
	Servers       map[string]*Server

	once         sync.Once
	accounts     []tests.AccountWithMnemonic
	failCh       chan struct{}
	executionErr error
}

func removeDirectory(dirPath string) error {
	// Check if the directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// Directory does not exist, no need to remove
		return nil
	}

	// Remove the directory and all its contents
	err := os.RemoveAll(dirPath)
	if err != nil {
		return fmt.Errorf("failed to remove directory %s: %w", dirPath, err)
	}

	return nil
}

func NewTestCluster(clusterConfig *ClusterConfig, serverConfigs []*ServerConfig) (*Cluster, error) {
	cluster := &Cluster{
		Config:        clusterConfig,
		serverConfigs: serverConfigs,
		Servers:       make(map[string]*Server),
		failCh:        make(chan struct{}),
		once:          sync.Once{},
	}

	if !clusterConfig.OldState {
		if err := removeDirectory(clusterConfig.TempDir); err != nil {
			return nil, err
		}
	}

	if err := bgcommon.CreateDirSafe(clusterConfig.TempDir, os.ModePerm); err != nil {
		return nil, err
	}

	if err := cluster.startBootNode(); err != nil {
		return nil, errors.Wrap(err, "failed to start bootnode")
	}

	if !clusterConfig.OldState {
		if err := cluster.generateDataDir(); err != nil {
			return nil, errors.Wrap(err, "failed to generate data directories")
		}

		if err := cluster.generateAccounts(); err != nil {
			return nil, errors.Wrap(err, "failed to generate accounts file")
		}

		if err := cluster.generateArtifact(); err != nil {
			return nil, errors.Wrap(err, "failed to generate artifact file")
		}

		if err := cluster.generateGenesis(); err != nil {
			return nil, errors.Wrap(err, "failed to generate genesis file")
		}

		if err := cluster.updateGenesisWithAssets(); err != nil {
			return nil, errors.Wrap(err, "failed to update genesis with assets")
		}
	}

	for i := 0; i < clusterConfig.ValidatorCount; i++ {
		cluster.initTestServer(i)
	}

	return cluster, nil
}

func (c *Cluster) generateArtifact() error {
	type artifact struct {
		Name     string `json:"name"`
		Callsite string `json:"callsite"`
		Calldata string `json:"calldata"`
		Manifest string `json:"manifest"`
	}

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngine(pisa.NewEngine())

	// Read manifest file
	manifest, err := engineio.NewManifestFromFile(c.Config.FlipperPathDir(flipperManifest))
	if err != nil {
		return err
	}

	encodedManifest, err := manifest.Encode(common.POLO)
	if err != nil {
		return err
	}

	inputs := struct {
		Initial bool `polo:"initial"`
	}{
		Initial: false,
	}

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(inputs, polo.DocStructs())
	if err != nil {
		return err
	}

	a := artifact{
		Name:     "flipper-logic",
		Callsite: "Seed",
		Calldata: "0x" + hex.EncodeToString(calldata.Bytes()),
		Manifest: "0x" + hex.EncodeToString(encodedManifest),
	}

	file, err := json.MarshalIndent(a, "", "\t")
	if err != nil {
		return err
	}

	err = os.WriteFile(c.Config.Dir(artifactFile), file, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func (c *Cluster) Accounts() []tests.AccountWithMnemonic {
	return c.accounts
}

func (c *Cluster) startBootNode() error {
	fileKey := c.Config.Dir(bootnodeFileKey)

	var (
		ip      string
		id      peer.ID
		privKey crypto.PrivKey
		err     error
	)

	if !c.Config.OldState {
		if privKey, err = generateAndStoreNewKey(fileKey); err != nil {
			return err
		}
	} else {
		if err := c.readAccounts(); err != nil {
			return err
		}

		data, err := os.ReadFile(fileKey)
		if err != nil {
			return err
		}

		if privKey, err = crypto.UnmarshalPrivateKey(data); err != nil {
			return err
		}
	}

	if id, err = peer.IDFromPublicKey(privKey.GetPublic()); err != nil {
		return err
	}

	if ip, err = getBindedIP(); err != nil {
		return err
	}

	// Build arguments
	args := []string{
		"bootnode",
		// add port
		"--port", fmt.Sprintf("%d", c.Config.BootNodePort),
		"--key-path", fileKey,
		"--ipv4-address", ip,
	}

	// start the bootNode
	stdout := c.Config.GetStdout("BootNode-" + uuid.New().String())

	n, err := NewNode(c.Config.MOIpodBinary, args, stdout)
	if err != nil {
		return err
	}

	go func(node *Node) {
		<-node.Wait()

		if !node.ExitResult().Signaled {
			c.Fail(fmt.Errorf("bootnode at dir  has stopped unexpectedly"))
		}
	}(n)

	// read the bootstrapID and store it
	c.Config.BootstrapID = fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip, c.Config.BootNodePort, id.String())
	c.BootNode = n

	return err
}

func (c *Cluster) generateDataDir() error {
	args := []string{
		"init",
		// add data dir
		"--dir-count", fmt.Sprintf("%d", c.Config.ValidatorCount),
		"--libp2pPort", fmt.Sprintf("%d", c.Config.Libp2pPort),
		"--jsonrpcPort", fmt.Sprintf("%d", c.Config.JSONRPCPort),
		"--bootnode", c.Config.BootstrapID,
		"--instances-path", c.Config.Dir(instancesFile),
		"--directory-path", c.Config.TempDir,
		fmt.Sprintf("--shouldExecute=%v", c.Config.ShouldExecute),
		fmt.Sprintf("--enable-sortition=%v", c.Config.EnableSortition),
	}

	return runCommand(c.Config.McutilsBinary, args, c.Config.GetStdout("init"))
}

func (c *Cluster) readAccounts() error {
	accounts := make([]tests.AccountWithMnemonic, 0)

	file, err := os.ReadFile(c.Config.Dir(accountsFile))
	if err != nil {
		return err
	}

	err = json.Unmarshal(file, &accounts)
	if err != nil {
		return err
	}

	c.accounts = accounts

	return nil
}

func (c *Cluster) generateAccounts() error {
	args := []string{
		"account",
		// add account count
		"--count", fmt.Sprintf("%d", c.Config.GenesisAccountCount),
		"--accounts-path", c.Config.Dir(accountsFile),
	}

	if err := runCommand(c.Config.McutilsBinary, args, c.Config.GetStdout("account")); err != nil {
		return err
	}

	if err := c.readAccounts(); err != nil {
		return err
	}

	return nil
}

func (c *Cluster) generateGenesis() error {
	args := []string{
		"genesis",
		// add account count
		"--premine-amount", fmt.Sprintf("%d", c.Config.PremineAmount),
		"--accounts-path", c.Config.Dir(accountsFile),
		"--instances-path", c.Config.Dir(instancesFile),
		"--artifact-path", c.Config.Dir(artifactFile),
		"--genesis-path", c.Config.Dir(genesisFile),
		"--assetartifacts-path", c.Config.AssetArtifactPath(assetArtifactFile),
	}

	return runCommand(c.Config.McutilsBinary, args, c.Config.GetStdout("genesis"))
}

func (c *Cluster) updateGenesisWithAsset(assetInfo string, alloc string) error {
	fmt.Println("Updating genesis with asset info:", assetInfo)
	args := []string{
		"genesis",
		"premine",
		// add account count
		"--asset-info", assetInfo,
		"--allocations", alloc,
		"--genesis-path", c.Config.Dir(genesisFile),
		"--instances-path", c.Config.Dir(instancesFile),
	}

	return runCommand(c.Config.MOIpodBinary, args, os.Stdout)
}

// updateGenesisWithAssets generate random assets of given count and appends assets to genesis file
func (c *Cluster) updateGenesisWithAssets() error {
	for i := 0; i < c.Config.GenesisAssetCount; i++ {
		asset, err := getRandomUpperCaseString(5)
		if err != nil {
			return errors.Wrap(err, "failed to generate random upper case string")
		}

		accounts, err := tests.GetAccountsFromFile(c.Config.Dir(accountsFile))
		if err != nil {
			return errors.Wrap(err, "failed to read accounts file")
		}
		// <symbol:dimension:standard:decimals:maxsupply:manager>
		err = c.updateGenesisWithAsset(
			asset+":0:0:0:"+strconv.Itoa(3*c.Config.PremineAmount)+":"+accounts[0].ID.String(),
			accounts[1].ID.String()+":"+strconv.Itoa(c.Config.PremineAmount)+
				","+
				accounts[2].ID.String()+":"+strconv.Itoa(c.Config.PremineAmount),
		)
		if err != nil {
			return errors.Wrap(err, "failed to update genesis with asset info")
		}
	}

	return nil
}

func (c *Cluster) Fail(err error) {
	c.once.Do(func() {
		c.executionErr = err
		close(c.failCh)
	})
}

func (c *Cluster) Stop() {
	if c.Servers != nil {
		for _, srv := range c.Servers {
			if srv.isRunning() {
				srv.Stop()
			}
		}

		c.Servers = nil
	}

	if c.BootNode != nil {
		err := c.BootNode.Stop()
		if err != nil {
			log.Fatal(err)
		}

		c.BootNode = nil
	}
}

func (c *Cluster) initTestServer(i int) {
	cfg := &ServerConfig{
		Name:              fmt.Sprintf("test_%d", i),
		LogLevel:          c.Config.LogLevel,
		DataDir:           "./" + c.Config.TempDir + "/test_" + strconv.Itoa(i),
		CleanDB:           "false",
		DiscoveryInterval: "1000",
		ConfigPath:        "./" + c.Config.TempDir + fmt.Sprintf("/test_%d/config.json", i),
		GenesisPath:       c.Config.Dir(genesisFile),
		OperatorSlots:     1,
		ValidatorSlots:    3,
	}

	if c.Config.EnableSortition {
		cfg.OperatorSlots = 1
	} else if i == 0 {
		cfg.OperatorSlots = 1
	}

	// make the first node as operator
	if i == 0 {
		cfg.JSONRPCPort = c.Config.JSONRPCPort // use the give port number as it is without incrementing it
	} else {
		cfg.JSONRPCPort = c.Config.GetOpenPortForServer() // get incremented port number from second node onwards
	}

	if len(c.serverConfigs) > 0 {
		cfg = c.serverConfigs[i]
	}

	srv := NewServer(c.Config, cfg)

	go func(node *Node) {
		<-node.Wait()

		if !node.ExitResult().Signaled {
			c.Fail(fmt.Errorf("server at dir '%s' has stopped unexpectedly", cfg.DataDir))
		}
	}(srv.Node)

	c.Servers[srv.JSONRPCAddr()] = srv
}

// helpers
func generateAndStoreNewKey(keyfile string) (crypto.PrivKey, error) {
	key, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 256, crand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key: %w", err)
	}

	data, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := os.WriteFile(keyfile, data, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write key to file: %w", err)
	}

	logger.Debug("Generated new key")

	return key, nil
}

func getBindedIP() (string, error) {
	// Retrieve the network interface address of the host machine
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Return the error
		return "", errors.New("unable to read network interfaces")
	}

	var ipAddrs string

	// Iterate over the network interface addresses
	for _, a := range addrs {
		// Check that the address is an IP address and not a loopback address
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Check if the IP address is an IPv4 address
			if ipnet.IP.To4() != nil {
				// Convert into a string IP address and store to variable
				ipAddrs = ipnet.IP.String()

				break
			}
		}
	}

	return ipAddrs, nil
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}
