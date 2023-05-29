package server

import (
	"encoding/json"
	"log"
	"math/big"
	"os"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
)

func ReadConfig(path string) (*Config, error) {
	cfg := new(Config)

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

func BuildConfig(dataDir string, fileCfg *Config) (*common.Config, error) {
	var err error

	nodeCfg := common.DefaultConfig(dataDir)
	nodeCfg.LogFilePath = fileCfg.LogFilePath

	// TODO:Check node type and krama version

	buildChainConfig(nodeCfg, fileCfg)

	if err = buildNetworkConfig(nodeCfg, fileCfg); err != nil {
		return nil, err
	}

	buildConsensusConfig(nodeCfg, fileCfg)
	buildIxPoolConfig(nodeCfg, fileCfg)
	buildDBConfig(nodeCfg, fileCfg)

	if err = buildTelemetryConfig(nodeCfg, fileCfg); err != nil {
		return nil, err
	}

	buildVaultConfig(nodeCfg, fileCfg)

	return nodeCfg, nil
}

func buildChainConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Genesis != "" {
		nodeCfg.Chain.GenesisFilePath = fileCfg.Genesis
	}
}

func buildNetworkConfig(nodeCfg *common.Config, fileCfg *Config) (err error) {
	assignNetworkSize(nodeCfg)
	assignNetworkMTQ(nodeCfg)
	assignNetworkNoDiscovery(nodeCfg)
	assignNetworkRefreshSenatus(nodeCfg)
	assignNetworkCORS(nodeCfg)
	assignNetworkInboundLimit(nodeCfg, fileCfg)
	assignNetworkOutboundLimit(nodeCfg, fileCfg)

	if err = assignNetworkNodes(nodeCfg, fileCfg); err != nil {
		return err
	}

	if err = assignNetworkLibp2pListenAddress(nodeCfg, fileCfg); err != nil {
		return err
	}

	if err = assignNetworkJSONRPCAddr(nodeCfg, fileCfg); err != nil {
		return err
	}

	return nil
}

func buildConsensusConfig(nodeCfg *common.Config, fileCfg *Config) {
	if OperatorSlots != -1 {
		nodeCfg.Consensus.OperatorSlotCount = OperatorSlots
	} else if fileCfg.Consensus.OperatorSlots != 0 {
		nodeCfg.Consensus.OperatorSlotCount = fileCfg.Consensus.OperatorSlots
	}

	if ValidatorSlots != -1 {
		nodeCfg.Consensus.ValidatorSlotCount = ValidatorSlots
	} else if fileCfg.Consensus.ValidatorSlots != 0 {
		nodeCfg.Consensus.ValidatorSlotCount = fileCfg.Consensus.ValidatorSlots
	}

	if AccountWaitTime != 0 {
		nodeCfg.Consensus.AccountWaitTime = time.Duration(AccountWaitTime) * time.Millisecond
	} else if fileCfg.Consensus.AccountWaitTime != 0 {
		nodeCfg.Consensus.AccountWaitTime = time.Duration(fileCfg.Consensus.AccountWaitTime) * time.Millisecond
	}
}

func buildIxPoolConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Ixpool.PriceLimit.Cmp(big.NewInt(0)) == 1 {
		nodeCfg.IxPool.PriceLimit = fileCfg.Ixpool.PriceLimit
	}

	if fileCfg.Ixpool.Mode != 0 {
		nodeCfg.IxPool.Mode = fileCfg.Ixpool.Mode
	}
}

func buildDBConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.DB.DBFolder != "" {
		nodeCfg.DB.DBFolderPath = fileCfg.DB.DBFolder
	}

	nodeCfg.DB.CleanDB = CleanDB
}

func buildTelemetryConfig(nodeCfg *common.Config, fileCfg *Config) (err error) {
	if fileCfg.Telemetry.PrometheusAddr != "" {
		nodeCfg.Metrics.PrometheusAddr, err = common.ResolveAddr(fileCfg.Telemetry.PrometheusAddr)
		if err != nil {
			return errors.New("invalid prometheus address")
		}
	}

	if EnableTracing {
		switch {
		case JaegerAddress != "":
			nodeCfg.Metrics.JaegerAddr = JaegerAddress
		case fileCfg.Telemetry.JaegerAddr != "":
			nodeCfg.Metrics.JaegerAddr = fileCfg.Telemetry.JaegerAddr
		default:
			return errors.New("tracing is enabled but a valid JaegerCollector address is not passed")
		}
	}

	return nil
}

func buildVaultConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Vault.NodePassword != "" {
		nodeCfg.Vault.NodePassword = fileCfg.Vault.NodePassword
	}

	if fileCfg.Vault.DataDir != "" {
		nodeCfg.Vault.DataDir = fileCfg.Vault.DataDir
	}
}

