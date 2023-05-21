package common

import (
	"math/big"

	"github.com/sarvalabs/moichain/types"
)

var DefaultIxPriceLimit = big.NewInt(10)

// Network defaults
const (
	DefaultJSONRPCPort       = 1600
	DefaultPrometheusPort    = 30000
	DefaultInboundConnLimit  = 80
	DefaultOutboundConnLimit = 20
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
	DefaultDBDirectory = "/db"
	DefaultSnapSize    = 1024 * 1024 * 1024
)

// Execution defaults

const (
	DefaultFuelLimit = 1000
)

// Consensus defaults
