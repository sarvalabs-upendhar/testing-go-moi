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
	DefaultListenerPort      = 6000
	DefaultJSONRPCPort       = 1600
	DefaultPrometheusPort    = 30000
	DefaultInboundConnLimit  = 50
	DefaultOutboundConnLimit = 15
	DefaultWatchDogURL       = "https://babylon-watchdog.moi.techbology/add"
	DefaultICSRequestTimeout = 2500 * time.Millisecond
	DefaultDiscoveryInterval = 60 * time.Second
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
