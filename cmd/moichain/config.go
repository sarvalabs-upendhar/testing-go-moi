package main

import (
	"encoding/json"
	"errors"
	"math/big"
	"os"
)

var ErrReadingPeerList = errors.New("error reading peer list file")

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
	Libp2pAddr        []string   `json:"libp2p_addr"`
	JSONRPCAddr       string     `json:"jsonrpc_addr"`
	BootStrapPeers    []string   `json:"bootnodes"`
	TrustedPeers      []PeerInfo `json:"trusted_peers"`
	StaticPeers       []PeerInfo `json:"static_peers"`
	InboundConnLimit  int64      `json:"inbound_conn_limit"`
	OutboundConnLimit int64      `json:"outbound_conn_limit"`
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

type PeerInfo struct {
	ID      string `json:"krama_id"`
	Address string `json:"address"`
}

type PeerList struct {
	TrustedPeers []PeerInfo `json:"trusted_peers"`
	StaticPeers  []PeerInfo `json:"static_peers"`
}

// ReadPeerList reads the list of trusted and static peers from the given file and returns it.
func ReadPeerList(path string) (*PeerList, error) {
	if path == "" {
		return &PeerList{}, nil
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrReadingPeerList
	}

	peerList := new(PeerList)
	if err = json.Unmarshal(file, peerList); err != nil {
		return nil, ErrReadingPeerList
	}

	return peerList, nil
}
