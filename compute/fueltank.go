package compute

import (
	"math/big"
	"sync"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

// FuelTank is a simple thread-safe bounded effort counter.
// The tank has some capacity (bound) which can be incrementally consumed until it is exhausted.
type FuelTank struct {
	*sync.Mutex
	Consumed, Capacity engineio.Fuel
}

// NewFuelTank generates a new FuelTank with the given capacity
func NewFuelTank(capacity engineio.Fuel) *FuelTank {
	return &FuelTank{
		Mutex:    &sync.Mutex{},
		Capacity: capacity,
		Consumed: engineio.NewFuel(0),
	}
}

// Level returns the current amount of unconsumed fuel in the tank
func (tank *FuelTank) Level() engineio.Fuel {
	return new(big.Int).Sub(tank.Capacity, tank.Consumed)
}

// Exhaust consumes the given amount of fuel from tank's capacity.
// Returns false if there isn't sufficient fuel to exhaust.
func (tank *FuelTank) Exhaust(fuel engineio.Fuel) bool {
	tank.Lock()
	defer tank.Unlock()

	if tank.Level().Cmp(fuel) >= 0 {
		tank.Consumed = new(big.Int).Add(tank.Consumed, fuel)

		return true
	}

	return false
}