func assignNetworkInboundLimit(nodeCfg *common.Config, fileCfg *Config) {
	if InboundConnLimit != common.DefaultInboundConnLimit {
		nodeCfg.Network.InboundConnLimit = InboundConnLimit
	} else if fileCfg.Network.InboundConnLimit != 0 {
		nodeCfg.Network.InboundConnLimit = fileCfg.Network.InboundConnLimit
	}
}

func assignNetworkOutboundLimit(nodeCfg *common.Config, fileCfg *Config) {
	if OutboundConnLimit != common.DefaultOutboundConnLimit {
		nodeCfg.Network.OutboundConnLimit = OutboundConnLimit
	} else if fileCfg.Network.OutboundConnLimit != 0 {
		nodeCfg.Network.OutboundConnLimit = fileCfg.Network.OutboundConnLimit
	}
}

func assignNetworkSize(nodeCfg *common.Config) {
	if NetworkSize != 0 {
		nodeCfg.Network.NetworkSize = NetworkSize
	}
}

func assignNetworkMTQ(nodeCfg *common.Config) {
	if MTQ != 0 {
		nodeCfg.Network.MTQ = MTQ
	}
}

func assignNetworkNoDiscovery(nodeCfg *common.Config) {
	nodeCfg.Network.NoDiscovery = NoDiscovery
}

func assignNetworkRefreshSenatus(nodeCfg *common.Config) {
	nodeCfg.Network.RefreshSenatus = RefreshSenatus
}

func assignNetworkCORS(nodeCfg *common.Config) {
	nodeCfg.Network.CorsAllowedOrigins = CorsAllowedOrigins
}

func assignNetworkBootStrapNodes(nodeCfg *common.Config, fileCfg *Config) error {
	if Bootnode != "" {
		addr, err := maddr.NewMultiaddr(Bootnode)
		if err != nil {
			return errors.New("invalid bootnode address")
		}

		nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)

		return nil
	}

	// validate bootnode address
	if len(fileCfg.Network.BootStrapPeers) == 0 {
		return errors.New("minimum one bootnode is required")
	}

	for _, v := range fileCfg.Network.BootStrapPeers {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid bootnode address")
		}

		nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)
	}

	return nil
}

func assignNetworkTrustedNodes(nodeCfg *common.Config, fileCfg *Config, trustedNodes []PeerInfo) error {
	if len(trustedNodes) == 0 && len(fileCfg.Network.TrustedPeers) > 0 {
		trustedNodes = fileCfg.Network.TrustedPeers
	}

	for _, trustedNode := range trustedNodes {
		addr, err := maddr.NewMultiaddr(trustedNode.Address)
		if err != nil {
			return errors.New("invalid trusted node address")
		}

		nodeCfg.Network.TrustedPeers = append(nodeCfg.Network.TrustedPeers, common.NodeInfo{
			ID:      kramaid.KramaID(trustedNode.ID),
			Address: addr,
		})
	}

	return nil
}

func assignNetworkStaticNodes(nodeCfg *common.Config, fileCfg *Config, staticNodes []PeerInfo) error {
	if len(staticNodes) == 0 && len(fileCfg.Network.StaticPeers) > 0 {
		staticNodes = fileCfg.Network.StaticPeers
	}

	for _, staticNode := range staticNodes {
		addr, err := maddr.NewMultiaddr(staticNode.Address)
		if err != nil {
			return errors.New("invalid static node address")
		}

		nodeCfg.Network.StaticPeers = append(nodeCfg.Network.StaticPeers, common.NodeInfo{
			ID:      kramaid.KramaID(staticNode.ID),
			Address: addr,
		})
	}

	return nil
}

func assignNetworkNodes(nodeCfg *common.Config, fileCfg *Config) error {
	peerList, err := ReadPeerList(PeerListFilePath)
	if err != nil {
		return err
	}

	if err = assignNetworkTrustedNodes(nodeCfg, fileCfg, peerList.TrustedPeers); err != nil {
		return err
	}

	if err = assignNetworkStaticNodes(nodeCfg, fileCfg, peerList.StaticPeers); err != nil {
		return err
	}

	if err = assignNetworkBootStrapNodes(nodeCfg, fileCfg); err != nil {
		return err
	}

	return nil
}

func assignNetworkLibp2pListenAddress(nodeCfg *common.Config, fileCfg *Config) error {
	if len(fileCfg.Network.Libp2pAddr) == 0 {
		return errors.New("lip2p address not specified")
	}

	for _, v := range fileCfg.Network.Libp2pAddr {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid libp2p address")
		}

		nodeCfg.Network.ListenAddresses = append(nodeCfg.Network.ListenAddresses, addr)
	}

	return nil
}

func assignNetworkJSONRPCAddr(nodeCfg *common.Config, fileCfg *Config) (err error) {
	// validate json-rpc address
	if fileCfg.Network.JSONRPCAddr == "" {
		return errors.New("empty json address")
	}

	nodeCfg.Network.JSONRPCAddr, err = common.ResolveAddr(fileCfg.Network.JSONRPCAddr)
	if err != nil {
		return errors.New("invalid json-rpc address")
	}

	return nil
}
