package jug

import (
	"github.com/sarvalabs/moichain/jug/engineio"
)

var (
	FuelSimpleValueTransfer   = engineio.NewFuel(100)
	FuelAssetCreation         = engineio.NewFuel(100)
	FuelLogicObjectDeployment = engineio.NewFuel(100)
	FuelSimpleAssetMint       = engineio.NewFuel(100)
)
