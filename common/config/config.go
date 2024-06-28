package config

import (
	"math/big"
	"net"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-legacy-kramaid"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
)

type Config struct {
	NodeType       int
	KramaIDVersion int
	Vault          *crypto.VaultConfig
	Network        *NetworkConfig
	Consensus      *ConsensusConfig
	DB             *DBConfig
	Execution      *ExecutionConfig
	IxPool         *IxPoolConfig
	Syncer         *SyncerConfig
	Metrics        Telemetry
	LogFilePath    string
	JSONRPC        *JSONRPCConfig
	NetworkID      NetworkID
	State          *StateConfig
}

type Telemetry struct {
	PrometheusAddr *net.TCPAddr
	OtlpAddress    string
	Token          string
}

type SyncerConfig struct {
	ShouldExecute  bool
	TrustedPeers   []NodeInfo
	EnableSnapSync bool
	SyncMode       common.SyncMode
}

type DBConfig struct {
	CleanDB      bool
	DBFolderPath string
	MaxSnapSize  uint64
}

type ExecutionConfig struct {
	FuelLimit uint64
}

type IxPoolConfig struct {
	Mode       int
	PriceLimit *big.Int
	MaxSlots   uint64
}

type NodeInfo struct {
	ID      kramaid.KramaID
	Address maddr.Multiaddr
}

// NetworkConfig is the p2p configuration of the node
type NetworkConfig struct {
	BootstrapPeers     []maddr.Multiaddr
	TrustedPeers       []NodeInfo
	StaticPeers        []NodeInfo
	MaxPeers           uint
	RelayNodeAddr      string
	ListenAddresses    []maddr.Multiaddr
	PublicP2pAddresses []maddr.Multiaddr
	JSONRPCAddr        *net.TCPAddr
	P2PHostPort        int
	MTQ                float64
	CorsAllowedOrigins []string

	// this will be removed
	NetworkSize uint64

	NoDiscovery        bool
	RefreshSenatus     bool
	InboundConnLimit   int64
	OutboundConnLimit  int64
	MinimumConnections int
	MaximumConnections int
	AllowIPv6Addresses bool
	DisablePrivateIP   bool
	DiscoveryInterval  time.Duration
}

type ConsensusConfig struct {
	DirectoryPath         string
	TimeoutPropose        time.Duration
	TimeoutProposeDelta   time.Duration
	TimeoutPrevote        time.Duration
	TimeoutPrevoteDelta   time.Duration
	TimeoutPrecommit      time.Duration
	TimeoutPrecommitDelta time.Duration
	TimeoutCommit         time.Duration
	SkipTimeoutCommit     bool
	AccountWaitTime       time.Duration
	MessageDelay          time.Duration
	Precision             time.Duration
	ValidatorSlotCount    int
	OperatorSlotCount     int
	EnableDebugMode       bool
	MaxGossipPeers        int
	MinGossipPeers        int
	GenesisSeed           string
	GenesisProof          string
	EnableSortition       bool
	GenesisTimestamp      uint64
	GenesisFilePath       string
}

type JSONRPCConfig struct {
	TesseractRangeLimit uint8
	BatchLengthLimit    uint64
}

type StateConfig struct {
	TreeCacheSize uint64
}

func DefaultDevnetConfig(path string) *Config {
	c := &Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: &crypto.VaultConfig{
			DataDir: path,
			Mode:    crypto.GuardianMode,
		},
		Network: &NetworkConfig{
			ListenAddresses:    make([]maddr.Multiaddr, 0),
			PublicP2pAddresses: make([]maddr.Multiaddr, 0),
			BootstrapPeers:     make([]maddr.Multiaddr, 0),
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   DefaultInboundConnLimit,
			OutboundConnLimit:  DefaultOutboundConnLimit,
			MinimumConnections: DefaultMinimumConnections,
			MaximumConnections: DefaultMaximumConnections,
			AllowIPv6Addresses: false,
			DisablePrivateIP:   false,
			DiscoveryInterval:  DefaultDiscoveryInterval,
		},
		Syncer: &SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       DefaultSyncMode,
			EnableSnapSync: true,
		},

		Consensus: &ConsensusConfig{
			DirectoryPath:         path + "/consensus",
			TimeoutPropose:        30000 * time.Millisecond,
			TimeoutProposeDelta:   50000 * time.Millisecond,
			TimeoutPrevote:        10000 * time.Millisecond,
			TimeoutPrevoteDelta:   50000 * time.Millisecond,
			TimeoutPrecommit:      10000 * time.Millisecond,
			TimeoutPrecommitDelta: 50000 * time.Millisecond,
			TimeoutCommit:         10000 * time.Millisecond,
			Precision:             1000 * time.Nanosecond,
			MessageDelay:          5500 * time.Millisecond,
			AccountWaitTime:       1500 * time.Millisecond,
			OperatorSlotCount:     -1,
			ValidatorSlotCount:    3,
			MaxGossipPeers:        5,
			MinGossipPeers:        3,
			GenesisFilePath:       path + "/genesis.json",
			EnableSortition:       false,
			GenesisProof:          DefaultGenesisProof,
			GenesisSeed:           DefaultGenesisSeed,
		},
		DB: &DBConfig{
			CleanDB:      false,
			DBFolderPath: path + DefaultDBDirectory,
			MaxSnapSize:  DefaultSnapSize, // 6GB limit
		},
		Execution: &ExecutionConfig{
			FuelLimit: DefaultFuelLimit,
		},
		IxPool: &IxPoolConfig{
			Mode:       DefaultIxPoolMode,
			PriceLimit: DefaultIxPriceLimit,
		},
		Metrics: Telemetry{
			PrometheusAddr: nil,
			OtlpAddress:    "",
			Token:          "",
		},
		JSONRPC: &JSONRPCConfig{
			TesseractRangeLimit: DefaultTesseractRangeLimit,
			BatchLengthLimit:    DefaultBatchLengthLimit,
		},
	}

	return c
}

// PrevoteWaitDuration returns the amount of time to wait for straggler votes after receiving any +2/3 prevotes
func (cfg *ConsensusConfig) PrevoteWaitDuration(round int32) time.Duration {
	return time.Duration(
		cfg.TimeoutPrevote.Nanoseconds()+cfg.TimeoutPrevoteDelta.Nanoseconds()*int64(round),
	) * time.Nanosecond
}

// PrecommitWaitDuration returns the amount of time to wait for straggler votes after receiving any +2/3 precommits
func (cfg *ConsensusConfig) PrecommitWaitDuration(round int32) time.Duration {
	return time.Duration(
		cfg.TimeoutPrecommit.Nanoseconds()+cfg.TimeoutPrecommitDelta.Nanoseconds()*int64(round),
	) * time.Nanosecond
}
