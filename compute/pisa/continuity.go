package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
)

// Continue represents an interface for managing execution continuity and flow.
// Each implementation of Continuity describes to the execution flow how to "continue"
// with the execution, containing any parameters required to modify the instruction flow.
type Continue interface {
	mode() continueMode
	fuel() engineio.Fuel
}

type continueMode int

const (
	continueModeOk continueMode = iota
	continueModeTerm
	continueModeJump
	continueModeExcept
)

// continueOk implements Continue and
// specifies successful progression
type continueOk struct {
	consumed uint64
}

func (ok continueOk) mode() continueMode  { return continueModeOk }
func (ok continueOk) fuel() engineio.Fuel { return engineio.NewFuel(ok.consumed) }

// continueTerm implements Continue and
// specifies successful termination
type continueTerm struct{}

func (term continueTerm) mode() continueMode  { return continueModeTerm }
func (term continueTerm) fuel() engineio.Fuel { return engineio.NewFuel(0) }

// continueJump implements Continue and
// specifies instruction jumping
type continueJump struct {
	consumed uint64
	jumpdest uint64
}

func (jump continueJump) mode() continueMode  { return continueModeJump }
func (jump continueJump) fuel() engineio.Fuel { return engineio.NewFuel(jump.consumed) }

// continueException implements Continue
// and specifies a raised exception
type continueException struct {
	consumed  uint64
	exception *Exception
}

func (except continueException) mode() continueMode  { return continueModeExcept }
func (except continueException) fuel() engineio.Fuel { return engineio.NewFuel(except.consumed) }

// withConsumption wraps the continueException object with some fuel consumption and returns it
func (except continueException) withConsumption(consumption uint64) continueException {
	except.consumed = consumption

	return except
}
