package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
)

var EnvironmentType = EnvironmentValue{}.Type()

// EnvironmentValue represents a RegisterValue that operates like a class struct
type EnvironmentValue struct {
	driver engineio.EnvDriver
}

func (env EnvironmentValue) Type() Datatype {
	return BuiltinDatatype{
		name:   "Environment",
		fields: makefields([]*TypeField{}),
	}
}

func (env EnvironmentValue) Copy() RegisterValue {
	return EnvironmentValue{
		driver: env.driver,
	}
}

func (env EnvironmentValue) Data() []byte {
	return []byte{}
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

		// EnvironmentValue.ClusterID() -> string
		0x11: makeBuiltinMethod(
			"ClusterID",
			EnvironmentType, 0x11, 10,
			makefields([]*TypeField{{"self", EnvironmentType}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				envObj := inputs[0].(EnvironmentValue)

				return RegisterSet{0: StringValue(envObj.driver.ClusterID())}, nil
			},
		),
	}
}
