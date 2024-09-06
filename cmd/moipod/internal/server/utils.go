package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/spf13/cobra"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
)

const (
	genesisURL      = "https://moichain-pub.s3.amazonaws.com/genesis.json"
	trustedPeersURL = "https://moichain-pub.s3.amazonaws.com/trusted_peers.json"
)

// Params holds raw config and also custom types which will be extracted from raw config
type Params struct {
	rawCfg              *cmdCommon.Config
	NetworkTrustedPeers []config.NodeInfo
	StaticPeers         []config.NodeInfo
	SyncerPeers         []config.NodeInfo
	TrustedPeers        []config.NodeInfo
	BootstrapPeers      []maddr.Multiaddr
	ListenAddresses     []maddr.Multiaddr
	PublicP2PAddresses  []maddr.Multiaddr
	JSONRPCAddr         *net.TCPAddr
	PrometheusAddr      *net.TCPAddr
}

func (p *Params) buildTelemetryConfig() (err error) {
	if p.rawCfg.Telemetry.PrometheusAddr != "" {
		p.PrometheusAddr, err = utils.ResolveAddr(p.rawCfg.Telemetry.PrometheusAddr)
		if err != nil {
			return errors.New("invalid prometheus address")
		}
	}

	return nil
}

func (p *Params) assignNetworkBootStrapNodes() error {
	// validate bootnode address
	if len(p.rawCfg.Network.BootStrapPeers) == 0 {
		return errors.New("minimum one bootnode is required")
	}

	for _, v := range p.rawCfg.Network.BootStrapPeers {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid bootnode address")
		}

		p.BootstrapPeers = append(p.BootstrapPeers, addr)
	}

	return nil
}

func (p *Params) assignNetworkTrustedNodes() error {
	for _, trustedNode := range p.rawCfg.Network.TrustedPeers {
		addr, err := maddr.NewMultiaddr(trustedNode.Address)
		if err != nil {
			return errors.New("invalid network trusted node address")
		}

		p.NetworkTrustedPeers = append(p.NetworkTrustedPeers, config.NodeInfo{
			ID:      kramaid.KramaID(trustedNode.ID),
			Address: addr,
		})
	}

	return nil
}

func (p *Params) assignNetworkStaticNodes() error {
	for _, staticNode := range p.rawCfg.Network.StaticPeers {
		addr, err := maddr.NewMultiaddr(staticNode.Address)
		if err != nil {
			return errors.New("invalid static node address")
		}

		p.StaticPeers = append(p.StaticPeers, config.NodeInfo{
			ID:      kramaid.KramaID(staticNode.ID),
			Address: addr,
		})
	}

	return nil
}

func (p *Params) assignSyncerPeers() error {
	for _, syncPeer := range p.rawCfg.Syncer.SyncPeers {
		addr, err := maddr.NewMultiaddr(syncPeer.Address)
		if err != nil {
			return errors.New("invalid syncer trusted node address")
		}

		p.SyncerPeers = append(p.SyncerPeers, config.NodeInfo{
			ID:      kramaid.KramaID(syncPeer.ID),
			Address: addr,
		})
	}

	return nil
}

func (p *Params) assignTrustedPeers() error {
	for _, trustedPeer := range p.rawCfg.Consensus.TrustedPeers {
		addr, err := maddr.NewMultiaddr(trustedPeer.Address)
		if err != nil {
			return errors.New("invalid trusted node address")
		}

		p.TrustedPeers = append(p.TrustedPeers, config.NodeInfo{
			ID:      kramaid.KramaID(trustedPeer.ID),
			Address: addr,
		})
	}

	return nil
}

func (p *Params) assignNetworkNodes() error {
	var err error

	if err = p.assignNetworkTrustedNodes(); err != nil {
		return err
	}

	if err = p.assignNetworkStaticNodes(); err != nil {
		return err
	}

	if err = p.assignNetworkBootStrapNodes(); err != nil {
		return err
	}

	return nil
}

