//nolint:nlreturn
package pisa

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/jug/pisa/register"
)

// Instruction represent a single logical instruction
// to execute with an opcode and the arguments for it.
type Instruction struct {
	Op   OpCode
	Args []byte
}

// Instructions represents a Set() of instruction objects.
type Instructions []Instruction

func (instructs Instructions) Len() uint64 {
	return uint64(len(instructs))
}

// InstructOperation represents the instruction operation for a single PISA OpCode.
// The function executes within a given execution scope
type InstructOperation func(*CallScope, []byte) engineio.Fuel

// InstructionSet represents the opcode instructions for the PISA Runtime
type InstructionSet [256]InstructOperation

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
		// ZERO: opZERO,
		// CLEAR: opCLEAR,
		// SAME: opSAME,
		COPY: opCOPY,
		// SWAP: opSWAP,
		// SERIAL: opSERIAL,
		// DESERIAL: opDESERIAL,

		MAKE:  opMAKE,
		PMAKE: opPMAKE,
		// VMAKE: opVMAKE,
		// BMAKE: opBMAKE,

		// BUILD: opBUILD,
		// THROW: opTHROW,
		// EMIT: opEMIT,
		// JOIN: opJOIN,

		LT: opLT,
		GT: opGT,
		EQ: opEQ,

		BOOL: opBOOL,
		STR:  opSTR,
		// ADDR: opADDR,

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

func opTERM(scope *CallScope, _ []byte) engineio.Fuel {
	scope.stop()
	return 0
}

func opDEST(_ *CallScope, _ []byte) engineio.Fuel {
	return 1
}

func opJUMP(scope *CallScope, operands []byte) engineio.Fuel {
	// JUMP [$X: ptr]
	destination := operands[0]

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(destination)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	scope.jumpTo(uint64(pointer))

	return 10
}

func opJUMPI(scope *CallScope, operands []byte) engineio.Fuel {
	// JUMPI [$X: ptr][$Y: __bool__]
	destination, condition := operands[0], operands[1]

	// Retrieve the condition register
	regCondition, exists := scope.registers.Get(condition)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", condition))
		return 0
	}

	// Check that condition register implements __bool__
	methodBool, ok := scope.engine.GetTypeMethod(regCondition.Type(), register.MethodBool)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __bool__", regCondition.Type()))
		return 0
	}

	// Execute the __bool__ method of the condition
	outputs := methodBool.Execute(scope, register.ValueTable{0: regCondition})
	// Check if an exception was thrown
	if scope.ExceptionThrown() {
		return 10
	}

	// If condition is false, no jump
	result := outputs[0]
	if !result.(register.BoolValue) { //nolint:forcetypeassert
		return 10
	}

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(destination)
	if except != nil {
		scope.Throw(except)
		return 10
	}

	scope.jumpTo(uint64(pointer))

	return 20
}

func opOBTAIN(scope *CallScope, operands []byte) engineio.Fuel {
	// OBTAIN [$X][&Y]
	reg, slot := operands[0], operands[1]

	// Retrieve the calldata value
	val, exists := scope.inputs.Get(slot)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.InputNotFound, "input &%v", slot))
		return 0
	}

	// Set the register value
	scope.registers.Set(reg, val)

	return 5
}

func opYIELD(scope *CallScope, operands []byte) engineio.Fuel {
	// YIELD [$X][&Y]
	reg, slot := operands[0], operands[1]

	// Retrieve the register
	value, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	// Set the return slot value
	scope.outputs.Set(slot, value)

	return 5
}

func opCONST(scope *CallScope, operands []byte) engineio.Fuel {
	// CONST [$X][$Y: ptr]
	out, reg := operands[0], operands[1]

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(reg)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	// Get the constant from the environment
	constant, err := scope.engine.GetConstant(pointer)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "constant %#v not found: %v", pointer, err))
		return 0
	}

	// Create value from the constant definition
	constVal, err := constant.Value()
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))
		return 0
	}

	// Set the constant value into the register
	scope.registers.Set(out, constVal)

	return 20
}

func opLDPTR(scope *CallScope, operands []byte) engineio.Fuel {
	// LDPTRn [$X: ptr][0x00]
	target, ptr := operands[0], operands[1:]

	// Decipher constant ID into 64-bit address
	value, err := ptrdecode(ptr)
	if err != nil {
		scope.Throw(exception.Exception(exception.PointerOverflow, ""))
		return 0
	}

	// Create a new Pointer value
	pointer := register.PtrValue(value)
	// Set the register value
	scope.registers.Set(target, pointer)

	return 10 + engineio.Fuel(len(ptr))
}

