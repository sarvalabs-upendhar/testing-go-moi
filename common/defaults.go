package common

import (
	"math/big"

	"github.com/sarvalabs/moichain/mudra"

	"github.com/sarvalabs/moichain/types"
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
)

// IxPool defaults

const (
	DefaultIxPoolMode = 0
)

// Syncer defaults

const (
	DefaultSyncMode = types.FullSync
)

// DB defaults

const (
	DefaultDBDirectory  = "/db"
	DefaultLogDirectory = "/log"
	DefaultSnapSize     = 1024 * 1024 * 1024
)

const (
	DefaultMoiWalletPath = mudra.DefaultMOIWalletPath
	DefaultMOIIDPath     = mudra.DefaultMOIIDPath
)
