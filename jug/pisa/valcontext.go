package pisa

import "github.com/sarvalabs/go-polo"

// LogicContextType is the Datatype for the builtin.LogicContext class.
var LogicContextType = LogicContextValue{}.Type()

// LogicContextValue is the BuiltinValue implementation for the builtin.LogicContext class.
type LogicContextValue struct {
	addr AddressValue
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
		addr: logic.addr.Copy().(AddressValue), //nolint:forcetypeassert
	}
}

func (logic LogicContextValue) Data() []byte {
	doc := make(polo.Document)
	doc.SetRaw("addr", logic.addr.Data())

	return doc.Bytes()
}

func (logic LogicContextValue) Norm() any {
	norm := make(map[string]any, 1)
	norm["addr"] = logic.addr.Norm()

	return norm
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
				return RegisterSet{0: logicCtx.addr}, nil
			},
		),
	}
}

// ParticipantContextType is the Datatype for the builtin.ParticipantContext class.
var ParticipantContextType = ParticipantContextValue{}.Type()

// ParticipantContextValue is the BuiltinValue implementation for the builtin.ParticipantContext class.
type ParticipantContextValue struct {
	addr AddressValue
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
	return LogicContextValue{
		addr: participant.addr.Copy().(AddressValue), //nolint:forcetypeassert
	}
}

func (participant ParticipantContextValue) Data() []byte {
	doc := make(polo.Document)
	doc.SetRaw("addr", participant.addr.Data())

	return doc.Bytes()
}

func (participant ParticipantContextValue) Norm() any {
	norm := make(map[string]any, 1)
	norm["addr"] = participant.addr.Norm()

	return norm
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
				return RegisterSet{0: participantCtx.addr}, nil
			},
		),
	}
}
