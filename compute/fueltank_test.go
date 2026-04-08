package compute

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFuelTank(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10, 20)

	// Verify that the FuelTank was initialized with the correct capacity and consumed fuel
	require.Equal(t, uint64(0), tank.ComputeConsumed, "FuelTank should be initialized with 0 Compute Consumed")
	require.Equal(t, uint64(0), tank.StorageConsumed, " FuelTank should be initialized with 0 Storage Consumed")
	require.Equal(t, uint64(10), tank.ComputeCapacity)
	require.Equal(t, uint64(20), tank.StorageCapacity)
}

func TestFuelTank_Level(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10, 20)
	// Verify that the initial fuel level is equal to the capacity
	AreFuelTanksEqual(t, tank, 0, 0, 10, 20)

	// Exhaust some fuel from the tank and verify that the fuel level has decreased
	tank.Exhaust(5, 10)

	computeLevel, storageLevel := tank.Level()
	require.Equal(t, uint64(5), computeLevel, "Compute level does not match expected value")
	require.Equal(t, uint64(10), storageLevel, "Storage level does not match expected value")
}

func TestFuelTank_Exhaust(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10, 10)

	// Exhaust all the fuel from the tank and verify that the function returns true
	require.True(t, tank.Exhaust(10, 10), "FuelTank did not exhaust fuel below the capacity")
	// Attempt to exhaust more fuel from the tank than is available and verify that the function returns false
	require.False(t, tank.Exhaust(1, 1), "FuelTank allowed more fuel to be exhausted than was available")
	// Verify that the amount of consumed fuel has increased after fuel was exhausted
	AreFuelTanksEqual(t, tank, 10, 10, 10, 10)
}

func AreFuelTanksEqual(
	t *testing.T,
	a *FuelTank,
	computeConsumed,
	storageConsumed,
	computeCapacity,
	storageCapacity uint64,
) {
	t.Helper()
	require.Equal(t, a.ComputeConsumed, computeConsumed, "computeConsumed values are not equal")
	require.Equal(t, a.StorageConsumed, storageConsumed, "storageConsumed values are not equal")
	require.Equal(t, a.ComputeCapacity, computeCapacity, "computeCapacity values are not equal")
	require.Equal(t, a.StorageCapacity, storageCapacity, "storageCapacity values are not equal")
}