func opISNULL(scope *CallScope, operands []byte) engineio.Fuel {
	// ISNULL [$X: bool][$Y]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	// Set isnull to true if register has nil value or has type Null
	isnull := register.BoolValue(register.IsNullValue(regVal))
	// Set the register
	scope.registers.Set(out, isnull)

	return 5
}

func opCOPY(scope *CallScope, operands []byte) engineio.Fuel {
	// Fetch the source and destination registers IDs
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	reg, exists := scope.registers.Get(source)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", source))
		return 0
	}

	// Set a copy of the register to the destination
	scope.registers.Set(destination, reg.Copy())

	return 5
}

func opMAKE(scope *CallScope, operands []byte) engineio.Fuel {
	// MAKE [$X][$Y: ptr]
	reg := operands[0]

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(reg)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	typedef, err := scope.engine.GetTypedef(pointer)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "typedef %#v not found: %v", pointer, err))
		return 0
	}

	// Create a new default value for the typedef
	typeval, err := register.NewValue(typedef, nil)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))
		return 0
	}

	// Set the constant value into the register
	scope.registers.Set(reg, typeval)

	return 20
}

func opPMAKE(scope *CallScope, operands []byte) engineio.Fuel {
	// PMAKE [$X][Y: 0x00]
	output, typeID := operands[0], operands[1]

	// Check if type ID is valid
	if typeID > engineio.MaxPrimitive {
		scope.Throw(exception.Exceptionf(exception.InvalidTypeID, "type ID %#v", typeID))
		return 0
	}

	// Create a datatype from the type ID
	datatype := engineio.Primitive(typeID).Datatype()
	// Create a value for the datatype
	value, err := register.NewValue(datatype, nil)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))
		return 0
	}

	// Set the register value
	scope.registers.Set(output, value)

	return 10
}

func opLT(scope *CallScope, operands []byte) engineio.Fuel {
	// LT [$X: bool][$Y: __lt__][$Z: __lt__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	// Retrieve the __lt__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodLt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __lt__", regA.Type()))
		return 0
	}

	// Execute the __lt__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})
	// Check for exceptions
	if scope.ExceptionThrown() {
		return 0
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(out, result)

	return 20
}

func opGT(scope *CallScope, operands []byte) engineio.Fuel {
	// GT [$X: bool][$Y: __gt__][$Z: __gt__]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	// Retrieve the __gt__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodGt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __gt__", regA.Type()))
		return 0
	}

	// Execute the __gt__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})
	// Check for exceptions
	if scope.ExceptionThrown() {
		return 0
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(out, result)

	return 20
}

func opEQ(scope *CallScope, operands []byte) engineio.Fuel {
	// EQ [$X: bool][$Y: __eq__][$Z: __eq__]
	a, b := operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	// Retrieve the __eq__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodEq)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __eq__", regA.Type()))
		return 0
	}

	// Execute the __eq__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})
	// Check for exceptions
	if scope.ExceptionThrown() {
		return 0
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(operands[0], result)

	return 20
}

func opBOOL(scope *CallScope, operands []byte) engineio.Fuel {
	// BOOL [$X: bool][$Y: __bool__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	// Retrieve the __bool__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regVal.Type(), register.MethodBool)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __bool__", regVal.Type()))
		return 0
	}

	// Execute the __bool__ method
	outputs := method.Execute(scope, register.ValueTable{0: regVal})
	// Check for exceptions
	if scope.ExceptionThrown() {
		return 0
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(out, result)

	return 20
}

func opSTR(scope *CallScope, operands []byte) engineio.Fuel {
	// STR [$X: string][$Y: __str__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	// Retrieve the __str__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regVal.Type(), register.MethodStr)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __str__", regVal.Type()))
		return 0
	}

	// Execute the __str__ method
	outputs := method.Execute(scope, register.ValueTable{0: regVal})
	// Check for exceptions
	if scope.ExceptionThrown() {
		return 0
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(out, result)

	return 20
}

func opGETFLD(scope *CallScope, operands []byte) engineio.Fuel {
	// GETFLD [$X][$Y: class][&Z: 0x00]
	output, class, slot := operands[0], operands[1], operands[2]

	var (
		exists   bool
		regClass register.Value
	)

	// Retrieve the register for class
	if regClass, exists = scope.registers.Get(class); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", class))
		return 0
	}

	// Verify that register is of ClassType
	if regClass.Type().Kind != engineio.ClassType {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a class type", class))
		return 0
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*register.ClassValue) //nolint:forcetypeassert
	// Get the field value for the slot
	fieldValue, err := classValue.Get(slot)
	if err != nil {
		scope.Throw(exception.Exception(exception.ClassFieldAccess, err.Error()))
		return 0
	}

	// Set the output register
	scope.registers.Set(output, fieldValue)

	return 10
}

