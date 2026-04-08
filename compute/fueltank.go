package compute

// FuelTank is a simple thread-safe bounded effort counter.
// The tank has some capacity (bound) which can be incrementally consumed until it is exhausted.
type FuelTank struct {
	ComputeConsumed, ComputeCapacity, StorageConsumed, StorageCapacity uint64
}

// NewFuelTank generates a new FuelTank with the given capacity
func NewFuelTank(compute, storage uint64) *FuelTank {
	return &FuelTank{
		ComputeCapacity: compute,
		StorageCapacity: storage,
	}
}

// Level returns the current amount of unconsumed fuel in the tank
func (tank *FuelTank) Level() (uint64, uint64) {
	return tank.ComputeCapacity - tank.ComputeConsumed, tank.StorageCapacity - tank.StorageConsumed
}

// Exhaust consumes the given amount of fuel from tank's capacity.
// Returns false if there isn't sufficient fuel to exhaust.
func (tank *FuelTank) Exhaust(compute, storage uint64) bool {
	computeAvailable, storageAvailable := tank.Level()
	if computeAvailable < compute || storageAvailable < storage {
		tank.ComputeConsumed = tank.ComputeCapacity
		tank.StorageConsumed = tank.StorageCapacity

		return false
	}

	tank.ComputeConsumed += compute
	tank.StorageConsumed += storage

	return true
}

func (tank *FuelTank) Consumed() (uint64, uint64) {
	return tank.ComputeConsumed, tank.StorageConsumed
}
