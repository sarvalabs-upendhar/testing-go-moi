package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cmdCommon "github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/mudra"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const genesisURL = "https://moichain-pub.s3.amazonaws.com/genesis.json"

// Params holds raw config and also custom types which will be extracted from raw config
type Params struct {
	rawCfg          *cmdCommon.Config
	TrustedPeers    []common.NodeInfo
	StaticPeers     []common.NodeInfo
	BootstrapPeers  []maddr.Multiaddr
	ListenAddresses []maddr.Multiaddr
	JSONRPCAddr     *net.TCPAddr
	PrometheusAddr  *net.TCPAddr
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
			return errors.New("invalid trusted node address")
		}

		p.TrustedPeers = append(p.TrustedPeers, common.NodeInfo{
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

		p.StaticPeers = append(p.StaticPeers, common.NodeInfo{
			ID:      kramaid.KramaID(staticNode.ID),
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
// if enable tracing is true and jaegar address isn't provided then error will be thrown
// if babylon flag is provided genesis file will be downloaded at path/genesis.json and raw config path is set
func (p *Params) applyFlags(cmd *cobra.Command, path string) error {
	if isGenesisSet(cmd) {
		p.rawCfg.Genesis = GenesisPath
	}

	if isOperatorSlotSet(cmd) {
		p.rawCfg.Consensus.OperatorSlots = OperatorSlots
	}

	if isValidatorSlotSet(cmd) {
		p.rawCfg.Consensus.ValidatorSlots = ValidatorSlots
	}

	if isCleanDBSet(cmd) {
		p.rawCfg.DB.CleanDB = CleanDB
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

	if EnableTracing && p.rawCfg.Telemetry.JaegerAddr == "" {
		return errors.New("tracing is enabled but a valid JaegerCollector address is not passed")
	}

	if Babylon {
		p.rawCfg.Genesis = path + "/genesis.json"
		if err := downloadFile(p.rawCfg.Genesis, genesisURL); err != nil {
			return err
		}

		return nil
	}

	return nil
}

func (p *Params) getVaultConfig() *mudra.VaultConfig {
	return &mudra.VaultConfig{
		DataDir:      p.rawCfg.Vault.DataDir,
		NodePassword: p.rawCfg.Vault.NodePassword,
		SeedPhrase:   p.rawCfg.Vault.SeedPhrase,
		Mode:         p.rawCfg.Vault.Mode,
		NodeIndex:    p.rawCfg.Vault.NodeIndex,
		InMemory:     p.rawCfg.Vault.InMemory,
	}
}

func (p *Params) getNetworkConfig() *common.NetworkConfig {
	return &common.NetworkConfig{
		BootstrapPeers:     p.BootstrapPeers,
		TrustedPeers:       p.TrustedPeers,
		StaticPeers:        p.StaticPeers,
		MaxPeers:           p.rawCfg.Network.MaxPeers,
		RelayNodeAddr:      p.rawCfg.Network.RelayNodeAddr,
		ListenAddresses:    p.ListenAddresses,
		JSONRPCAddr:        p.JSONRPCAddr,
		MTQ:                p.rawCfg.Network.MTQ,
		CorsAllowedOrigins: p.rawCfg.Network.CorsAllowedOrigins,
		NetworkSize:        p.rawCfg.Network.NetworkSize,
		NoDiscovery:        p.rawCfg.Network.NoDiscovery,
		RefreshSenatus:     p.rawCfg.Network.RefreshSenatus,
		InboundConnLimit:   p.rawCfg.Network.InboundConnLimit,
		OutboundConnLimit:  p.rawCfg.Network.OutboundConnLimit,
	}
}

func (p *Params) getConsensusConfig(path string) *common.ConsensusConfig {
	return &common.ConsensusConfig{
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
	}
}

func (p *Params) getSyncerConfig() *common.SyncerConfig {
	return &common.SyncerConfig{
		ShouldExecute:  p.rawCfg.Syncer.ShouldExecute,
		TrustedPeers:   p.rawCfg.Syncer.TrustedPeers,
		EnableSnapSync: p.rawCfg.Syncer.EnableSnapSync,
		SyncMode:       types.SyncMode(p.rawCfg.Syncer.SyncMode),
	}
}

func (p *Params) getChainConfig() *common.ChainConfig {
	return &common.ChainConfig{
		GenesisFilePath: p.rawCfg.Genesis,
	}
}

func (p *Params) getDBConfig(path string) *common.DBConfig {
	return &common.DBConfig{
		CleanDB:      p.rawCfg.DB.CleanDB,
		DBFolderPath: path + common.DefaultDBDirectory,
		MaxSnapSize:  p.rawCfg.DB.MaxSnapSize,
	}
}

func (p *Params) getExecutionConfig() *common.ExecutionConfig {
	return &common.ExecutionConfig{
		FuelLimit: p.rawCfg.Execution.FuelLimit.ToInt(),
	}
}

func (p *Params) getIXPoolConfig() *common.IxPoolConfig {
	return &common.IxPoolConfig{
		Mode:       p.rawCfg.Ixpool.Mode,
		PriceLimit: p.rawCfg.Ixpool.PriceLimit.ToInt(),
	}
}

func (p *Params) getTelemetryConfig() *common.Telemetry {
	return &common.Telemetry{
		PrometheusAddr: p.PrometheusAddr,
		JaegerAddr:     p.rawCfg.Telemetry.JaegerAddr,
	}
}

// processRawParams converts all raw types to custom types
func (p *Params) processRawParams() error {
	if err := p.assignNetworkNodes(); err != nil {
		return err
	}

	if err := p.assignNetworkLibp2pListenAddress(); err != nil {
		return err
	}

	if err := p.assignNetworkJSONRPCAddr(); err != nil {
		return err
	}

	if err := p.buildTelemetryConfig(); err != nil {
		return err
	}

	return nil
}

// generateNodeConfig generates node config using params
func (p *Params) generateNodeConfig(dataDir string) *common.Config {
	return &common.Config{
		NodeType:       p.rawCfg.NodeType,
		KramaIDVersion: p.rawCfg.KramaIDVersion,
		Vault:          p.getVaultConfig(),
		Network:        p.getNetworkConfig(),
		Chain:          p.getChainConfig(),
		Consensus:      p.getConsensusConfig(dataDir),
		DB:             p.getDBConfig(dataDir),
		Execution:      p.getExecutionConfig(),
		IxPool:         p.getIXPoolConfig(),
		Syncer:         p.getSyncerConfig(),
		Metrics:        *p.getTelemetryConfig(),
		LogFilePath:    p.rawCfg.LogFilePath,
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

func isConfigPathSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(configFlag)
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

func isCleanDBSet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(cleanDBFlag)
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

// BuildNodeConfig function creates a node configuration by combining a default configuration, a configuration file,
// and any provided flags. Here are the steps involved:
// 1. Default configuration is selected based on the "babylon" flag.
// 2. If a configuration file path is provided, it overrides the default configuration.
// 3. Any provided flags overwrite the previous state of the configuration.
// 4. The raw configuration is converted to custom types and stored in params.
// 5. The final node configuration is generated from the params.
func BuildNodeConfig(cmd *cobra.Command, dataDir string) (*common.Config, error) {
	var (
		err    error
		params = Params{
			rawCfg:          &cmdCommon.Config{},
			TrustedPeers:    make([]common.NodeInfo, 0),
			StaticPeers:     make([]common.NodeInfo, 0),
			BootstrapPeers:  make([]maddr.Multiaddr, 0),
			ListenAddresses: make([]maddr.Multiaddr, 0),
		}
	)

	if Babylon {
		params.rawCfg = cmdCommon.DefaultBabylonConfig(dataDir)
	} else {
		params.rawCfg = cmdCommon.DefaultDevnetConfig(dataDir)
	}

	if isConfigPathSet(cmd) {
		params.rawCfg, err = ReadConfig(filepath.Join(Directory, ConfigPath))
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