func opSETFLD(scope *CallScope, operands []byte) engineio.Fuel {
	// SETFLD [$X: class][&Y: 0x00][$Z]
	class, slot, element := operands[0], operands[1], operands[2]

	var (
		exists            bool
		regClass, regElem register.Value
	)

	// Retrieve the register for class
	if regClass, exists = scope.registers.Get(class); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", class))
		return 0
	}

	// Retrieve the register for field element
	if regElem, exists = scope.registers.Get(element); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", element))
		return 0
	}

	// Check if class value has been initialized
	if register.IsNullValue(regClass) {
		scope.Throw(exception.Exception(exception.NilCollection, "cannot set to nil class"))
		return 0
	}

	// Verify that register is of ClassType
	if regClass.Type().Kind != engineio.ClassType {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a class type", class))
		return 0
	}

	// Cast the value into a ClassValue
	classValue := regClass.(*register.ClassValue) //nolint:forcetypeassert
	// Set the element value to the class
	if err := classValue.Set(slot, regElem); err != nil {
		scope.Throw(exception.Exception(exception.ClassFieldAccess, err.Error()))
		return 0
	}

	// Update the class register
	scope.registers.Set(class, classValue)

	return 20
}

func opGETIDX(scope *CallScope, operands []byte) engineio.Fuel {
	// GETIDX [$X][$Y:col][$Z: idx]
	output, collection, index := operands[0], operands[1], operands[2]

	var (
		exists         bool
		regCol, regIdx register.Value
	)

	// Retrieve the register for collection
	if regCol, exists = scope.registers.Get(collection); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", collection))
		return 0
	}

	// Retrieve the register for index
	if regIdx, exists = scope.registers.Get(index); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", index))
		return 0
	}

	if !regCol.Type().Kind.IsCollection() {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a collection type", collection))
		return 0
	}

	// Cast the collection into a Collection
	collectionValue := regCol.(register.Collection) //nolint:forcetypeassert
	// Get the value from the collection
	element, err := collectionValue.Get(regIdx)
	if err != nil {
		scope.Throw(exception.Exception(exception.CollectionAccess, err.Error()))
		return 0
	}

	// Set the output register
	scope.registers.Set(output, element)

	return 10
}

func opSETIDX(scope *CallScope, operands []byte) engineio.Fuel {
	// SETIDX [$X: col][$Y: idx][$Z]
	collection, index, element := operands[0], operands[1], operands[2]

	var (
		exists                  bool
		regCol, regIdx, regElem register.Value
	)

	// Retrieve the register for collection
	if regCol, exists = scope.registers.Get(collection); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", collection))
		return 0
	}

	// Retrieve the register for index
	if regIdx, exists = scope.registers.Get(index); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", index))
		return 0
	}

	// Retrieve the register for element
	if regElem, exists = scope.registers.Get(element); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", element))
		return 0
	}

	// Check if collection value has been initialized
	if register.IsNullValue(regCol) {
		scope.Throw(exception.Exception(exception.NilCollection, "cannot set to nil mapping"))
		return 0
	}

	if !regCol.Type().Kind.IsCollection() {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a collection type", collection))
		return 0
	}

	// Cast the collection into a Collection
	collectionValue := regCol.(register.Collection) //nolint:forcetypeassert
	// Set the element value to the collection
	if err := collectionValue.Set(regIdx, regElem); err != nil {
		scope.Throw(exception.Exception(exception.CollectionAccess, err.Error()))
		return 0
	}

	// Update the collection register
	scope.registers.Set(collection, collectionValue)

	return 20
}

func opNOT(scope *CallScope, operands []byte) engineio.Fuel {
	// NOT [$X: bool][$Y: __bool__]
	out, reg := operands[0], operands[1]

	// Retrieve the register
	regVal, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	// Check that register is Boolean
	if !regVal.Type().Equals(engineio.TypeBool) {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "type %v does not implement __bool__", regVal.Type()))
		return 0
	}

	// Cast the register value to Bool and flip
	inverted := regVal.(register.BoolValue).Not() //nolint:forcetypeassert
	// Set the register
	scope.registers.Set(out, inverted)

	return 10
}