func (p *Params) assignNetworkLibp2pListenAddress() error {
	if len(p.rawCfg.Network.Libp2pAddr) == 0 {
		return errors.New("listener address not specified")
	}

	for _, v := range p.rawCfg.Network.Libp2pAddr {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid libp2p address")
		}

		p.ListenAddresses = append(p.ListenAddresses, addr)
	}

	return nil
}

func (p *Params) assignNetworkLibp2pPublicAddress() error {
	for _, v := range p.rawCfg.Network.PublicP2PAddresses {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid public p2p address")
		}

		p.PublicP2PAddresses = append(p.PublicP2PAddresses, addr)
	}

	return nil
}

func (p *Params) assignNetworkJSONRPCAddr() (err error) {
	// validate json-rpc address
	if p.rawCfg.Network.JSONRPCAddr == "" {
		return errors.New("empty json address")
	}

	p.JSONRPCAddr, err = utils.ResolveAddr(p.rawCfg.Network.JSONRPCAddr)
	if err != nil {
		return errors.New("invalid json-rpc address")
	}

	return nil
}

// applyFlags sets raw config flags accordingly if flags are provided explicitly
// if enable tracing is true and OTLP address isn't provided then error will be thrown
// if babylon flag is provided genesis file will be downloaded at path/genesis.json and raw config path is set
func (p *Params) applyFlags(cmd *cobra.Command, path string) error {
	if isGenesisSet(cmd) {
		p.rawCfg.Consensus.GenesisPath = GenesisPath
	}

	if isPublicP2PAddrSet(cmd) {
		addrs := make([]string, 0)

		for _, addr := range PublicP2PAddresses {
			if strings.Contains(addr, "ip6") && !AllowIPv6Addresses {
				return errors.New("ipv6 addresses are disable through the flag")
			}

			addrs = append(addrs, addr)
		}

		p.rawCfg.Network.PublicP2PAddresses = addrs
	}

	if isOperatorSlotSet(cmd) {
		p.rawCfg.Consensus.OperatorSlots = OperatorSlots
	}

	if isValidatorSlotSet(cmd) {
		p.rawCfg.Consensus.ValidatorSlots = ValidatorSlots
	}

	if isEnableDebugModeSet(cmd) {
		p.rawCfg.Consensus.EnableDebugMode = enableDebugMode
	}

	if isCleanDBSet(cmd) {
		p.rawCfg.DB.CleanDB = CleanDB
	}

	if isDisableRegistrationSet(cmd) {
		p.rawCfg.DisableRegistration = DisableRegistration
	}

	if isAllowOriginsSet(cmd) {
		p.rawCfg.Network.CorsAllowedOrigins = CorsAllowedOrigins
	}

	if isBootnodesSet(cmd) {
		p.rawCfg.Network.BootStrapPeers = Bootnodes
	}

	if isNodePasswordSet(cmd) {
		p.rawCfg.Vault.NodePassword = NodePassword
	}

	if isLogPathSet(cmd) {
		p.rawCfg.LogFilePath = LogDirPath
	}

	if isAllowIPv6AddressesSet(cmd) {
		p.rawCfg.Network.AllowIPv6Addresses = AllowIPv6Addresses
	}

	if isDiscoveryIntervalSet(cmd) {
		p.rawCfg.Network.DiscoveryInterval = DiscoveryInterval
	}

	if EnableTracing && p.rawCfg.Telemetry.OtlpAddress == "" {
		return errors.New("tracing is enabled but a valid OtlpCollector address is not passed")
	}

	if Babylon {
		p.rawCfg.Consensus.GenesisPath = path + "/genesis.json"
		if err := downloadFile(p.rawCfg.Consensus.GenesisPath, genesisURL); err != nil {
			return err
		}

		trustedPeers := path + "/trustedpeers.json"

		if err := downloadFile(trustedPeers, trustedPeersURL); err != nil {
			return err
		}

		peers, err := readTrustedPeers(trustedPeers)
		if err != nil {
			return err
		}

		p.rawCfg.Consensus.TrustedPeers = peers

		return nil
	}

	return nil
}

