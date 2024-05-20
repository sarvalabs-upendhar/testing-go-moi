package config

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/crypto"

	"github.com/sarvalabs/go-moi/common"
)

const DefaultFuelLimit uint64 = 10000

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
	LocalID                   = 111
	DevnetID                  = 112
	BabylonID                 = 113
)

// Chain defaults
const (
	DefaultGenesisTime = 1688741089 // time when the Babylon testnet started
)

// IxPool defaults
const (
	DefaultIxPoolMode     = 0
	DefaultMaxIXPoolSlots = 60000
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
