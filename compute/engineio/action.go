package engineio

import "github.com/sarvalabs/go-polo"

type FuelGauge struct {
	Compute uint64
	Storage uint64
}

func NewFuelGauge(computeLimit, storageLimit uint64) *FuelGauge {
	return &FuelGauge{
		Compute: computeLimit,
		Storage: storageLimit,
	}
}

func (fg *FuelGauge) Consumed(newGuage FuelGauge) *FuelGauge {
	return &FuelGauge{
		Compute: fg.Compute - newGuage.Compute,
		Storage: fg.Storage - newGuage.Storage,
	}
}

func (fg *FuelGauge) Add(newGauge *FuelGauge) {
	if newGauge == nil {
		return
	}

	fg.Compute += newGauge.Compute
	fg.Storage += newGauge.Storage
}

type Action interface {
	Callsite() string
	Calldata() polo.Document
	Timestamp() uint64
	Identifier() [32]byte
	Origin() [32]byte
	Access(id [32]byte) (bool, error)
	AccessList() map[[32]byte]bool
	Parameters() map[string][]byte
}
