package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	"github.com/sarvalabs/moichain/mudra"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/moiclient"
	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common/hexutil"

	"github.com/sarvalabs/moichain/types"
)

const (
	DefaultBehaviouralCount = 1
	DefaultRandomCount      = 0
)

var ErrReadingPeerList = errors.New("error reading peer list file")

type Instance struct {
	KramaID      string `json:"krama_id"`
	RPCUrl       string `json:"rpc_url"`
	ConsensusKey string `json:"consensus_key"`
}

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
}

func DefaultBabylonConfig(path string) *Config {
	return &Config{
		Genesis:        path + "/genesis.json",
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    mudra.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr: []string{"/ip4/0.0.0.0/tcp/" + strconv.Itoa(common.DefaultListenerPort)},
			BootStrapPeers: []string{
				"/ip4/65.109.138.198/tcp/5000/p2p/16Uiu2HAmNPceqBKGNWXGTKTtWDPty4UhncdhB84VbDEPpn1H11Cb",
				"/ip4/135.181.206.93/tcp/5000/p2p/16Uiu2HAmFXiKHS3GWgdS1V36uUBDUjigf3RZRJCrjDFFMjexR3V8",
			},
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   common.DefaultInboundConnLimit,
			OutboundConnLimit:  common.DefaultOutboundConnLimit,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(common.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(common.DefaultSyncMode),
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
			DBFolder:    path + common.DefaultDBDirectory,
			MaxSnapSize: common.DefaultSnapSize, // 1GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Big(*common.DefaultFuelLimit),
		},
		Ixpool: IxPoolConfig{
			Mode:       common.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*common.DefaultIxPriceLimit),
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
		},
	}
}

func DefaultDevnetConfig(path string) *Config {
	return &Config{
		Genesis:        path + "/genesis.json",
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    mudra.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr:         []string{"/ip4/0.0.0.0/tcp/" + strconv.Itoa(common.DefaultListenerPort)},
			BootStrapPeers:     make([]string, 0),
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   common.DefaultInboundConnLimit,
			OutboundConnLimit:  common.DefaultOutboundConnLimit,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(common.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(common.DefaultSyncMode),
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
			DBFolder:    path + common.DefaultDBDirectory,
			MaxSnapSize: common.DefaultSnapSize, // 1GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Big(*common.DefaultFuelLimit),
		},
		Ixpool: IxPoolConfig{
			Mode:       common.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*common.DefaultIxPriceLimit),
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
		},
	}
}

type NetworkConfig struct {
	BootStrapPeers     []string   `json:"bootnodes"`
	TrustedPeers       []PeerInfo `json:"trusted_peers"`
	StaticPeers        []PeerInfo `json:"static_peers"`
	MaxPeers           uint       `json:"max_peers"`
	RelayNodeAddr      string     `json:"relay_node_addr"`
	Libp2pAddr         []string   `json:"libp2p_addr"`
	JSONRPCAddr        string     `json:"jsonrpc_addr"`
	MTQ                float64    `json:"mtq"`
	CorsAllowedOrigins []string   `json:"cors_allowed_origins"`
	NetworkSize        uint64     `json:"network_size"`
	NoDiscovery        bool       `json:"no_discovery"`
	RefreshSenatus     bool       `json:"refresh_senatus"`
	InboundConnLimit   int64      `json:"inbound_conn_limit"`
	OutboundConnLimit  int64      `json:"outbound_conn_limit"`
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
}

type DBConfig struct {
	DBFolder    string `json:"db_folder"`
	CleanDB     bool   `json:"clean_db"`
	MaxSnapSize uint64 `json:"max_snap_size"`
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
	AccountWaitTime       int   `json:"wait_time"`
	MessageDelay          int64 `json:"message_delay"`
	Precision             int64 `json:"precision"`
	OperatorSlots         int   `json:"operator_slots"`
	ValidatorSlots        int   `json:"validator_slots"`
}

type ExecutionConfig struct {
	FuelLimit hexutil.Big `json:"fuel_limit"`
}

type VaultConfig struct {
	DataDir      string
	NodePassword string
	SeedPhrase   string
	Mode         int8   // 0: Server, 1: Register/User mode
	NodeIndex    uint32 // Requires only in Register mode
	InMemory     bool
}

type PeerInfo struct {
	ID      string `json:"krama_id"`
	Address string `json:"address"`
}

type PeerList struct {
	TrustedPeers []PeerInfo `json:"trusted_peers"`
	StaticPeers  []PeerInfo `json:"static_peers"`
}

type Artifact struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`
}

func Err(err error) {
	if err != nil {
		fmt.Println("MOIPod failed Error occurred:", err)
		os.Exit(1)
	}
}

func WaitForReceipts(ctx context.Context, client *moiclient.Client, ixHash types.Hash) (*ptypes.RPCReceipt, error) {
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Failed to fetch receipt please try after some time IxHash %s \n", ixHash)

			return nil, ctx.Err()
		default:
			rpcReceipt, err := client.InteractionReceipt(&ptypes.ReceiptArgs{
				Hash: ixHash,
			})
			if err != nil {
				continue
			}

			return rpcReceipt, err
		}
	}
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

func ReadArtifactFile(path string) (*Artifact, error) {
	ar := new(Artifact)

	file, err := os.ReadFile(path)
	if err != nil {
		log.Println("path : ", path)

		return nil, errors.New("error reading artifact file")
	}

	if err = json.Unmarshal(file, ar); err != nil {
		return nil, errors.New("error unmarshalling into artifact")
	}

	return ar, nil
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

func ReadKramaIDsFromInstancesFile(path string) ([]string, error) {
	instances, err := readInstancesFile(path)
	if err != nil {
		return nil, err
	}

	kramaIDs := make([]string, len(instances))
	for i, instance := range instances {
		kramaIDs[i] = instance.KramaID
	}

	return kramaIDs, nil
}

// WriteToGenesisFile creates a new file if it doesn't exist, or replaces an existing one.
func WriteToGenesisFile(path string, genesis *types.GenesisFile) error {
	file, err := json.MarshalIndent(genesis, "", "\t")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(path, file, os.ModePerm); err != nil {
		return err
	}

	log.Println("Genesis file created or updated")

	return nil
}
