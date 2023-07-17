package pisa

import (
	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
)

var EnvironmentType = EnvironmentValue{}.Type()

// EnvironmentValue represents a RegisterValue that operates like a class struct
type EnvironmentValue struct {
	driver engineio.EnvDriver
}

func (env EnvironmentValue) Type() Datatype {
	return BuiltinDatatype{
		name: "Environment",
		fields: makefields([]*TypeField{
			{"timestamp", PrimitiveI64},
			{"fuelprice", PrimitiveU256},
		}),
	}
}

func (env EnvironmentValue) Copy() RegisterValue {
	return EnvironmentValue{
		driver: env.driver,
	}
}

func (env EnvironmentValue) Data() []byte {
	doc := make(polo.Document)

	polorizedTime, _ := polo.Polorize(env.driver.Timestamp())
	doc.SetRaw("timestamp", polorizedTime)

	polorizedFuel, _ := polo.Polorize(env.driver.FuelPrice())
	doc.SetRaw("fuelprice", polorizedFuel)

	return doc.Bytes()
}

func (env EnvironmentValue) Norm() any {
	return map[string]any{}
}

//nolint:forcetypeassert
func (env EnvironmentValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// EnvironmentValue.Time() -> int64
		0x10: makeBuiltinMethod(
			"Timestamp",
			EnvironmentType, 0x10, 10,
			makefields([]*TypeField{{"self", EnvironmentType}}),
			makefields([]*TypeField{{"result", PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				envObj := inputs[0].(EnvironmentValue)

				return RegisterSet{0: I64Value(envObj.driver.Timestamp())}, nil
			},
		),
		// EnvironmentValue.FuelPrice() -> int64
		0x11: makeBuiltinMethod(
			"FuelPrice",
			EnvironmentType, 0x11, 10,
			makefields([]*TypeField{{"self", EnvironmentType}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				envObj := inputs[0].(EnvironmentValue)

				val, overflow := uint256.FromBig(envObj.driver.FuelPrice())
				if overflow {
					return nil, exception(OverflowError, "conversion overflow error")
				}

				return RegisterSet{0: &U256Value{value: val}}, nil
			},
		),
	}
}
