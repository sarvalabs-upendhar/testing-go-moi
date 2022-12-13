package common

import (
	"fmt"
	"math/big"
	"net"
	"time"

	kcrypto "github.com/sarvalabs/moichain/mudra"

	"github.com/libp2p/go-libp2p/core/protocol"
	maddr "github.com/multiformats/go-multiaddr"
)

var DefaultIxPriceLimit = big.NewInt(10)

type Config struct {
	NodeType       int
	KramaIDVersion int
	Vault          *kcrypto.VaultConfig
	Network        *NetworkConfig
	Chain          *ChainConfig
	Consensus      *ConsensusConfig
	DB             *DBConfig
	Execution      *ExecutionConfig
	IxPool         *IxPoolConfig
	Metrics        Telemetry
	LogFilePath    string
}

type Telemetry struct {
	PrometheusAddr *net.TCPAddr
	JaegerAddr     string
}

type ChainConfig struct {
	SkipGenesis   bool
	Genesis       string
	ShouldExecute bool
}

type DBConfig struct {
	DBFolderPath      string `yaml:"DBFolderPath"`
	CidPrefixVersion  uint64 `yaml:"cidPrefixVersion"`
	CidPrefixCodec    uint64 `yaml:"cidPrefixCodec"`
	CidPrefixMhType   uint64 `yaml:"cidPrefixMhType"`
	CidPrefixMhLength int    `yaml:"cidPrefixMhLength"`
}

type ExecutionConfig struct {
	FuelLimit uint64
}

type IxPoolConfig struct {
	Mode       int
	PriceLimit *big.Int
}

// NetworkConfig is the p2p configuration of the node
type NetworkConfig struct {
	BootstrapPeers  []maddr.Multiaddr
	MaxPeers        uint
	RelayNodeAddr   string
	ListenAddresses []maddr.Multiaddr
	ProtocolID      protocol.ID
	JSONRPCAddr     *net.TCPAddr
	MTQ             float64

	// this will be removed
	NetworkSize uint64
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
}

func DefaultConfig(path string) *Config {
	c := &Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: &kcrypto.VaultConfig{
			DataDir: path,
		},

		Network: &NetworkConfig{
			ListenAddresses: make([]maddr.Multiaddr, 0),
			BootstrapPeers:  make([]maddr.Multiaddr, 0),
			MaxPeers:        0, // current we don't limit the no.of peers
			ProtocolID:      protocol.ID("MOI"),
		},
		Chain: &ChainConfig{
			Genesis:       path + "/genesis.json",
			ShouldExecute: true,
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
			OperatorSlotCount:     2,
			ValidatorSlotCount:    3,
		},
		DB: &DBConfig{
			DBFolderPath:      path + "/db",
			CidPrefixVersion:  1,
			CidPrefixCodec:    0x50,
			CidPrefixMhType:   0xb220,
			CidPrefixMhLength: -1,
		},
		Execution: &ExecutionConfig{
			FuelLimit: 1000,
		},
		IxPool: &IxPoolConfig{
			Mode:       0,
			PriceLimit: DefaultIxPriceLimit,
		},
		Metrics: Telemetry{
			PrometheusAddr: nil,
		},
	}

	return c
}

// ResolveAddr resolves the passed in TCP address
func ResolveAddr(raw string) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", raw, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP("0.0.0.0")
	}

	return addr, nil
}
