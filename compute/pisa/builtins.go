package pisa

import (
	"crypto/sha256"
	"math/big"

	"github.com/holiman/uint256"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"
)

// BuiltinRunner is the executor function for a builtin executable
type BuiltinRunner func(*Engine, RegisterSet) (RegisterSet, *Exception)

// Builtin represents an executable of implementation code (golang)
// Implements the Runnable interface
type Builtin struct {
	// CallFields embeds the input and output
	// symbols for the Builtin calling interface.
	CallFields

	// Name represents the name of the builtin
	Name string
	// Ptr represents the pointer reference of the routine
	Ptr uint64

	Cost engineio.Fuel
	// Execute represents the execution function for the builtin.
	// It accepts set of inputs and returns some outputs along with an error code and message.
	runner BuiltinRunner
}

func (builtin Builtin) name() string { return builtin.Name }

func (builtin Builtin) callfields() CallFields { return builtin.CallFields }

func (builtin Builtin) ptr() engineio.ElementPtr { return engineio.ElementPtr(builtin.Ptr) }

func (builtin Builtin) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: "builtin",
		label: builtin.name(),
		point: uint64(builtin.ptr()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	// Exhaust fuel for operation
	if ok := engine.exhaustFuel(builtin.Cost); !ok {
		return nil, exception(FuelError, "fuel exhausted").traced(engine.callstack.trace())
	}

	return builtin.runner(engine, inputs)
}

// BuiltinMethod represents a method executable of implementation code (golang)
// Implements the Runnable & Method interfaces
type BuiltinMethod struct {
	// Builtin embeds all Builtin properties
	Builtin
	// Code represents the method code of the method
	Code MethodCode
	// Datatype represents the type that the method belongs to.
	Datatype Datatype
}

func (bmethod BuiltinMethod) code() MethodCode { return bmethod.Code }

func (bmethod BuiltinMethod) datatype() Datatype { return bmethod.Datatype }

func (bmethod BuiltinMethod) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: bmethod.datatype().String(),
		label: bmethod.name(),
		point: uint64(bmethod.code()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	// Exhaust fuel for operation
	if ok := engine.exhaustFuel(bmethod.Cost); !ok {
		return nil, exception(FuelError, "fuel exhausted").traced(engine.callstack.trace())
	}

	return bmethod.runner(engine, inputs)
}

func makeBuiltinMethod(
	name string, datatype Datatype, code MethodCode, cost uint64,
	inputs, outputs *TypeFields, runner BuiltinRunner,
) *BuiltinMethod {
	return &BuiltinMethod{
		Datatype: datatype, Code: code,
		Builtin: Builtin{
			Name: name, runner: runner, Cost: big.NewInt(int64(cost)),
			CallFields: CallFields{Inputs: inputs, Outputs: outputs},
		},
	}
}

func makeBuiltin(
	name string, ptr uint64, cost uint64,
	inputs, outputs *TypeFields, runner BuiltinRunner,
) *Builtin {
	return &Builtin{
		Name: name, Ptr: ptr, runner: runner, Cost: big.NewInt(int64(cost)),
		CallFields: CallFields{Inputs: inputs, Outputs: outputs},
	}
}

//nolint:forcetypeassert
func builtinSHA256() *Builtin {
	return makeBuiltin(
		"SHA256", 0, 50,
		makefields([]*TypeField{{"data", PrimitiveBytes}}),
		makefields([]*TypeField{{"hash", PrimitiveU256}}),
		func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
			data := inputs[0].(BytesValue)

			// Hash the data and create a u256 value from it
			hash := sha256.Sum256(data)
			u256 := new(uint256.Int).SetBytes(hash[:])

			return RegisterSet{0: &U256Value{u256}}, nil
		},
	)
}

//nolint:forcetypeassert
func builtinKeccak256() *Builtin {
	return makeBuiltin(
		"Keccak256", 1, 50,
		makefields([]*TypeField{{"data", PrimitiveBytes}}),
		makefields([]*TypeField{{"hash", PrimitiveU256}}),
		func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
			data := inputs[0].(BytesValue)

			// Hash the data and create a u256 value from it
			hash := sha3.Sum256(data)
			u256 := new(uint256.Int).SetBytes(hash[:])

			return RegisterSet{0: &U256Value{u256}}, nil
		},
	)
}

//nolint:forcetypeassert
func builtinBLAKE2b() *Builtin {
	return makeBuiltin(
		"BLAKE2b", 2, 50,
		makefields([]*TypeField{{"data", PrimitiveBytes}}),
		makefields([]*TypeField{{"hash", PrimitiveU256}}),
		func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
			data := inputs[0].(BytesValue)

			// Hash the data and create a u256 value from it
			hash := blake2b.Sum256(data)
			u256 := new(uint256.Int).SetBytes(hash[:])

			return RegisterSet{0: &U256Value{u256}}, nil
		},
	)
}

//nolint:forcetypeassert
func builtinSignatureVerify() *Builtin {
	return makeBuiltin(
		"SignatureVerify", 3, 50,
		makefields([]*TypeField{{"data", PrimitiveBytes}, {"signature", PrimitiveBytes}, {"pubkey", PrimitiveBytes}}),
		makefields([]*TypeField{{"ok", PrimitiveBool}}),
		func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
			data, sig, pubBytes := inputs[0].(BytesValue), inputs[1].(BytesValue), inputs[2].(BytesValue)

			if !crypto.ValidateSignature(sig) {
				return nil, exception(CallError, "insufficient length for signature")
			}

			ok, err := crypto.Verify(data, sig, pubBytes)
			if err != nil {
				return nil, exception(RuntimeError, err.Error())
			}

			return RegisterSet{0: BoolValue(ok)}, nil
		},
	)
}