func (p *Params) getVaultConfig() *crypto.VaultConfig {
	return &crypto.VaultConfig{
		DataDir:      p.rawCfg.Vault.DataDir,
		NodePassword: p.rawCfg.Vault.NodePassword,
		SeedPhrase:   p.rawCfg.Vault.SeedPhrase,
		Mode:         p.rawCfg.Vault.Mode,
		NodeIndex:    p.rawCfg.Vault.NodeIndex,
		InMemory:     p.rawCfg.Vault.InMemory,
	}
}

func (p *Params) getNetworkConfig() *config.NetworkConfig {
	return &config.NetworkConfig{
		BootstrapPeers:     p.BootstrapPeers,
		TrustedPeers:       p.NetworkTrustedPeers,
		StaticPeers:        p.StaticPeers,
		MaxPeers:           p.rawCfg.Network.MaxPeers,
		RelayNodeAddr:      p.rawCfg.Network.RelayNodeAddr,
		ListenAddresses:    p.ListenAddresses,
		PublicP2PAddresses: p.PublicP2PAddresses,
		JSONRPCAddr:        p.JSONRPCAddr,
		MTQ:                p.rawCfg.Network.MTQ,
		CorsAllowedOrigins: p.rawCfg.Network.CorsAllowedOrigins,
		NetworkSize:        p.rawCfg.Network.NetworkSize,
		NoDiscovery:        p.rawCfg.Network.NoDiscovery,
		RefreshSenatus:     p.rawCfg.Network.RefreshSenatus,
		InboundConnLimit:   p.rawCfg.Network.InboundConnLimit,
		OutboundConnLimit:  p.rawCfg.Network.OutboundConnLimit,
		MinimumConnections: p.rawCfg.Network.MinimumConnections,
		MaximumConnections: p.rawCfg.Network.MaximumConnections,
		AllowIPv6Addresses: p.rawCfg.Network.AllowIPv6Addresses,
		DisablePrivateIP:   p.rawCfg.Network.DisablePrivateIP,
		DiscoveryInterval:  p.rawCfg.Network.DiscoveryInterval,
		EnableIPColocation: p.rawCfg.Network.EnableIPColocation,
	}
}

func (p *Params) getConsensusConfig(path string) *config.ConsensusConfig {
	return &config.ConsensusConfig{
		DirectoryPath:         path + "/consensus",
		TimeoutPropose:        time.Duration(p.rawCfg.Consensus.TimeoutPropose) * time.Millisecond,
		TimeoutProposeDelta:   time.Duration(p.rawCfg.Consensus.TimeoutProposeDelta) * time.Millisecond,
		TimeoutPrevote:        time.Duration(p.rawCfg.Consensus.TimeoutPrevote) * time.Millisecond,
		TimeoutPrevoteDelta:   time.Duration(p.rawCfg.Consensus.TimeoutPrevoteDelta) * time.Millisecond,
		TimeoutPrecommit:      time.Duration(p.rawCfg.Consensus.TimeoutPrecommit) * time.Millisecond,
		TimeoutPrecommitDelta: time.Duration(p.rawCfg.Consensus.TimeoutPrecommitDelta) * time.Millisecond,
		TimeoutCommit:         time.Duration(p.rawCfg.Consensus.TimeoutCommit) * time.Millisecond,
		SkipTimeoutCommit:     p.rawCfg.Consensus.SkipTimeoutCommit,
		AccountWaitTime:       time.Duration(p.rawCfg.Consensus.AccountWaitTime) * time.Millisecond,
		MessageDelay:          time.Duration(p.rawCfg.Consensus.MessageDelay) * time.Millisecond,
		Precision:             time.Duration(p.rawCfg.Consensus.Precision) * time.Nanosecond,
		ValidatorSlotCount:    p.rawCfg.Consensus.ValidatorSlots,
		OperatorSlotCount:     p.rawCfg.Consensus.OperatorSlots,
		EnableDebugMode:       p.rawCfg.Consensus.EnableDebugMode,
		MinGossipPeers:        p.rawCfg.Consensus.MinGossipPeers,
		MaxGossipPeers:        p.rawCfg.Consensus.MinGossipPeers,
		EnableSortition:       p.rawCfg.Consensus.EnableSortition,
		GenesisTimestamp:      p.rawCfg.Consensus.GenesisTime,
		GenesisFilePath:       p.rawCfg.Consensus.GenesisPath,
		GenesisSeed:           p.rawCfg.Consensus.GenesisSeed,
		GenesisProof:          p.rawCfg.Consensus.GenesisProof,
		TrustedPeers:          p.TrustedPeers,
	}
}

