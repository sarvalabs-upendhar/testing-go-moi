package engineio

import "math/big"

// Fuel represents some execution effort points
type Fuel = *big.Int

func NewFuel(fuel uint64) Fuel {
	return big.NewInt(int64(fuel))
}

// CallResult is the output emitted by Engine when making function calls.
// It contains the amount of fuel expended for the call along with either output value or returned error.
type CallResult struct {
	Consumed Fuel
	Outputs  []byte
	Error    []byte
}

// Ok returns whether the CallResult has some error data
func (result CallResult) Ok() bool {
	return result.Error == nil
}

// ErrorResult is an interface for errors emitted
// by an Engine within the CallResult object.
type ErrorResult interface {
	Engine() EngineKind
	String() string
	Bytes() []byte
}
