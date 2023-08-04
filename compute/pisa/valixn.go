package pisa

import (
	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

var InteractionType = InteractionValue{}.Type()

// InteractionValue represents a RegisterValue that operates like a class struct
type InteractionValue struct {
	driver engineio.IxnDriver
}

func (ixn InteractionValue) Type() Datatype {
	return BuiltinDatatype{
		name:   "Interaction",
		fields: makefields([]*TypeField{}),
	}
}

func (ixn InteractionValue) Copy() RegisterValue {
	return InteractionValue{
		driver: ixn.driver,
	}
}

func (ixn InteractionValue) Data() []byte {
	return []byte{}
}

func (ixn InteractionValue) Norm() any {
	return map[string]any{}
}

//nolint:forcetypeassert
func (ixn InteractionValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// InteractionValue.FuelPrice() -> uint256
		0x10: makeBuiltinMethod(
			"FuelPrice",
			InteractionType, 0x10, 10,
			makefields([]*TypeField{{"self", InteractionType}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				ixnObj := inputs[0].(InteractionValue)

				val, overflow := uint256.FromBig(ixnObj.driver.FuelPrice())
				if overflow {
					return nil, exception(OverflowError, "conversion overflow error")
				}

				return RegisterSet{0: &U256Value{value: val}}, nil
			},
		),
		// InteractionValue.FuelLimit() -> uint256
		0x11: makeBuiltinMethod(
			"FuelLimit",
			InteractionType, 0x11, 10,
			makefields([]*TypeField{{"self", InteractionType}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				ixnObj := inputs[0].(InteractionValue)

				val, overflow := uint256.FromBig(ixnObj.driver.FuelLimit())
				if overflow {
					return nil, exception(OverflowError, "conversion overflow error")
				}

				return RegisterSet{0: &U256Value{value: val}}, nil
			},
		),
		// InteractionValue.IxnType() -> string
		0x12: makeBuiltinMethod(
			"IxnType",
			InteractionType, 0x12, 10,
			makefields([]*TypeField{{"self", InteractionType}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				ixnObj := inputs[0].(InteractionValue)

				return RegisterSet{0: StringValue(ixnObj.driver.Type().String())}, nil
			},
		),
	}
}