func (p *Params) getSyncerConfig() *config.SyncerConfig {
	return &config.SyncerConfig{
		ShouldExecute:  p.rawCfg.Syncer.ShouldExecute,
		SyncPeers:      p.SyncerPeers,
		EnableSnapSync: p.rawCfg.Syncer.EnableSnapSync,
		SyncMode:       common.SyncMode(p.rawCfg.Syncer.SyncMode),
	}
}

func (p *Params) getDBConfig(path string) *config.DBConfig {
	return &config.DBConfig{
		CleanDB:      p.rawCfg.DB.CleanDB,
		DBFolderPath: path + config.DefaultDBDirectory,
		MaxSnapSize:  p.rawCfg.DB.MaxSnapSize,
	}
}

func (p *Params) getExecutionConfig() *config.ExecutionConfig {
	return &config.ExecutionConfig{
		FuelLimit: uint64(p.rawCfg.Execution.FuelLimit),
	}
}

func (p *Params) getIXPoolConfig() *config.IxPoolConfig {
	return &config.IxPoolConfig{
		Mode:                    p.rawCfg.IxPool.Mode,
		PriceLimit:              p.rawCfg.IxPool.PriceLimit.ToInt(),
		MaxSlots:                p.rawCfg.IxPool.MaxSlots,
		IxIncomingFilterMaxSize: p.rawCfg.IxPool.IxIncomingFilterMaxSize,
		MaxIxGroupSize:          p.rawCfg.IxPool.MaxIxGroupSize,
		EnableIxFlooding:        p.rawCfg.IxPool.EnableIxFlooding,
		EnableRawIxFiltering:    p.rawCfg.IxPool.EnableRawIxFiltering,
	}
}

func (p *Params) getTelemetryConfig() *config.Telemetry {
	return &config.Telemetry{
		PrometheusAddr: p.PrometheusAddr,
		OtlpAddress:    p.rawCfg.Telemetry.OtlpAddress,
		Token:          p.rawCfg.Telemetry.Token,
	}
}

func (p *Params) getJSONRPCConfig() *config.JSONRPCConfig {
	return &config.JSONRPCConfig{
		TesseractRangeLimit: config.DefaultTesseractRangeLimit,
		BatchLengthLimit:    config.DefaultBatchLengthLimit,
	}
}

func (p *Params) getStateConfig() *config.StateConfig {
	return &config.StateConfig{
		TreeCacheSize: p.rawCfg.State.TreeCacheSize,
	}
}

// processRawParams converts all raw types to custom types
func (p *Params) processRawParams() error {
	var err error

	if err = p.assignNetworkNodes(); err != nil {
		return err
	}

	if err = p.assignSyncerPeers(); err != nil {
		return err
	}

	if err = p.assignTrustedPeers(); err != nil {
		return err
	}

	if err = p.assignNetworkLibp2pListenAddress(); err != nil {
		return err
	}

	if err = p.assignNetworkLibp2pPublicAddress(); err != nil {
		return err
	}

	if err = p.assignNetworkJSONRPCAddr(); err != nil {
		return err
	}

	if err = p.buildTelemetryConfig(); err != nil {
		return err
	}

	return nil
}

