package config

import (
	"math/big"
	"net/netip"
	"strconv"
	"time"

	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/x/rate"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/sarvalabs/go-moi/common"
)

const (
	DefaultComputeFuelLimit uint64 = 10000
	DefaultStorageFuelLimit uint64 = 1000000 // TODO: Verify this value
)

var DefaultIxPriceLimit = big.NewInt(1)

const (
	DefaultMoiWalletPath = crypto.DefaultMOIWalletPath
	DefaultMOIIDPath     = crypto.DefaultMOIIDPath
)

// Network defaults
const (
	DefaultP2PPort            = 6000
	DefaultJSONRPCPort        = 1600
	DefaultPrometheusPort     = 30000
	DefaultInboundConnLimit   = 60
	DefaultOutboundConnLimit  = 25
	DefaultMinimumConnections = 100
	DefaultMaximumConnections = 200
	DefaultWatchDogURL        = "https://babylon-watchdog.moi.techbology/add"
	DefaultICSRequestTimeout  = 800 * time.Millisecond
	DefaultDiscoveryInterval  = 60 * time.Second
)

// Chain defaults
const (
	DefaultGenesisTime  = 1688741089 // time when the Babylon testnet started
	DefaultGenesisSeed  = "30df386a7dd9eb09f2522d6d299f68e1fd657e75ba4fc3163064d9c995ab9626"
	DefaultGenesisProof = "06eab156cf8ecf0d77fcc24c2e065e5a13180718e3022275e395b2fa22c6b4c5c83ac787f8622e6af" +
		"2a1651216ff547303726987af751cff7dc38e4a4c63e26063bf12ff2e89e5930f78c246ca89f69ec6cb8303c694d02803" +
		"f253b23dcd476319e1094df5a8c926ff65a52a6e47059762ebc34c0e825698bffc397b8bbf58878e0ce8e3f578ed8f85870be" +
		"9a2ab97a116dce3eb2abb8320df9b09ba365d2fc66cb9df6e36f2f27284a70f87ef71d4722abd0aee46d025e726a3f1a1ade285ea"
)

// IxPool defaults
const (
	DefaultIxPoolMode              = 0
	DefaultMaxIXPoolSlots          = 60000
	DefaultIxIncomingFilterMaxSize = 1000
	DefaultMaxIxGroupSize          = 100
)

// Syncer defaults
const (
	DefaultSyncMode = common.FullSync
)

// DB defaults
const (
	DefaultDBDirectory  = "/db"
	DefaultLogDirectory = "/log"
	DefaultSnapSize     = 1024 * 1024 * 1024 * 6
)

// Subscription defaults
const (
	// DefaultTesseractRangeLimit is the maximum tesseract range allowed for json_rpc
	// requests with fromTesseract/toTesseract values (e.g. moi_getLogs)
	DefaultTesseractRangeLimit = 10

	// DefaultBatchLengthLimit is the maximum length allowed for json_rpc batch requests
	DefaultBatchLengthLimit = 20
)

// Tree defaults
const (
	DefaultTreeCacheSize = 1024 * 1024 * 200
)

type NetworkID int

const (
	Local NetworkID = iota + 111
	Devnet
	Babylon
)

func (n NetworkID) String() string {
	return strconv.Itoa(int(n))
}

func (n NetworkID) IsTestnet() bool {
	return n != Local && n != Devnet
}

var (
	DevnetMaxConcurrentConns = 20

	DevnetIPv4SubnetLimits = []rate.SubnetLimit{
		{
			PrefixLength: 32,
			Limit:        rate.Limit{RPS: 0.2, Burst: 2 * DevnetMaxConcurrentConns},
		},
	}

	DevnetIPv6SubnetLimits = []rate.SubnetLimit{
		{
			PrefixLength: 56,
			Limit:        rate.Limit{RPS: 0.2, Burst: 2 * DevnetMaxConcurrentConns},
		},
		{
			PrefixLength: 48,
			Limit:        rate.Limit{RPS: 0.5, Burst: 10 * DevnetMaxConcurrentConns},
		},
	}

	// DevnetNetworkPrefixLimits ensure that all connections on localhost always succeed
	DevnetNetworkPrefixLimits = []rate.PrefixLimit{
		{
			Prefix: netip.MustParsePrefix("127.0.0.0/8"),
			Limit:  rate.Limit{},
		},
		{
			Prefix: netip.MustParsePrefix("::1/128"),
			Limit:  rate.Limit{},
		},
	}

	DevnetRateLimiter = &rate.Limiter{
		NetworkPrefixLimits: DevnetNetworkPrefixLimits,
		GlobalLimit:         rate.Limit{},
		SubnetRateLimiter: rate.SubnetLimiter{
			IPv4SubnetLimits: DevnetIPv4SubnetLimits,
			IPv6SubnetLimits: DevnetIPv6SubnetLimits,
			GracePeriod:      1 * time.Minute,
		},
	}

	DevnetIpv4ConnLimitPerSubnet = []rcmgr.ConnLimitPerSubnet{
		{
			PrefixLength: 32,
			ConnCount:    60,
		},
	}
)
