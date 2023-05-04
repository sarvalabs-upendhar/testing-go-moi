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
		VMAKE: opVMAKE,
		// BMAKE: opBMAKE,

		// BUILD: opBUILD,
		THROW: opTHROW,
		// EMIT: opEMIT,
		JOIN: opJOIN,

		LT: opLT,
		GT: opGT,
		EQ: opEQ,

		BOOL: opBOOL,
		STR:  opSTR,
		ADDR: opADDR,
		LEN:  opLEN,

		SIZEOF: opSIZEOF,
		GETFLD: opGETFLD,
		SETFLD: opSETFLD,
		GETIDX: opGETIDX,
		SETIDX: opSETIDX,

		GROW: opGROW,
		// SLICE: opSLICE,
		APPEND: opAPPEND,
		POPEND: opPOPEND,
		HASKEY: opHASKEY,
		// MERGE: opMERGE,

		AND: opAND,
		OR:  opOR,
		NOT: opNOT,

		INCR: opINCR,
		DECR: opDECR,

		ADD: opADD,
		SUB: opSUB,
		MUL: opMUL,
		DIV: opDIV,
		MOD: opMOD,

		BXOR: opBXOR,
		BAND: opBAND,
		BOR:  opBOR,
		BNOT: opBNOT,

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
		return scope.propagate(except)
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
		return scope.propagate(except).withConsumption(10)
	}

	// If condition is false, no jump
	if !result {
		return continueOk{10}
	}

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(destination)
	if except != nil {
		return scope.propagate(except).withConsumption(10)
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
		return scope.propagate(except)
	}

	// Get the constant from the environment
	constant, err := scope.engine.GetConstant(engineio.ElementPtr(pointer))
	if err != nil {
		return scope.raise(exceptionf(ReferenceError, "constant %#v not found: %v", pointer, err))
	}

	// Create value from the constant definition
	constVal, except := constant.value()
	if except != nil {
		return scope.raise(except)
	}

	// Set the constant value into the register
	scope.memory.Set(out, constVal)

	return continueOk{20}
}

func opLDPTR(scope *callscope, operands []byte) Continue {
	// LDPTR[1..8] [$X: ptr][0x...]
	target, ptr := operands[0], operands[1:]

	// Decipher constant ID into 64-bit address
	value, err := ptrdecode(ptr)
	if err != nil {
		return scope.raise(exception(OverflowError, "pointer value exceeds 8 bytes"))
	}

	// Set the register value
	scope.memory.Set(target, PtrValue(value))

	return continueOk{8 + engineio.Fuel(len(ptr)*2)}
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
	newVal, _ := NewRegisterValue(regVal.Type(), nil)
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
		return scope.propagate(except)
	}

	typedef, err := scope.engine.GetTypedef(engineio.ElementPtr(pointer))
	if err != nil {
		return scope.raise(exceptionf(ReferenceError, "typedef %#x not found: %v", pointer, err))
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
		return scope.raise(exceptionf(TypeError, "invalid primitive type: %#x", typeID))
	}

	// Create a datatype from the type ID
	dt := Primitive(typeID).Datatype()
	// Create a value for the datatype
	value, _ := NewRegisterValue(dt, nil)

	// Set the register value
	scope.memory.Set(out, value)

	return continueOk{10}
}

func opVMAKE(scope *callscope, operands []byte) Continue {
	// VMAKE [$X][$Y: ptr][$Z: U64]
	out, reg, length := operands[0], operands[1], operands[2]

	// Load the pointer value from the register
	pointer, except := scope.getPtrValue(reg)
	if except != nil {
		return scope.propagate(except)
	}

	typedef, err := scope.engine.GetTypedef(engineio.ElementPtr(pointer))
	if err != nil {
		return scope.raise(exceptionf(ReferenceError, "typedef %#x not found: %v", pointer, err))
	}

	// Check that datatype is a varray
	if typedef.Kind != VarrayType {
		return scope.raise(exceptionf(TypeError, "typedef %#x is not a varray", pointer))
	}

	// Retrieve the register for length
	regLen := scope.memory.Get(length)
	// Cast the register to u64
	size, ok := regLen.(U64Value)
	if !ok {
		return scope.raise(exceptionInvalidDatatype(PrimitiveU64, length))
	}

	// Create a new list with size
	// We ignore the error because all checks have been performed
	list, _ := newSizedList(typedef, size)
	// Set the register value
	scope.memory.Set(out, list)

	return continueOk{10 + engineio.Fuel(5*size)}
}