//nolint:dupl
func opADD(scope *CallScope, operands []byte) engineio.Fuel {
	// ADD [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	var (
		result   register.Value
		overflow error
	)

	switch dt := regA.Type(); dt {
	case engineio.TypeU64:
		// Cast register values to U64 and call Add (check for overflow)
		result, overflow = regA.(register.U64Value).Add(regB.(register.U64Value))

	case engineio.TypeI64:
		// Cast register values to I64 and call Add (check for overflow)
		result, overflow = regA.(register.I64Value).Add(regB.(register.I64Value))

	default:
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "cannot ADD registers of type %v", regA.Type()))
		return 0
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "addition"))
		return 20
	}

	// Set the register
	scope.registers.Set(out, result)

	return 20
}

//nolint:dupl
func opSUB(scope *CallScope, operands []byte) engineio.Fuel {
	// SUB [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	var (
		result   register.Value
		overflow error
	)

	switch dt := regA.Type(); dt {
	case engineio.TypeU64:
		// Cast register values to U64 and call Sub (check for overflow)
		result, overflow = regA.(register.U64Value).Sub(regB.(register.U64Value))

	case engineio.TypeI64:
		// Cast register values to I64 and call Sub (check for overflow)
		result, overflow = regA.(register.I64Value).Sub(regB.(register.I64Value))

	default:
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "cannot SUB registers of type %v", regA.Type()))
		return 0
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "subtraction"))
		return 20
	}

	// Set the register
	scope.registers.Set(out, result)

	return 20
}

//nolint:dupl
func opMUL(scope *CallScope, operands []byte) engineio.Fuel {
	// MUL [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	var (
		result   register.Value
		overflow error
	)

	switch dt := regA.Type(); dt {
	case engineio.TypeU64:
		// Cast register values to U64 and call Mul (check for overflow)
		result, overflow = regA.(register.U64Value).Mul(regB.(register.U64Value))

	case engineio.TypeI64:
		// Cast register values to I64 and call Mul (check for overflow)
		result, overflow = regA.(register.I64Value).Mul(regB.(register.I64Value))

	default:
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "cannot MUL registers of type %v", regA.Type()))
		return 0
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "multiplication"))
		return 30
	}

	// Set the register
	scope.registers.Set(out, result)

	return 30
}

func opDIV(scope *CallScope, operands []byte) engineio.Fuel {
	// DIV [$X][$Y][$Z]
	out, a, b := operands[0], operands[1], operands[2]

	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)
		return 0
	}

	var (
		result  register.Value
		matherr error
	)

	switch dt := regA.Type(); dt {
	case engineio.TypeU64:
		// Cast register values to U64 and call Div (check for error)
		result, matherr = regA.(register.U64Value).Div(regB.(register.U64Value))

	case engineio.TypeI64:
		// Cast register values to I64 and call Div (check for overflow)
		result, matherr = regA.(register.I64Value).Div(regB.(register.I64Value))

	default:
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "cannot DIV registers of type %v", regA.Type()))
		return 0
	}

	// Throw an exception if overflow occurred
	if matherr != nil {
		if errors.Is(matherr, register.ErrIntegerOverflow) {
			scope.Throw(exception.Exception(exception.ArithmeticOverflow, "division"))
		} else if errors.Is(matherr, register.ErrDivideByZero) {
			scope.Throw(exception.Exception(exception.ArithmeticDivideByZero, "division"))
		}

		return 30
	}

	// Set the register
	scope.registers.Set(out, result)

	return 30
}

func opPLOAD(scope *CallScope, operands []byte) engineio.Fuel {
	// PLOAD [$X: stored][&Y: 0x00]
	reg, slot := operands[0], operands[1]

	layout, err := scope.engine.GetStateFields(engineio.PersistentState)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "persistent state field not found: %v", err))
		return 0
	}

	storageField := layout.Get(slot)
	if storageField == nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "persistent state field &%v", slot))
		return 0
	}

	storedValue, ok := scope.internal.GetStorageEntry(SlotHash(slot))
	if !ok {
		storedValue = nil
	}

	value, err := register.NewValue(storageField.Type, storedValue)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))
		return 0
	}

	// Set the register value
	scope.registers.Set(reg, value)

	return 50
}

func opPSAVE(scope *CallScope, operands []byte) engineio.Fuel {
	// PSAVE [$X: stored][&Y: 0x00]
	reg, slot := operands[0], operands[1]

	// Retrieve the register
	regVal, exists := scope.registers.Get(reg)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", reg))
		return 0
	}

	if ok := scope.internal.SetStorageEntry(SlotHash(slot), regVal.Data()); !ok {
		scope.Throw(exception.Exceptionf(exception.StorageWrite, "could not write to &%v", slot))
		return 0
	}

	return 100
}
