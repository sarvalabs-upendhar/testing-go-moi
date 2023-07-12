package compute

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
)

var (
	FuelSimpleValueTransfer   = engineio.NewFuel(100)
	FuelAssetCreation         = engineio.NewFuel(100)
	FuelLogicObjectDeployment = engineio.NewFuel(100)
	FuelSimpleAssetMint       = engineio.NewFuel(100)
)
