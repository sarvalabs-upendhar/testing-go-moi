package pisa

import (
	"fmt"
	"strings"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// Instruction represent a single logical instruction
// to execute with an opcode and the arguments for it.
type Instruction struct {
	Op   OpCode
	Args []byte
}

func (instruct Instruction) String() string {
	var str strings.Builder

	str.WriteString(instruct.Op.String())

	for _, arg := range instruct.Args {
		str.WriteString(fmt.Sprintf(" %#x", arg))
	}

	return str.String()
}

type (
	// Instructions represents a set of instruction objects.
	Instructions []Instruction

	// InstructionFunc represents
	InstructionFunc func(*callscope, []byte) Continue

	// InstructionSet represents the opcode instructions for the PISA Runtime.
	// A total of 256 opcodes are supported with each opcode associated with an executor function.
	InstructionSet [256]InstructionFunc
)

func (instructs Instructions) Len() uint64 {
	return uint64(len(instructs))
}

// BaseInstructionSet returns an InstructionSet with all the base opcodes and their instructions initialized.
func BaseInstructionSet() InstructionSet {
	return InstructionSet{
		TERM:   opTERM,
		DEST:   opDEST,
		JUMP:   opJUMP,
		JUMPI:  opJUMPI,
		OBTAIN: opOBTAIN,
		YIELD:  opYIELD,

		// CARGS: opCARGS,
		// CALLB:  opCALLB,
		// CALLR:  opCALLR,
		// CALLM:  opCALLM,

		CONST:  opCONST,
		LDPTR1: opLDPTR,
		LDPTR2: opLDPTR,
		LDPTR3: opLDPTR,
		LDPTR4: opLDPTR,
		LDPTR5: opLDPTR,
		LDPTR6: opLDPTR,
		LDPTR7: opLDPTR,
		LDPTR8: opLDPTR,

		ISNULL: opISNULL,
		ZERO:   opZERO,
		CLEAR:  opCLEAR,
		SAME:   opSAME,
		COPY:   opCOPY,
		SWAP:   opSWAP,
		// SERIAL: opSERIAL,
		// DESERIAL: opDESERIAL,

		MAKE:  opMAKE,
		PMAKE: opPMAKE,
		// VMAKE: opVMAKE,
		// BMAKE: opBMAKE,

		// BUILD: opBUILD,
		THROW: opTHROW,
		// EMIT: opEMIT,
		// JOIN: opJOIN,

		LT: opLT,
		GT: opGT,
		EQ: opEQ,

		BOOL: opBOOL,
		STR:  opSTR,
		// ADDR: opADDR,
		// LEN: opLEN,

		// SIZEOF: opSIZEOF,
		GETFLD: opGETFLD,
		SETFLD: opSETFLD,
		GETIDX: opGETIDX,
		SETIDX: opSETIDX,

		// GROW: opGROW,
		// SLICE: opSLICE,
		// APPEND: opAPPEND,
		// POPEND: opPOPEND,
		// DELKEY: opDELKEY,

		// AND: opAND,
		// OR: opOR,
		NOT: opNOT,

		// INCR: opINCR,
		// DECR: opDECR,

		ADD: opADD,
		SUB: opSUB,
		MUL: opMUL,
		DIV: opDIV,
		// MOD: opMOD,

		// BXOR: opBXOR,
		// BAND: opBAND,
		// BOR: opBOR,
		// BNOT: opBNOT,

		// LOGIC: opLOGIC,
		// SENDER: opSENDER,

		PLOAD: opPLOAD,
		PSAVE: opPSAVE,
	}
}

func opTERM(_ *callscope, _ []byte) Continue {
	return continueTerm{}
}

func opDEST(_ *callscope, _ []byte) Continue {
	return continueOk{1}
}

func opJUMP(scope *callscope, operands []byte) Continue {
	// JUMP [$X: ptr]
	destination := operands[0]

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(destination)
	if except != nil {
		return raise(except)
	}

	return continueJump{10, uint64(pointer)}
}

func opJUMPI(scope *callscope, operands []byte) Continue {
	// JUMPI [$X: ptr][$Y: __bool__]
	destination, condition := operands[0], operands[1]

	// Retrieve the condition register
	regCondition := scope.memory.Get(condition)
	// Call the __bool__ method of register
	result, except := scope.callMethodBool(regCondition)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	// If condition is false, no jump
	if !result {
		return continueOk{10}
	}

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(destination)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	return continueJump{20, uint64(pointer)}
}

func opOBTAIN(scope *callscope, operands []byte) Continue {
	// OBTAIN [$X][&Y]
	reg, slot := operands[0], operands[1]

	// Retrieve the calldata value
	val := scope.inputs.Get(slot)
	// Set the register value
	scope.memory.Set(reg, val)

	return continueOk{5}
}

func opYIELD(scope *callscope, operands []byte) Continue {
	// YIELD [$X][&Y]
	reg, slot := operands[0], operands[1]

	// Retrieve the register
	value := scope.memory.Get(reg)
	// Set the return slot value
	scope.outputs.Set(slot, value)

	return continueOk{5}
}

func opCONST(scope *callscope, operands []byte) Continue {
	// CONST [$X][$Y: ptr]
	out, reg := operands[0], operands[1]

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(reg)
	if except != nil {
		return raise(except)
	}

	// Get the constant from the environment
	constant, err := scope.engine.GetConstant(engineio.ElementPtr(pointer))
	if err != nil {
		return raise(scope.exceptionf(ReferenceError, "constant %#v not found: %v", pointer, err))
	}

	// Create value from the constant definition
	constVal, err := constant.Value()
	if err != nil {
		return raise(scope.exception(ValueError, err.Error()))
	}

	// Set the constant value into the register
	scope.memory.Set(out, constVal)

	return continueOk{20}
}

func opLDPTR(scope *callscope, operands []byte) Continue {
	// LDPTR[1..8] [$X: ptr][0x00]
	target, ptr := operands[0], operands[1:]

	// Decipher constant ID into 64-bit address
	value, err := ptrdecode(ptr)
	if err != nil {
		return raise(scope.exceptionf(OverflowError, "pointer value: %v", ptr))
	}

	// Set the register value
	scope.memory.Set(target, PtrValue(value))

	return continueOk{10 + engineio.Fuel(len(ptr))}
}

func opISNULL(scope *callscope, operands []byte) Continue {
	// ISNULL [$X: bool][$Y]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)
	// Set isnull to true if register has nil value or has type Null
	isnull := BoolValue(IsNullValue(regVal))
	// Set the register
	scope.memory.Set(out, isnull)

	return continueOk{5}
}

func opZERO(scope *callscope, operands []byte) Continue {
	// ZERO [$X]
	reg := operands[0]

	// Retrieve the register
	regVal := scope.memory.Get(reg)
	// Create a new value for register type with zero data
	newVal, err := NewRegisterValue(regVal.Type(), nil)
	if err != nil {
		return raise(scope.exception(ValueError, err.Error()))
	}

	// Set the new value to the zero value
	scope.memory.Set(reg, newVal)

	return continueOk{5}
}

func opCLEAR(scope *callscope, operands []byte) Continue {
	// CLEAR [$X]
	reg := operands[0]

	// Unset the register
	scope.memory.Unset(reg)

	return continueOk{5}
}

func opSAME(scope *callscope, operands []byte) Continue {
	// SAME [$X: bool][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Retrieve the register a & b
	regA, regB := scope.memory.Get(a), scope.memory.Get(b)
	// Check if the type of both registers is the same
	same := regA.Type().Equals(regB.Type())
	// Set the output as false
	scope.memory.Set(out, BoolValue(same))

	return continueOk{5}
}

func opCOPY(scope *callscope, operands []byte) Continue {
	// COPY [$X][$Y]
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	reg := scope.memory.Get(source)
	// Set a copy of the register to the destination
	scope.memory.Set(destination, reg.Copy())

	return continueOk{5}
}

func opSWAP(scope *callscope, operands []byte) Continue {
	// SWAP [$X][$Y]
	a, b := operands[0], operands[1]

	// Retrieve the register a & b
	regA, regB := scope.memory.Get(a), scope.memory.Get(b)
	// Swap the register values
	scope.memory.Set(b, regA.Copy())
	scope.memory.Set(a, regB.Copy())

	return continueOk{5}
}

func opMAKE(scope *callscope, operands []byte) Continue {
	// MAKE [$X][$Y: ptr]
	out, reg := operands[0], operands[1]

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(reg)
	if except != nil {
		return raise(except)
	}

	typedef, err := scope.engine.GetTypedef(engineio.ElementPtr(pointer))
	if err != nil {
		return raise(scope.exceptionf(ReferenceError, "typedef %#v not found: %v", pointer, err))
	}

	// Create a new default value for the typedef
	typeval, _ := NewRegisterValue(typedef, nil)
	// Set the constant value into the register
	scope.memory.Set(out, typeval)

	return continueOk{20}
}

func opPMAKE(scope *callscope, operands []byte) Continue {
	// PMAKE [$X][Y: 0x00]
	out, typeID := operands[0], operands[1]

	// Check if type ID is valid
	if typeID > MaxPrimitive {
		return raise(scope.exceptionf(TypeError, "invalid type ID: %v", typeID))
	}

	// Create a datatype from the type ID
	dt := Primitive(typeID).Datatype()
	// Create a value for the datatype
	value, _ := NewRegisterValue(dt, nil)

	// Set the register value
	scope.memory.Set(out, value)

	return continueOk{10}
}

func opTHROW(scope *callscope, operands []byte) Continue {
	// THROW [$X: __throw__]
	reg := operands[0]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __throw__ method of register
	errdata, except := scope.callMethodThrow(regVal)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	// Create the custom exception object
	except = scope.exception(CustomExceptionClass{regVal.Type()}, string(errdata))

	return raise(except).withConsumption(30)
}

func opLT(scope *callscope, operands []byte) Continue {
	// LT [$X: bool][$Y: __lt__][$Z: __lt__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	// Retrieve the __lt__ method for the register type
	methodLT, ok := scope.engine.lookupMethod(regA.Type(), MethodLt)
	if !ok {
		return raise(scope.exceptionf(NotImplementedError, "%v does not implement __lt__", regA.Type()))
	}

	// Execute the __lt__ method
	outputs, except := scope.engine.run(methodLT, RegisterSet{0: regA, 1: regB})
	if except != nil {
		return raise(except.Wrap(scope.engine.callstack.head().String()))
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opGT(scope *callscope, operands []byte) Continue {
	// GT [$X: bool][$Y: __gt__][$Z: __gt__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	// Retrieve the __gt__ method for the register type
	methodGT, ok := scope.engine.lookupMethod(regA.Type(), MethodGt)
	if !ok {
		return raise(scope.exceptionf(NotImplementedError, "%v does not implement __gt__", regA.Type()))
	}

	// Execute the __gt__ method
	outputs, except := scope.engine.run(methodGT, RegisterSet{0: regA, 1: regB})
	if except != nil {
		return raise(except.Wrap(scope.engine.callstack.head().String()))
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opEQ(scope *callscope, operands []byte) Continue {
	// EQ [$X: bool][$Y: __eq__][$Z: __eq__]
	a, b := operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	// Retrieve the __eq__ method for the register type
	methodEQ, ok := scope.engine.lookupMethod(regA.Type(), MethodEq)
	if !ok {
		return raise(scope.exceptionf(NotImplementedError, "%v does not implement __eq__", regA.Type()))
	}

	// Execute the __eq__ method
	outputs, except := scope.engine.run(methodEQ, RegisterSet{0: regA, 1: regB})
	if except != nil {
		return raise(except.Wrap(scope.engine.callstack.head().String()))
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.memory.Set(operands[0], result)

	return continueOk{20}
}

func opBOOL(scope *callscope, operands []byte) Continue {
	// BOOL [$X: bool][$Y: __bool__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __bool__ method of register
	result, except := scope.callMethodBool(regVal)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opSTR(scope *callscope, operands []byte) Continue {
	// STR [$X: string][$Y: __str__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __str__ method of register
	result, except := scope.callMethodStr(regVal)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opGETFLD(scope *callscope, operands []byte) Continue {
	// GETFLD [$X][$Y: class][&Z: 0x00]
	output, class, slot := operands[0], operands[1], operands[2]

	// Retrieve the register for class
	regClass := scope.memory.Get(class)
	// Verify that register is of ClassType
	if regClass.Type().Kind != ClassType {
		return raise(scope.exceptionf(ValueError, "$%v is not a class type", class))
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*ClassValue) //nolint:forcetypeassert
	// Get the field value for the slot
	fieldValue, err := classValue.Get(slot)
	if err != nil {
		return raise(scope.exception(AccessError, err.Error()))
	}

	// Set the output register
	scope.memory.Set(output, fieldValue)

	return continueOk{10}
}

func opSETFLD(scope *callscope, operands []byte) Continue {
	// SETFLD [$X: class][&Y: 0x00][$Z]
	class, slot, element := operands[0], operands[1], operands[2]

	// Retrieve the register for class and its field element
	regClass, regElem := scope.memory.Get(class), scope.memory.Get(element)

	// Check if class value has been initialized
	if IsNullValue(regClass) {
		return raise(scope.exception(ValueError, "nil register"))
	}

	// Verify that register is of ClassType
	if regClass.Type().Kind != ClassType {
		return raise(scope.exceptionf(ValueError, "$%v is not a class type", class))
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*ClassValue) //nolint:forcetypeassert
	// Set the element value to the class
	if err := classValue.Set(slot, regElem); err != nil {
		return raise(scope.exception(AccessError, err.Error()))
	}

	// Update the class register
	scope.memory.Set(class, classValue)

	return continueOk{20}
}

func opGETIDX(scope *callscope, operands []byte) Continue {
	// GETIDX [$X][$Y:col][$Z: idx]
	output, collection, index := operands[0], operands[1], operands[2]

	// Retrieve the register for collection and index
	regCol, regIdx := scope.memory.Get(collection), scope.memory.Get(index)

	// Verify that register is a Collection type
	if !regCol.Type().Kind.IsCollection() {
		return raise(scope.exceptionf(ValueError, "$%v is not a collection type", collection))
	}

	// Cast the collection into a CollectionValue
	collectionValue := regCol.(CollectionValue) //nolint:forcetypeassert
	// Get the value from the collection
	element, err := collectionValue.Get(regIdx)
	if err != nil {
		return raise(scope.exception(AccessError, err.Error()))
	}

	// Set the output register
	scope.memory.Set(output, element)

	return continueOk{10}
}

func opSETIDX(scope *callscope, operands []byte) Continue {
	// SETIDX [$X: col][$Y: idx][$Z]
	collection, index, element := operands[0], operands[1], operands[2]

	// Retrieve the register for collection, index and element
	regCol, regIdx, regElem := scope.memory.Get(collection), scope.memory.Get(index), scope.memory.Get(element)

	// Check if collection value has been initialized
	if IsNullValue(regCol) {
		return raise(scope.exception(ValueError, "nil register"))
	}

	if !regCol.Type().Kind.IsCollection() {
		return raise(scope.exceptionf(ValueError, "$%v is not a collection type", collection))
	}

	// Cast the collection into a CollectionValue
	collectionValue := regCol.(CollectionValue) //nolint:forcetypeassert
	// Set the element value to the collection
	if err := collectionValue.Set(regIdx, regElem); err != nil {
		return raise(scope.exception(AccessError, err.Error()))
	}

	// Update the collection register
	scope.memory.Set(collection, collectionValue)

	return continueOk{20}
}

func opNOT(scope *callscope, operands []byte) Continue {
	// NOT [$X: bool][$Y: __bool__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __bool__ method of register
	result, except := scope.callMethodBool(regVal)
	if except != nil {
		return raise(except).withConsumption(10)
	}

	// Flip the value
	inverted := result.Not()
	// Set the register
	scope.memory.Set(out, inverted)

	return continueOk{15}
}

func opADD(scope *callscope, operands []byte) Continue {
	// ADD [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	var (
		result  RegisterValue
		errcode ExceptionClass
	)

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Add (check for overflow)
		result, errcode = regA.(U64Value).Add(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Add (check for overflow)
		result, errcode = regA.(I64Value).Add(regB.(I64Value))

	default:
		return raise(scope.exceptionf(ValueError, "cannot add registers of type %v", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if errcode != Ok {
		return raise(scope.exception(errcode, "addition overflow")).withConsumption(20)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opSUB(scope *callscope, operands []byte) Continue {
	// SUB [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	var (
		result  RegisterValue
		errcode ExceptionClass
	)

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Sub (check for overflow)
		result, errcode = regA.(U64Value).Sub(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Sub (check for overflow)
		result, errcode = regA.(I64Value).Sub(regB.(I64Value))

	default:
		return raise(scope.exceptionf(ValueError, "cannot sub registers of type %v", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if errcode != Ok {
		return raise(scope.exception(errcode, "subtraction overflow")).withConsumption(20)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opMUL(scope *callscope, operands []byte) Continue {
	// MUL [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	var (
		result  RegisterValue
		errcode ExceptionClass
	)

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Mul (check for overflow)
		result, errcode = regA.(U64Value).Mul(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Mul (check for overflow)
		result, errcode = regA.(I64Value).Mul(regB.(I64Value))

	default:
		return raise(scope.exceptionf(ValueError, "cannot mul registers of type %v", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if errcode != Ok {
		return raise(scope.exception(errcode, "multiplication overflow")).withConsumption(30)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{30}
}

func opDIV(scope *callscope, operands []byte) Continue {
	// DIV [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return raise(except)
	}

	var (
		result  RegisterValue
		errcode ExceptionClass
	)

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Div (check for error)
		result, errcode = regA.(U64Value).Div(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Div (check for overflow)
		result, errcode = regA.(I64Value).Div(regB.(I64Value))

	default:
		return raise(scope.exceptionf(ValueError, "cannot div registers of type %v", regA.Type()))
	}

	// Throw an exception if error occurred
	if errcode != Ok {
		return raise(scope.exception(errcode, "division error")).withConsumption(30)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{30}
}

func opPLOAD(scope *callscope, operands []byte) Continue {
	// PLOAD [$X: stored][&Y: 0x00]
	reg, slot := operands[0], operands[1]

	layout, err := scope.engine.GetStateFields(engineio.PersistentState)
	if err != nil {
		return raise(scope.exceptionf(ReferenceError, "persistent state fields not found: %v", err))
	}

	storageField := layout.Get(slot)
	if storageField == nil {
		return raise(scope.exceptionf(ReferenceError, "persistent state field not found: %v", slot))
	}

	storedValue, ok := scope.engine.persistent.GetStorageEntry(SlotHash(slot))
	if !ok {
		storedValue = nil
	}

	value, err := NewRegisterValue(storageField.Type, storedValue)
	if err != nil {
		return raise(scope.exception(ValueError, err.Error()))
	}

	// Set the register value
	scope.memory.Set(reg, value)

	return continueOk{50}
}

func opPSAVE(scope *callscope, operands []byte) Continue {
	// PSAVE [$X: stored][&Y: 0x00]
	reg, slot := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	if ok := scope.engine.persistent.SetStorageEntry(SlotHash(slot), regVal.Data()); !ok {
		return raise(scope.exceptionf(AccessError, "could not write to &%v", slot))
	}

	return continueOk{100}
}
