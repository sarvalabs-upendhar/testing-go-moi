package compute

import (
	"sync"
)

// FuelTank is a simple thread-safe bounded effort counter.
// The tank has some capacity (bound) which can be incrementally consumed until it is exhausted.
type FuelTank struct {
	*sync.Mutex
	Consumed, Capacity uint64
}

// NewFuelTank generates a new FuelTank with the given capacity
func NewFuelTank(capacity uint64) *FuelTank {
	return &FuelTank{
		Mutex:    &sync.Mutex{},
		Capacity: capacity,
		Consumed: 0,
	}
}

// Level returns the current amount of unconsumed fuel in the tank
func (tank *FuelTank) Level() uint64 {
	return tank.Capacity - tank.Consumed
}

// Exhaust consumes the given amount of fuel from tank's capacity.
// Returns false if there isn't sufficient fuel to exhaust.
func (tank *FuelTank) Exhaust(fuel uint64) bool {
	tank.Lock()
	defer tank.Unlock()

	if tank.Level() < fuel {
		return false
	}

	tank.Consumed += fuel

	return true
}
