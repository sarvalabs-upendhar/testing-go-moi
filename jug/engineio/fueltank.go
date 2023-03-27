package engineio

import "sync"

// Fuel represents some execution effort points
type Fuel uint64

// FuelTank is a simple thread-safe bounded effort counter.
// The tank has some capacity (bound) which can be incrementally consumed until it is exhausted.
type FuelTank struct {
	*sync.Mutex
	Consumed, Capacity Fuel
}

// NewFuelTank generates a new FuelTank with the given capacity
func NewFuelTank(capacity Fuel) *FuelTank {
	return &FuelTank{Mutex: &sync.Mutex{}, Capacity: capacity}
}

// Level returns the current amount of unconsumed fuel in the tank
func (tank *FuelTank) Level() Fuel {
	return tank.Capacity - tank.Consumed
}

// Exhaust consumes the given amount of fuel from tank's capacity.
// Returns false if there isn't sufficient fuel to exhaust.
func (tank *FuelTank) Exhaust(fuel Fuel) bool {
	tank.Lock()
	defer tank.Unlock()

	if tank.Level() >= fuel {
		tank.Consumed += fuel

		return true
	}

	return false
}