func opTHROW(scope *callscope, operands []byte) Continue {
	// THROW [$X: __throw__]
	reg := operands[0]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __throw__ method of register
	errdata, except := scope.callMethodThrow(regVal)
	if except != nil {
		return scope.propagate(except).withConsumption(10)
	}

	// Create the custom exception object
	except = exception(CustomExceptionClass{regVal.Type()}, string(errdata))

	return scope.raise(except).withConsumption(30)
}

func opJOIN(scope *callscope, operands []byte) Continue {
	// JOIN [$X][$Y: __join__][$Z: __join__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	// Execute the __join__ method
	result, except := scope.callMethodJoin(regA, regB)
	if except != nil {
		return scope.propagate(except)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{30}
}

func opLT(scope *callscope, operands []byte) Continue {
	// LT [$X: bool][$Y: __lt__][$Z: __lt__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	// Execute the __lt__ method
	result, except := scope.callMethodCompare(MethodLt, regA, regB)
	if except != nil {
		return scope.propagate(except)
	}

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
		return scope.propagate(except)
	}

	// Execute the __gt__ method
	result, except := scope.callMethodCompare(MethodGt, regA, regB)
	if except != nil {
		return scope.propagate(except)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opEQ(scope *callscope, operands []byte) Continue {
	// EQ [$X: bool][$Y: __eq__][$Z: __eq__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	// Execute the __eq__ method
	result, except := scope.callMethodCompare(MethodEq, regA, regB)
	if except != nil {
		return scope.propagate(except)
	}

	// Set the register
	scope.memory.Set(out, result)

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
		return scope.propagate(except).withConsumption(10)
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
		return scope.propagate(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opADDR(scope *callscope, operands []byte) Continue {
	// ADDR [$X: address][$Y: __addr__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __addr__ method of register
	result, except := scope.callMethodAddr(regVal)
	if except != nil {
		return scope.propagate(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opLEN(scope *callscope, operands []byte) Continue {
	// LEN [$X: u64][$Y: __len__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Call the __len__ method of register
	result, except := scope.callMethodLen(regVal)
	if except != nil {
		return scope.propagate(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opSIZEOF(scope *callscope, operands []byte) Continue {
	// SIZEOF [$X: u64][$Y: col/class]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	// Check if class value has been initialized
	if IsNullValue(regVal) {
		return scope.raise(exceptionNullRegister(reg))
	}

	var size U64Value

	switch regVal.Type().Kind {
	case ClassType:
		// Cast the value into a ClassValue
		class := regVal.(*ClassValue) //nolint:forcetypeassert
		// Get the size of the class (number of fields)
		size = class.Size()

	case MappingType, VarrayType, ArrayType:
		// Cast the collection into a CollectionValue
		collection := regVal.(CollectionValue) //nolint:forcetypeassert
		// Get the size of the class (number of elements)
		size = collection.Size()

	default:
		return scope.raise(exceptionInvalidDatatype("sizeable", reg))
	}

	// Set the register
	scope.memory.Set(out, size)

	return continueOk{20}
}

func opGETFLD(scope *callscope, operands []byte) Continue {
	// GETFLD [$X][$Y: class][&Z: 0x00]
	output, class, slot := operands[0], operands[1], operands[2]

	// Retrieve the register for class
	regClass := scope.memory.Get(class)
	// Verify that register is of ClassType
	if regClass.Type().Kind != ClassType {
		return scope.raise(exceptionInvalidDatatype(ClassType, class))
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*ClassValue) //nolint:forcetypeassert
	// Get the field value for the slot
	fieldValue, except := classValue.Get(slot)
	if except != nil {
		return scope.propagate(except)
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

	//// Check if class value has been initialized
	// if IsNullValue(regClass) {
	//	return scope.raise(exceptionNullRegister(class))
	// }

	// Verify that register is of ClassType
	if regClass.Type().Kind != ClassType {
		return scope.raise(exceptionInvalidDatatype(ClassType, class))
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*ClassValue) //nolint:forcetypeassert
	// Set the element value to the class
	if except := classValue.Set(slot, regElem); except != nil {
		return scope.propagate(except)
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
		return scope.raise(exceptionInvalidDatatype("collection", collection))
	}

	// Cast the collection into a CollectionValue
	collectionValue := regCol.(CollectionValue) //nolint:forcetypeassert
	// Get the value from the collection
	element, except := collectionValue.Get(regIdx)
	if except != nil {
		return scope.propagate(except)
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

	//// Check if collection value has been initialized
	// if IsNullValue(regCol) {
	//	return scope.raise(exceptionNullRegister(collection))
	// }

	if !regCol.Type().Kind.IsCollection() {
		return scope.raise(exceptionInvalidDatatype("collection", collection))
	}

	// Cast the collection into a CollectionValue
	collectionValue := regCol.(CollectionValue) //nolint:forcetypeassert
	// Set the element value to the collection
	if except := collectionValue.Set(regIdx, regElem); except != nil {
		return scope.propagate(except)
	}

	// Update the collection register
	scope.memory.Set(collection, collectionValue)

	return continueOk{20}
}

func opGROW(scope *callscope, operands []byte) Continue {
	// GROW [$X: varray][$Y: U64]
	varray, length := operands[0], operands[1]

	// Retrieve the register for varray
	regVarray := scope.memory.Get(varray)
	// Check that value is of type varray
	if regVarray.Type().Kind != VarrayType {
		return scope.raise(exceptionInvalidDatatype(VarrayType, varray))
	}

	// Retrieve the register for length
	regLen := scope.memory.Get(length)
	// Cast the register to u64
	size, ok := regLen.(U64Value)
	if !ok {
		return scope.raise(exceptionInvalidDatatype(PrimitiveU64, length))
	}

	// Cast into ListValue
	list, _ := regVarray.(*ListValue)
	// Grow the list by the given size, we don't need to check for error
	// because we have already checked for the only possible error case.
	_ = list.Grow(size)

	return continueOk{5 + engineio.Fuel(5*size)}
}

func opAPPEND(scope *callscope, operands []byte) Continue {
	// APPEND [$X: varray][$Y]
	varray, element := operands[0], operands[1]

	// Retrieve the register for varray and element
	regVarray, regElem := scope.memory.Get(varray), scope.memory.Get(element)
	// Check that value is of type varray
	if regVarray.Type().Kind != VarrayType {
		return scope.raise(exceptionInvalidDatatype(VarrayType, varray))
	}

	// Cast into ListValue
	list, _ := regVarray.(*ListValue)
	// Append the value into list. Only possible error here is invalid element type
	if err := list.Append(regElem); err != nil {
		return scope.raise(exception(TypeError, err.Error()))
	}

	// Update the varray register
	scope.memory.Set(varray, regVarray)

	return continueOk{20}
}

func opPOPEND(scope *callscope, operands []byte) Continue {
	// POPEND [$X][$Y: varray]
	out, varray := operands[0], operands[1]

	// Retrieve the register for varray
	regVarray := scope.memory.Get(varray)
	// Check that value is of type varray
	if regVarray.Type().Kind != VarrayType {
		return scope.raise(exceptionInvalidDatatype(VarrayType, varray))
	}

	// Cast into ListValue
	list, _ := regVarray.(*ListValue)
	// Popend a value from the list. Only possible error here is empty varray
	element, err := list.Popend()
	if err != nil {
		return scope.raise(exception(ValueError, err.Error()))
	}

	// Update the varray register and set popped value
	scope.memory.Set(varray, regVarray)
	scope.memory.Set(out, element)

	return continueOk{20}
}

func opHASKEY(scope *callscope, operands []byte) Continue {
	// HASKEY [$X: bool][$Y: map][$Z]
	out, mapping, key := operands[0], operands[1], operands[2]

	// Retrieve the mapping register
	regMapping := scope.memory.Get(mapping)
	// Check that value is of type mapping
	if regMapping.Type().Kind != MappingType {
		return scope.raise(exceptionInvalidDatatype(MappingType, mapping))
	}

	// Retrieve the key register
	regKey := scope.memory.Get(key)
	// Cast the mapping into a MapValue
	mapvalue, _ := regMapping.(*MapValue)

	// Check if the map has the key
	result, except := mapvalue.Has(regKey)
	if except != nil {
		return scope.raise(except)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{15}
}

func opAND(scope *callscope, operands []byte) Continue {
	// AND [$X: bool][$Y: __bool__][$Y: __bool__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	// Call the __bool__ method of register A
	valueA, except := scope.callMethodBool(regA)
	if except != nil {
		return scope.propagate(except).withConsumption(5)
	}

	// Call the __bool__ method of register B
	// We skip the exception check here because, A and B are symmetric
	// If an exception was not thrown at the boolean evaluation of A, it will not be thrown here.
	valueB, _ := scope.callMethodBool(regB)

	// Perform boolean AND on bool(A) and bool(B)
	result := valueA.And(valueB)
	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opOR(scope *callscope, operands []byte) Continue {
	// OR [$X: bool][$Y: __bool__][$Y: __bool__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	// Call the __bool__ method of register A
	valueA, except := scope.callMethodBool(regA)
	if except != nil {
		return scope.propagate(except).withConsumption(5)
	}

	// Call the __bool__ method of register B
	valueB, except := scope.callMethodBool(regB)
	if except != nil {
		return scope.propagate(except).withConsumption(10)
	}

	// Perform boolean OR on bool(A) and bool(B)
	result := valueA.Or(valueB)
	// Set the register
	scope.memory.Set(out, result)

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
		return scope.propagate(except).withConsumption(5)
	}

	// Flip the value
	inverted := result.Not()
	// Set the register
	scope.memory.Set(out, inverted)

	return continueOk{15}
}

func opBXOR(scope *callscope, operands []byte) Continue {
	// BXOR [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Bxor
		result = regA.(U64Value).Bxor(regB.(U64Value)) //nolint:forcetypeassert

	case TypeI64:
		// Cast register values to I64 and call Bxor
		result = regA.(I64Value).Bxor(regB.(I64Value)) //nolint:forcetypeassert

	default:
		return scope.raise(exceptionInvalidOperationForType("bxor", regA.Type()))
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opBAND(scope *callscope, operands []byte) Continue {
	// BAND [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Band
		result = regA.(U64Value).Band(regB.(U64Value)) //nolint:forcetypeassert

	case TypeI64:
		// Cast register values to I64 and call Band
		result = regA.(I64Value).Band(regB.(I64Value)) //nolint:forcetypeassert

	default:
		return scope.raise(exceptionInvalidOperationForType("band", regA.Type()))
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opBOR(scope *callscope, operands []byte) Continue {
	// BOR [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Bor
		result = regA.(U64Value).Bor(regB.(U64Value)) //nolint:forcetypeassert

	case TypeI64:
		// Cast register values to I64 and call Bor
		result = regA.(I64Value).Bor(regB.(I64Value)) //nolint:forcetypeassert

	default:
		return scope.raise(exceptionInvalidOperationForType("bor", regA.Type()))
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opBNOT(scope *callscope, operands []byte) Continue {
	// BNOT [$X][$Y]
	out, a := operands[0], operands[1]

	// Get registervalue of the obtained address
	regA := scope.memory.Get(a)

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Bnot
		result = regA.(U64Value).Bnot() //nolint:forcetypeassert

	case TypeI64:
		// Cast register values to I64 and call Bnot
		result = regA.(I64Value).Bnot() //nolint:forcetypeassert

	default:
		return scope.raise(exceptionInvalidOperationForType("bnot", regA.Type()))
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{20}
}

func opINCR(scope *callscope, operands []byte) Continue {
	// INCR [$X]
	reg := operands[0]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	var (
		result RegisterValue
		except *Exception
	)

	switch dt := regVal.Type(); dt {
	case TypeU64:
		// Cast register value to U64 and call Inr (check for overflow)
		result, except = regVal.(U64Value).Incr()

	case TypeI64:
		// Cast register value to I64 and call Incr (check for overflow)
		result, except = regVal.(I64Value).Incr()

	default:
		return scope.raise(exceptionInvalidOperationForType("increment", regVal.Type()))
	}

	// Raise an exception if overflow occurred
	if except != nil {
		return scope.raise(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(reg, result)

	return continueOk{10}
}

func opDECR(scope *callscope, operands []byte) Continue {
	// DECR [$X]
	reg := operands[0]

	// Retrieve the register
	regVal := scope.memory.Get(reg)

	var (
		result RegisterValue
		except *Exception
	)

	switch dt := regVal.Type(); dt {
	case TypeU64:
		// Cast register value to U64 and call Decr (check for overflow)
		result, except = regVal.(U64Value).Decr()

	case TypeI64:
		// Cast register value to I64 and call Decr (check for overflow)
		result, except = regVal.(I64Value).Decr()

	default:
		return scope.raise(exceptionInvalidOperationForType("decrement", regVal.Type()))
	}

	// Throw an exception if overflow occurred
	if except != nil {
		return scope.raise(except).withConsumption(10)
	}

	// Set the register
	scope.memory.Set(reg, result)

	return continueOk{10}
}

func opADD(scope *callscope, operands []byte) Continue {
	// ADD [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Add (check for overflow)
		result, except = regA.(U64Value).Add(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Add (check for overflow)
		result, except = regA.(I64Value).Add(regB.(I64Value))

	default:
		return scope.raise(exceptionInvalidOperationForType("add", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if except != nil {
		return scope.raise(except).withConsumption(20)
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
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Sub (check for overflow)
		result, except = regA.(U64Value).Sub(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Sub (check for overflow)
		result, except = regA.(I64Value).Sub(regB.(I64Value))

	default:
		return scope.raise(exceptionInvalidOperationForType("subtract", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if except != nil {
		return scope.raise(except).withConsumption(20)
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
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Mul (check for overflow)
		result, except = regA.(U64Value).Mul(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Mul (check for overflow)
		result, except = regA.(I64Value).Mul(regB.(I64Value))

	default:
		return scope.raise(exceptionInvalidOperationForType("multiply", regA.Type()))
	}

	// Throw an exception if overflow occurred
	if except != nil {
		return scope.raise(except).withConsumption(30)
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
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Div (check for error)
		result, except = regA.(U64Value).Div(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Div (check for overflow)
		result, except = regA.(I64Value).Div(regB.(I64Value))

	default:
		return scope.raise(exceptionInvalidOperationForType("divide", regA.Type()))
	}

	// Throw an exception if error occurred
	if except != nil {
		return scope.raise(except).withConsumption(30)
	}

	// Set the register
	scope.memory.Set(out, result)

	return continueOk{30}
}

func opMOD(scope *callscope, operands []byte) Continue {
	// MOD [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.getSymmetricValues(a, b)
	if except != nil {
		return scope.propagate(except)
	}

	var result RegisterValue

	switch dt := regA.Type(); dt {
	case TypeU64:
		// Cast register values to U64 and call Mod (check for error)
		result, except = regA.(U64Value).Mod(regB.(U64Value))

	case TypeI64:
		// Cast register values to I64 and call Mod (check for overflow)
		result, except = regA.(I64Value).Mod(regB.(I64Value))

	default:
		return scope.raise(exceptionInvalidOperationForType("modulo divide", regA.Type()))
	}

	// Throw an exception if error occurred
	if except != nil {
		return scope.raise(except).withConsumption(30)
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
		return scope.raise(exceptionf(ReferenceError, "persistent state fields not found: %v", err))
	}

	storageField := layout.Get(slot)
	if storageField == nil {
		return scope.raise(exceptionf(ReferenceError, "persistent state field not found: %v", slot))
	}

	storedValue, ok := scope.engine.persistent.GetStorageEntry(SlotHash(slot))
	if !ok {
		storedValue = nil
	}

	value, err := NewRegisterValue(storageField.Type, storedValue)
	if err != nil {
		return scope.raise(exception(ValueError, err.Error()))
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
		return scope.raise(exceptionf(AccessError, "could not write to &%v", slot))
	}

	return continueOk{100}
}
