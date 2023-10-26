package common

import (
	"strconv"
	"time"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/crypto"
)

type Config struct {
	Genesis        string          `json:"genesis"`
	NodeType       int             `json:"node_type"`
	KramaIDVersion int             `json:"ḭd_version"`
	Vault          VaultConfig     `json:"vault"`
	Network        NetworkConfig   `json:"network"`
	Syncer         SyncerConfig    `json:"syncer"`
	Ixpool         IxPoolConfig    `json:"ixpool"`
	Consensus      ConsensusConfig `json:"consensus"`
	Execution      ExecutionConfig `json:"execution"`
	DB             DBConfig        `json:"database"`
	Telemetry      Telemetry       `json:"telemetry"`
	LogFilePath    string          `json:"logfile"`
	NetworkID      string          `json:"network_id"`
}

func DefaultBabylonConfig(path string) *Config {
	return &Config{
		Genesis:        path + "/genesis.json",
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    crypto.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(config.DefaultListenerPort),
				"/ip4/0.0.0.0/udp/" + strconv.Itoa(config.DefaultListenerPort) + "/quic-v1",
				"/ip6/::/tcp/" + strconv.Itoa(config.DefaultListenerPort),
				"/ip6/::/udp/" + strconv.Itoa(config.DefaultListenerPort) + "/quic-v1",
			},
			BootStrapPeers: []string{
				"/ip4/65.109.138.198/tcp/5000/p2p/16Uiu2HAmNPceqBKGNWXGTKTtWDPty4UhncdhB84VbDEPpn1H11Cb",
				"/ip4/135.181.206.93/tcp/5000/p2p/16Uiu2HAmFXiKHS3GWgdS1V36uUBDUjigf3RZRJCrjDFFMjexR3V8",
			},
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			DiscoveryInterval:  config.DefaultDiscoveryInterval,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(config.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
		},
		Consensus: ConsensusConfig{
			TimeoutPropose:        30000,
			TimeoutProposeDelta:   50000,
			TimeoutPrevote:        10000,
			TimeoutPrevoteDelta:   50000,
			TimeoutPrecommit:      10000,
			TimeoutPrecommitDelta: 50000,
			TimeoutCommit:         10000,
			Precision:             1000,
			MessageDelay:          5500,
			AccountWaitTime:       1500,
			OperatorSlots:         -1,
			ValidatorSlots:        5,
		},
		DB: DBConfig{
			CleanDB:     false,
			DBFolder:    path + config.DefaultDBDirectory,
			MaxSnapSize: config.DefaultSnapSize, // 1GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Uint64(config.DefaultFuelLimit),
		},
		Ixpool: IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
			OtlpAddress:    "",
			Token:          "",
		},
		LogFilePath: path + config.DefaultLogDirectory,
		NetworkID:   strconv.Itoa(config.BabylonID),
	}
}

func DefaultDevnetConfig(path string) *Config {
	return &Config{
		Genesis:        path + "/genesis.json",
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    crypto.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(config.DefaultListenerPort),
				"/ip4/0.0.0.0/udp/" + strconv.Itoa(config.DefaultListenerPort) + "/quic-v1",
				"/ip6/::/tcp/" + strconv.Itoa(config.DefaultListenerPort),
				"/ip6/::/udp/" + strconv.Itoa(config.DefaultListenerPort) + "/quic-v1",
			},
			BootStrapPeers:     make([]string, 0),
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			DiscoveryInterval:  config.DefaultDiscoveryInterval,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(config.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
		},
		Consensus: ConsensusConfig{
			TimeoutPropose:        30000,
			TimeoutProposeDelta:   50000,
			TimeoutPrevote:        10000,
			TimeoutPrevoteDelta:   50000,
			TimeoutPrecommit:      10000,
			TimeoutPrecommitDelta: 50000,
			TimeoutCommit:         10000,
			Precision:             1000,
			MessageDelay:          5500,
			AccountWaitTime:       1500,
			OperatorSlots:         -1,
			ValidatorSlots:        3,
		},
		DB: DBConfig{
			CleanDB:     false,
			DBFolder:    path + config.DefaultDBDirectory,
			MaxSnapSize: config.DefaultSnapSize, // 1GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Uint64(config.DefaultFuelLimit),
		},
		Ixpool: IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
			OtlpAddress:    "",
			Token:          "",
		},
		LogFilePath: path + config.DefaultLogDirectory,
		NetworkID:   strconv.Itoa(config.DevnetID),
	}
}

type NetworkConfig struct {
	BootStrapPeers     []string      `json:"bootnodes"`
	TrustedPeers       []PeerInfo    `json:"trusted_peers"`
	StaticPeers        []PeerInfo    `json:"static_peers"`
	MaxPeers           uint          `json:"max_peers"`
	RelayNodeAddr      string        `json:"relay_node_addr"`
	Libp2pAddr         []string      `json:"libp2p_addr"`
	PublicP2pAddr      []string      `json:"public_p2p_addr"`
	JSONRPCAddr        string        `json:"jsonrpc_addr"`
	MTQ                float64       `json:"mtq"`
	CorsAllowedOrigins []string      `json:"cors_allowed_origins"`
	NetworkSize        uint64        `json:"network_size"`
	NoDiscovery        bool          `json:"no_discovery"`
	RefreshSenatus     bool          `json:"refresh_senatus"`
	InboundConnLimit   int64         `json:"inbound_conn_limit"`
	OutboundConnLimit  int64         `json:"outbound_conn_limit"`
	DiscoveryInterval  time.Duration `json:"discovery_interval"`
}

type SyncerConfig struct {
	ShouldExecute  bool
	TrustedPeers   []string
	EnableSnapSync bool
	SyncMode       int
}

type IxPoolConfig struct {
	Mode       int         `json:"mode"`
	PriceLimit hexutil.Big `json:"price_limit"`
	MaxSlots   uint64      `json:"max_slots"`
}

type DBConfig struct {
	DBFolder    string `json:"db_folder"`
	CleanDB     bool   `json:"clean_db"`
	MaxSnapSize uint64 `json:"max_snap_size"`
}

type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr"`
	OtlpAddress    string `json:"otlp_addr"`
	Token          string `json:"token"`
}

type ConsensusConfig struct {
	TimeoutPropose        int64 `json:"timeout_propose"`
	TimeoutProposeDelta   int64 `json:"timeout_propose_delta"`
	TimeoutPrevote        int64 `json:"timeout_prevote"`
	TimeoutPrevoteDelta   int64 `json:"timeout_prevote_delta"`
	TimeoutPrecommit      int64 `json:"timeout_precommit"`
	TimeoutPrecommitDelta int64 `json:"timeout_precommit_delta"`
	TimeoutCommit         int64 `json:"timeout_commit"`
	SkipTimeoutCommit     bool  `json:"skip_timeout_commit"`
	AccountWaitTime       int   `json:"wait_time"`
	MessageDelay          int64 `json:"message_delay"`
	Precision             int64 `json:"precision"`
	OperatorSlots         int   `json:"operator_slots"`
	ValidatorSlots        int   `json:"validator_slots"`
	EnableDebugMode       bool  `json:"enable_debug_mode"`
}

type ExecutionConfig struct {
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`
}

type VaultConfig struct {
	DataDir      string
	NodePassword string
	SeedPhrase   string
	Mode         int8   // 0: Server, 1: Register/User mode
	NodeIndex    uint32 // Requires only in Register mode
	InMemory     bool
}
