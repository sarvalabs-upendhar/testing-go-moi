package config

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
)

const DefaultFuelLimit uint64 = 10000

var DefaultIxPriceLimit = big.NewInt(1)

// Network defaults
const (
	DefaultListenerPort       = 6000
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
	DefaultSnapSize     = 1024 * 1024 * 1024
)

const (
	DefaultMoiWalletPath = crypto.DefaultMOIWalletPath
	DefaultMOIIDPath     = crypto.DefaultMOIIDPath
)

// Subscription defaults
const (
	DefaultTesseractRangeLimit = 10
)
