package compute

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFuelTank(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10)

	// Verify that the FuelTank was initialized with the correct capacity and consumed fuel
	require.Equal(t, uint64(0), tank.Consumed, "FuelTank was not initialized with 0 consumed fuel")
	require.Equal(t, uint64(10), tank.Capacity, "FuelTank was not initialized with the correct capacity")
}

func TestFuelTank_Level(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10)
	// Verify that the initial fuel level is equal to the capacity
	require.Equal(t, tank.Capacity, tank.Level(), "Fuel level is not equal to capacity")

	// Exhaust some fuel from the tank and verify that the fuel level has decreased
	tank.Exhaust(5)
	require.Equal(t,
		tank.Capacity-5, tank.Level(),
		"Fuel level did not decrease after fuel was exhausted",
	)
}

func TestFuelTank_Exhaust(t *testing.T) {
	// Initialize a new FuelTank with a capacity of 10
	tank := NewFuelTank(10)

	// Exhaust all the fuel from the tank and verify that the function returns true
	require.True(t, tank.Exhaust(10), "FuelTank did not exhaust all of its fuel")
	// Attempt to exhaust more fuel from the tank than is available and verify that the function returns false
	require.False(t, tank.Exhaust(1), "FuelTank allowed more fuel to be exhausted than was available")
	// Verify that the amount of consumed fuel has increased after fuel was exhausted
	require.Equal(t, tank.Capacity, tank.Consumed, "Consumed fuel was not updated after fuel was exhausted")
}
