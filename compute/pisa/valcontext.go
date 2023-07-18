package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
)

// LogicContextType is the Datatype for the builtin.LogicContext class.
var LogicContextType = LogicContextValue{}.Type()

// LogicContextValue is the BuiltinValue implementation for the builtin.LogicContext class.
type LogicContextValue struct {
	ctx engineio.CtxDriver
}

func (logic LogicContextValue) Size() U64Value {
	return 1
}

func (logic LogicContextValue) Get(u uint8) (RegisterValue, *Exception) {
	if u > uint8(logic.Size()) {
		return nil, exception(AccessError, "field out of bounds")
	}

	switch u {
	case 0:
		return AddressValue(logic.ctx.Address()), nil
	default:
		return nil, exception(AccessError, "inaccessible field slot")
	}
}

func (logic LogicContextValue) Set(u uint8, value RegisterValue) *Exception {
	return exception(AccessError, "unsettable field slot")
}

func (logic LogicContextValue) Type() Datatype {
	return BuiltinDatatype{
		name: "LogicContext",
		fields: makefields([]*TypeField{
			{"addr", PrimitiveAddress},
		}),
	}
}

func (logic LogicContextValue) Copy() RegisterValue {
	return LogicContextValue{
		ctx: logic.ctx,
	}
}

func (logic LogicContextValue) Data() []byte {
	doc := make(polo.Document)
	doc.SetRaw("addr", AddressValue(logic.ctx.Address()).Data())

	return doc.Bytes()
}

func (logic LogicContextValue) Norm() any {
	return map[string]any{
		"addr": logic.ctx.Address(),
	}
}

//nolint:forcetypeassert
func (logic LogicContextValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// LogicContext.__addr__() -> address
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			LogicContextType, MethodAddr, 10,
			makefields([]*TypeField{{"self", LogicContextType}}),
			makefields([]*TypeField{{"result", PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Cast the first input into a LogicContextValue
				logicCtx := inputs[0].(LogicContextValue)
				// Return the address field of the LogicContext
				return RegisterSet{0: AddressValue(logicCtx.ctx.Address())}, nil
			},
		),
	}
}

// ParticipantContextType is the Datatype for the builtin.ParticipantContext class.
var ParticipantContextType = ParticipantContextValue{}.Type()

// ParticipantContextValue is the BuiltinValue implementation for the builtin.ParticipantContext class.
type ParticipantContextValue struct {
	ctx engineio.CtxDriver
}

func (participant ParticipantContextValue) Size() U64Value {
	return 1
}

func (participant ParticipantContextValue) Get(u uint8) (RegisterValue, *Exception) {
	if u > uint8(participant.Size()) {
		return nil, exception(AccessError, "field out of bounds")
	}

	switch u {
	case 0:
		return AddressValue(participant.ctx.Address()), nil
	default:
		return nil, exception(AccessError, "inaccessible field slot")
	}
}

func (participant ParticipantContextValue) Set(u uint8, value RegisterValue) *Exception {
	return exception(AccessError, "unsettable field slot")
}

func (participant ParticipantContextValue) Type() Datatype {
	return BuiltinDatatype{
		name: "ParticipantContext",
		fields: makefields([]*TypeField{
			{"addr", PrimitiveAddress},
		}),
	}
}

func (participant ParticipantContextValue) Copy() RegisterValue {
	return ParticipantContextValue{
		ctx: participant.ctx,
	}
}

func (participant ParticipantContextValue) Data() []byte {
	doc := make(polo.Document)
	doc.SetRaw("addr", AddressValue(participant.ctx.Address()).Data())

	return doc.Bytes()
}

func (participant ParticipantContextValue) Norm() any {
	return map[string]any{
		"addr": participant.ctx.Address(),
	}
}

//nolint:forcetypeassert
func (participant ParticipantContextValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// ParticipantContext.__addr__() -> address
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			ParticipantContextType, MethodAddr, 10,
			makefields([]*TypeField{{"self", ParticipantContextType}}),
			makefields([]*TypeField{{"result", PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Cast the first input into a ParticipantContextValue
				participantCtx := inputs[0].(ParticipantContextValue)
				// Return the address field of the LogicContext
				return RegisterSet{0: AddressValue(participantCtx.ctx.Address())}, nil
			},
		),
	}
}
