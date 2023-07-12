package config

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
)

var (
	DefaultFuelLimit    = big.NewInt(5000)
	DefaultIxPriceLimit = big.NewInt(1)
)

// Network defaults
const (
	DefaultListenerPort      = 6000
	DefaultJSONRPCPort       = 1600
	DefaultPrometheusPort    = 30000
	DefaultInboundConnLimit  = 80
	DefaultOutboundConnLimit = 20
	DefaultWatchDogURL       = "https://babylon-watchdog.moi.techbology/add"
	DefaultMaxIXPoolSlots    = 60000
)

// IxPool defaults

const (
	DefaultIxPoolMode = 0
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
