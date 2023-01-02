package cmd

import "math/big"

type Config struct {
	Genesis        string          `json:"genesis"`
	NodeType       int             `json:"node_type"`
	KramaIDVersion int             `json:"ḭd_version"`
	Vault          VaultConfig     `json:"vault"`
	Network        NetworkConfig   `json:"network"`
	Ixpool         IxPoolConfig    `json:"ixpool"`
	Consensus      ConsensusConfig `json:"consensus"`
	DB             DBConfig        `json:"database"`
	Telemetry      Telemetry       `json:"telemetry"`
	LogFilePath    string          `json:"logfile"`
}

type NetworkConfig struct {
	Libp2pAddr        []string `json:"libp2p_addr"`
	ProtocolID        string   `json:"protocol_id"`
	JSONRPCAddr       string   `json:"jsonrpc_addr"`
	BootStrapPeers    []string `json:"bootnodes"`
	InboundConnLimit  uint     `json:"inbound_conn_limit"`
	OutboundConnLimit uint     `json:"outbound_conn_limit"`
}

type IxPoolConfig struct {
	Mode       int      `json:"mode"`
	PriceLimit *big.Int `json:"price_limit"`
}

type DBConfig struct {
	DBFolder string `json:"db_folder"`
}

type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr"`
	JaegerAddr     string `json:"jaeger_addr"`
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
	MaxSlots              int   `json:"max_slots"`
	OperatorSlots         int   `json:"operator_slots"`
	ValidatorSlots        int   `json:"validator_slots"`
	AccountWaitTime       int   `json:"wait_time"`
}

type VaultConfig struct {
	DataDir       string
	MoiIDUsername string
	MoiIDPassword string
	MoiIDURL      string
	NodePassword  string
}