// generateNodeConfig generates node config using params
func (p *Params) generateNodeConfig(dataDir string) *config.Config {
	return &config.Config{
		NodeType:            p.rawCfg.NodeType,
		KramaIDVersion:      p.rawCfg.KramaIDVersion,
		Vault:               p.getVaultConfig(),
		Network:             p.getNetworkConfig(),
		Consensus:           p.getConsensusConfig(dataDir),
		DB:                  p.getDBConfig(dataDir),
		Execution:           p.getExecutionConfig(),
		IxPool:              p.getIXPoolConfig(),
		Syncer:              p.getSyncerConfig(),
		Metrics:             *p.getTelemetryConfig(),
		LogFilePath:         p.rawCfg.LogFilePath,
		JSONRPC:             p.getJSONRPCConfig(),
		NetworkID:           p.rawCfg.NetworkID,
		State:               p.getStateConfig(),
		DisableRegistration: p.rawCfg.DisableRegistration,
	}
}

func ReadConfig(path string) (*cmdCommon.Config, error) {
	cfg := new(cmdCommon.Config)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrReadingConfig
	}

	if err = json.Unmarshal(file, cfg); err != nil {
		log.Print(err)

		return nil, errors.Wrap(err, ErrReadingConfig.Error())
	}

	return cfg, nil
}

func readTrustedPeers(path string) ([]cmdCommon.PeerInfo, error) {
	peers := make([]cmdCommon.PeerInfo, 0)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading trusted peers file")
	}

	if err = json.Unmarshal(file, &peers); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling trusted peers file")
	}

	return peers, nil
}

func isConfigPathSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(configFlag)
}

func isLogPathSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(logDirPathFlag)
}

func isAllowIPv6AddressesSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(allowIPv6AddressesFlag)
}

func isDiscoveryIntervalSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(discoveryIntervalFlag)
}

func isGenesisSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(genesisFlag)
}

func isOperatorSlotSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(operatorSlotFlag)
}

func isValidatorSlotSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(validatorSlotFlag)
}

func isEnableDebugModeSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(enableDebugModeFlag)
}

func isCleanDBSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(cleanDBFlag)
}

func isDisableRegistrationSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(disableRegistration)
}

func isAllowOriginsSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(allowOriginsFlag)
}

func isBootnodesSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(bootNodesFlag)
}

func isNodePasswordSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(nodePasswordFlag)
}

func isPublicP2PAddrSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(publicP2PAddrFlag)
}

// BuildNodeConfig function creates a node configuration by combining default configuration and the configuration file,
// and any provided flags. Here are the steps involved:
// 1. Default configuration is selected based on the "babylon" flag.
// 2. If a configuration file path is provided, it overrides the default configuration.
// 3. Any provided flags overwrite the previous state of the configuration.
// 4. The raw configuration is converted to custom types and stored in params.
// 5. The final node configuration is generated from the params.
func BuildNodeConfig(cmd *cobra.Command, dataDir string) (*config.Config, error) {
	var (
		err    error
		params = Params{
			rawCfg:              &cmdCommon.Config{},
			NetworkTrustedPeers: make([]config.NodeInfo, 0),
			StaticPeers:         make([]config.NodeInfo, 0),
			BootstrapPeers:      make([]maddr.Multiaddr, 0),
			ListenAddresses:     make([]maddr.Multiaddr, 0),
		}
	)

	if Babylon {
		params.rawCfg = cmdCommon.DefaultBabylonConfig(dataDir)
	} else {
		params.rawCfg = cmdCommon.DefaultDevnetConfig(dataDir)
	}

	if isConfigPathSet(cmd) {
		params.rawCfg, err = ReadConfig(ConfigPath)
		if err != nil {
			cmdCommon.Err(err)
		}
	}

	if err := params.applyFlags(cmd, dataDir); err != nil {
		return nil, err
	}

	if err := params.processRawParams(); err != nil {
		return nil, err
	}

	return params.generateNodeConfig(dataDir), nil
}

func downloadFile(outputPath string, url string) error {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	defer outputFile.Close()

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("unexpected response status %v", resp.StatusCode))
	}

	_, err = io.Copy(outputFile, resp.Body)
	if err != nil {
		return err
	}

	log.Println("File downloaded successfully")

	return nil
}
