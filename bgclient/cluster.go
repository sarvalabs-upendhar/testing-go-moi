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

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/battleground/common"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/tests"
)

const (
	accountsFile    = "accounts.json"
	instancesFile   = "instances.json"
	artifactFile    = "artifact.json"
	genesisFile     = "genesis.json"
	bootnodeFileKey = "file.key"

	guardianManifest = "./../compute/corelogics/guardian-registry/guardian_manifest.json"
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

type Guardian struct {
	GuardianOperator string
	KramaID          string
	DeviceID         string
	PublicKey        []byte
	IncentiveWallet  identifiers.Address
	ExtraData        []byte
}

type SetupInput struct {
	EnforceApprovals    bool `polo:"enforceApprovals"`
	EnforceNodeLimits   bool `polo:"enforceNodeLimits"`
	EnforceDeviceLimits bool `polo:"enforceDeviceLimits"`

	LimitKYC    uint64 `polo:"limitKYC"`
	LimitKYB    uint64 `polo:"limitKYB"`
	LimitDevice uint64 `polo:"limitDevice"`

	Master               string                `polo:"master"`
	Approvers            []identifiers.Address `polo:"approvers"`
	PreApprovedKramaIDs  []string              `polo:"preApprovedKramaIDs"`
	PreApprovedAddresses []identifiers.Address `polo:"preApprovedAddresses"`

	Guardians []Guardian `polo:"guardians"`
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

func NewTestCluster(clusterConfig *ClusterConfig, serverConfigs []*ServerConfig) (*Cluster, error) {
	cluster := &Cluster{
		Config:        clusterConfig,
		serverConfigs: serverConfigs,
		Servers:       make(map[string]*Server),
		failCh:        make(chan struct{}),
		once:          sync.Once{},
	}

	if err := common.CreateDirSafe(clusterConfig.TempDir, os.ModePerm); err != nil {
		return nil, err
	}

	if err := cluster.startBootNode(); err != nil {
		return nil, errors.Wrap(err, "failed to start bootnode")
	}

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

	instances, err := readInstancesFile(c.Config.Dir(instancesFile))
	if err != nil {
		return err
	}

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterRuntime(pisa.NewRuntime(), nil)

	// Read manifest file
	manifest, err := engineio.ReadManifestFile(c.Config.GuardianPathDir(guardianManifest))
	if err != nil {
		return err
	}

	encodedManifest, err := manifest.Encode(engineio.POLO)
	if err != nil {
		return err
	}

	guardians := make([]Guardian, 0)
	wallet, _ := identifiers.NewAddressFromHex("0x39ff5c082ef1bd55782fd44939f3c7011af10592a423cbc00df3bf01e306b6dc")

	for _, instance := range instances {
		guardians = append(guardians, Guardian{
			GuardianOperator: instance.KramaID,
			KramaID:          instance.KramaID,
			PublicKey:        must(hex.DecodeString(instance.ConsensusKey)),
			DeviceID:         "ABCDEFG",
			IncentiveWallet:  wallet,
		})
	}

	calldata, _ := polo.Polorize(SetupInput{
		EnforceApprovals:    true,
		EnforceNodeLimits:   true,
		EnforceDeviceLimits: false,
		LimitKYC:            20,
		LimitKYB:            100,
		LimitDevice:         1,
		Master:              "0x39ff5c082ef1bd55782fd44939f3c7011af10592a423cbc00df3bf01e306b6dc",
		Approvers: []identifiers.Address{
			must(identifiers.NewAddressFromHex("0x53e9ec9f78f0397cd611bf0a0793c07673cbbf51cb172ae7d6ccf0efa5803f94")),
			must(identifiers.NewAddressFromHex("0x898ca25ac7a51a36894b9c9f55ec6212500dd8e0c01f6591f0eb9f5b0bc84655")),
		},
		PreApprovedKramaIDs: []string{
			"a5JLBNzoxVHvxFRUUhoFpC8YwZHUAb5krfnQWokcA8MdibmZ9H.16Uiu2HAmVNTp43B3axQfZYwU2hTVXuHMBJzGcvghHST9BzDvwpnn",
		},
		PreApprovedAddresses: []identifiers.Address{
			must(identifiers.NewAddressFromHex("0x9a3245e022fc769d8ee70548c4e6e7d833a0bbd5d3a6b2597ee3e692c10e0bd5")),
		},
		Guardians: guardians,
	}, polo.DocStructs())

	a := artifact{
		Name:     "guardian-contract",
		Callsite: "Setup!",
		Calldata: "0x" + hex.EncodeToString(calldata),
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

	privKey, err := generateAndStoreNewKey(fileKey)
	if err != nil {
		return err
	}

	id, err := peer.IDFromPublicKey(privKey.GetPublic())
	if err != nil {
		return err
	}

	ip, err := getBindedIP()
	if err != nil {
		return err
	}

	// Build arguments
	args := []string{
		"bootnode",
		// add port
		"--port", fmt.Sprintf("%d", c.Config.BootNodePort),
		"--key-path", fileKey,
		"--ip-address", ip,
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
		"--guardian-path", c.Config.Dir(artifactFile),
		"--genesis-path", c.Config.Dir(genesisFile),
	}

	return runCommand(c.Config.McutilsBinary, args, c.Config.GetStdout("genesis"))
}

func (c *Cluster) updateGenesisWithAsset(assetInfo string, alloc string) error {
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

		accounts, err := tests.GetAddressFromAccountsFile(c.Config.Dir(accountsFile))
		if err != nil {
			return errors.Wrap(err, "failed to read accounts file")
		}

		err = c.updateGenesisWithAsset(
			asset+":0:0:false:false:"+accounts[0],
			accounts[0]+":"+strconv.Itoa(c.Config.PremineAmount)+
				","+
				accounts[1]+":"+strconv.Itoa(c.Config.PremineAmount),
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
		DataDir:           "./test_" + strconv.Itoa(i),
		CleanDB:           "true",
		DiscoveryInterval: "1s",
		ConfigPath:        fmt.Sprintf("./test_%d/config.json", i),
		GenesisPath:       c.Config.Dir(genesisFile),
		OperatorSlots:     -1,
		ValidatorSlots:    3,
	}

	// make the first node as operator
	if i == 0 {
		cfg.OperatorSlots = 1
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
