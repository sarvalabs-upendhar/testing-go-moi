package pisa

import (
	"bytes"

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

// InstructOperation represents the instruction operations for a single PISA OpCode.
// The operations include fuel calculation, operand reading and op execution
type InstructOperation struct {
	// operand specifies a function for reading the operands for an OpCode from a bytes.Reader.
	// The function returns the slice of operands as well as a bool indicating a successful read.
	Operand func(*bytes.Reader) ([]byte, bool)

	// execute specifies a function for executing an OpCode within a given Context for some operands.
	Execute func(*ExecutionScope, []byte)
	// expense specifies a function for calculating the fuel consumption of an OpCode.
	Expense func(scope *ExecutionScope) engineio.Fuel
}

// InstructionSet represents the opcode instructions for the PISA Runtime
type InstructionSet [256]*InstructOperation

// BaseInstructionSet returns an InstructionSet with all the base opcodes and their instructions initialized.
func BaseInstructionSet() InstructionSet {
	return InstructionSet{
		TERM:  {operand0, opTERM, fuel},
		DEST:  {operand0, opDEST, fuel},
		JUMP:  {operand1, opJUMP, fuel},
		JUMPI: {operand2, opJUMPI, fuel},

		MAKE:  {operand2, opMAKE, fuel},
		LDPTR: {operandLDPTR, opLDPTR, fuel},
		CONST: {operand1, opCONST, fuel},
		BUILD: {operand1, opBUILD, fuel},

		ACCEPT: {operand2, opACCEPT, fuel},
		RETURN: {operand2, opRETURN, fuel},

		LOAD:  {operand2, opLOAD, fuel},
		STORE: {operand2, opSTORE, fuel},

		BOOL: {operand1, opBOOL, fuel},
		STR:  {operand1, opSTR, fuel},

		ISNULL: {operand2, opISNULL, fuel},

		COPY: {operand2, opCOPY, fuel},
		MOVE: {operand2, opMOVE, fuel},

		GETIDX: {operand3, opGETIDX, fuel},
		SETIDX: {operand3, opSETIDX, fuel},

		LT:  {operand3, opLT, fuel},
		LE:  {operand3, opLE, fuel},
		GT:  {operand3, opGT, fuel},
		GE:  {operand3, opGE, fuel},
		EQ:  {operand3, opEQ, fuel},
		NEQ: {operand3, opNEQ, fuel},

		INVERT: {operand1, opINVERT, fuel},

		ADD: {operand3, opADD, fuel},
		SUB: {operand3, opSUB, fuel},
		MUL: {operand3, opMUL, fuel},
		DIV: {operand3, opDIV, fuel},
	}
}

// fuel is a standard fuel function that deducts 10 FUEL
func fuel(_ *ExecutionScope) engineio.Fuel { return 10 }

func opTERM(scope *ExecutionScope, _ []byte) { scope.stop() }

func opDEST(_ *ExecutionScope, _ []byte) {}

func opJUMP(scope *ExecutionScope, operands []byte) {
	destination := operands[0]

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(destination)
	if except != nil {
		scope.Throw(except)

		return
	}

	scope.jumpTo(uint64(pointer))
}

func opJUMPI(scope *ExecutionScope, operands []byte) {
	condition, destination := operands[0], operands[1]

	// Retrieve the condition register
	regCondition, exists := scope.registers.Get(condition)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", condition))

		return
	}

	// Check that register is Boolean
	if !regCondition.Type().Equals(engineio.TypeBool) {
		scope.Throw(exception.Exception(
			exception.InvalidRegisterType,
			"cannot evaluate non-boolean register for jump condition",
		))

		return
	}

	// If condition is false, no jump
	if !regCondition.(register.BoolValue) { //nolint:forcetypeassert
		return
	}

	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(destination)
	if except != nil {
		scope.Throw(except)

		return
	}

	scope.jumpTo(uint64(pointer))
}

func opMAKE(scope *ExecutionScope, operands []byte) {
	// Fetch the target register and the type ID
	output, typeID := operands[0], operands[1]

	// Check if type ID is valid
	if typeID > engineio.MaxPrimitive {
		scope.Throw(exception.Exceptionf(exception.InvalidTypeID, "type ID %#v", typeID))

		return
	}

	// Create a datatype from the type ID
	datatype := engineio.Primitive(typeID).Datatype()
	// Create a value for the datatype
	value, err := register.NewValue(datatype, nil)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))

		return
	}

	// Set the register value
	scope.registers.Set(output, value)
}

func opLDPTR(scope *ExecutionScope, operands []byte) {
	// Fetch the register ID and pointer value
	target, pointerData := operands[1], operands[2:]

	// Decipher constant ID into 64-bit address
	pointerVal, err := ptrdecode(pointerData)
	if err != nil {
		scope.Throw(exception.Exception(exception.PointerOverflow, ""))

		return
	}

	// Create a new Pointer value
	pointer := register.PtrValue(pointerVal)
	// Set the register value
	scope.registers.Set(target, pointer)
}

func opCONST(scope *ExecutionScope, operands []byte) {
	// Fetch the registers ID
	regID := operands[0]
	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(regID)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Get the constant from the environment
	constant, err := scope.engine.GetConstant(pointer)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "constant %#v not found: %v", pointer, err))

		return
	}

	// Create value from the constant definition
	constVal, err := constant.Value()
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))

		return
	}

	// Set the constant value into the register
	scope.registers.Set(regID, constVal)
}

func opBUILD(scope *ExecutionScope, operands []byte) {
	// Fetch the registers ID
	regID := operands[0]
	// Load the pointer value from the register
	pointer, except := scope.GetPtrValue(regID)
	if except != nil {
		scope.Throw(except)

		return
	}

	typedef, err := scope.engine.GetTypedef(pointer)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "typedef %#v not found: %v", pointer, err))

		return
	}

	// Create a new default value for the typedef
	typeval, err := register.NewValue(typedef, nil)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))

		return
	}

	// Set the constant value into the register
	scope.registers.Set(regID, typeval)
}

func opACCEPT(scope *ExecutionScope, operands []byte) {
	// Fetch the register ID and load slot
	regID, slot := operands[0], operands[1]

	// Retrieve the calldata value
	val, exists := scope.inputs.Get(slot)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.InputNotFound, "input &%v", slot))

		return
	}

	// Set the register value
	scope.registers.Set(regID, val)
}

func opRETURN(scope *ExecutionScope, operands []byte) {
	// Fetch the register ID and return slot
	regID, slot := operands[0], operands[1]

	// Retrieve the register
	value, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	// Set the calldata value
	scope.outputs.Set(slot, value)
}

func opLOAD(scope *ExecutionScope, operands []byte) {
	// Fetch the register ID and storage slot
	regID, slot := operands[0], operands[1]

	layout, err := scope.engine.GetStateFields(engineio.PersistentState)
	if err != nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "persistent state field not found: %v", err))

		return
	}

	storageField := layout.Get(slot)
	if storageField == nil {
		scope.Throw(exception.Exceptionf(exception.ElementNotFound, "persistent state field &%v", slot))

		return
	}

	storedValue, ok := scope.internal.GetStorageEntry(SlotHash(slot))
	if !ok {
		storedValue = nil
	}

	value, err := register.NewValue(storageField.Type, storedValue)
	if err != nil {
		scope.Throw(exception.Exception(exception.ValueInit, err.Error()))

		return
	}

	// Set the register value
	scope.registers.Set(regID, value)
}

func opSTORE(scope *ExecutionScope, operands []byte) {
	// Fetch the register ID and storage slot
	regID, slot := operands[0], operands[1]

	// Retrieve the register
	reg, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	if ok := scope.internal.SetStorageEntry(SlotHash(slot), reg.Data()); !ok {
		scope.Throw(exception.Exceptionf(exception.StorageWrite, "could not write to &%v", slot))

		return
	}
}

func opBOOL(scope *ExecutionScope, operands []byte) {
	regID := operands[0]

	// Retrieve the register
	reg, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	// Retrieve the __bool__ method for the register type
	method, ok := scope.engine.GetTypeMethod(reg.Type(), register.MethodBool)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __bool__", reg.Type()))

		return
	}

	// Execute the __bool__ method
	outputs := method.Execute(scope, register.ValueTable{0: reg})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(regID, result)
}

func opSTR(scope *ExecutionScope, operands []byte) {
	regID := operands[0]

	// Retrieve the register
	reg, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	// Retrieve the __str__ method for the register type
	method, ok := scope.engine.GetTypeMethod(reg.Type(), register.MethodStr)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __str__", reg.Type()))

		return
	}

	// Execute the __str__ method
	outputs := method.Execute(scope, register.ValueTable{0: reg})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(regID, result)
}

func opISNULL(scope *ExecutionScope, operands []byte) {
	// Fetch the registers IDs
	out, regID := operands[0], operands[1]

	// Retrieve the register
	reg, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	// Set isnull to true if register has nil value or has type Null
	isnull := register.BoolValue(register.IsNullValue(reg))
	// Set the register
	scope.registers.Set(out, isnull)
}

func opCOPY(scope *ExecutionScope, operands []byte) {
	// Fetch the source and destination registers IDs
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	reg, exists := scope.registers.Get(source)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", source))

		return
	}

	// Set a copy of the register to the destination
	scope.registers.Set(destination, reg.Copy())
}

func opMOVE(scope *ExecutionScope, operands []byte) {
	// Fetch the source and destination registers IDs
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	reg, exists := scope.registers.Get(source)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", source))

		return
	}

	// Set a copy of the register to the destination
	scope.registers.Set(destination, reg.Copy())
	// UnSet() the register at source
	scope.registers.Unset(source)
}

func opGETIDX(scope *ExecutionScope, operands []byte) {
	// <reg:B> <reg:map[A]B> <reg:A>
	output, collection, index := operands[0], operands[1], operands[2]

	var (
		exists         bool
		regCol, regIdx register.Value
	)

	// Retrieve the register for collection
	if regCol, exists = scope.registers.Get(collection); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", collection))

		return
	}

	// Retrieve the register for index
	if regIdx, exists = scope.registers.Get(index); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", index))

		return
	}

	if !regCol.Type().Kind.IsCollection() {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a collection type", collection))

		return
	}

	// Cast the collection into a Collection
	collectionValue := regCol.(register.Collection) //nolint:forcetypeassert
	// Get the value from the collection
	element, err := collectionValue.Get(regIdx)
	if err != nil {
		scope.Throw(exception.Exception(exception.CollectionAccess, err.Error()))

		return
	}

	// Set the output register
	scope.registers.Set(output, element)
}

func opSETIDX(scope *ExecutionScope, operands []byte) {
	// <reg:map[A]B> <reg:A> <reg:B>
	collection, index, element := operands[0], operands[1], operands[2]

	var (
		exists                  bool
		regCol, regIdx, regElem register.Value
	)

	// Retrieve the register for collection
	if regCol, exists = scope.registers.Get(collection); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", collection))

		return
	}

	// Retrieve the register for index
	if regIdx, exists = scope.registers.Get(index); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", index))

		return
	}

	// Retrieve the register for element
	if regElem, exists = scope.registers.Get(element); !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", element))

		return
	}

	// Check if collection value has been initialized
	if register.IsNullValue(regCol) {
		scope.Throw(exception.Exception(exception.NilCollection, "cannot set to nil mapping"))

		return
	}

	if !regCol.Type().Kind.IsCollection() {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "$%v is not a collection type", collection))

		return
	}

	// Cast the collection into a Collection
	collectionValue := regCol.(register.Collection) //nolint:forcetypeassert
	// Set the element value to the collection
	if err := collectionValue.Set(regIdx, regElem); err != nil {
		scope.Throw(exception.Exception(exception.CollectionAccess, err.Error()))

		return
	}

	// Update the collection register
	scope.registers.Set(collection, collectionValue)
}

func opLT(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __lt__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodLt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __lt__", regA.Type()))

		return
	}

	// Execute the __lt__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(operands[0], result)
}

//nolint:dupl
func opLE(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __lt__ method for the register type
	methodLT, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodLt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __lt__", regA.Type()))

		return
	}

	// Retrieve the __eq__ method for the register type
	methodEQ, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodEq)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __eq__", regA.Type()))

		return
	}

	// Execute the __lt__ method
	outputsLT := methodLT.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Execute the __eq__ method
	outputsEQ := methodEQ.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs lt OR eq (as booleans)
	result := outputsLT[0].(register.BoolValue).Or(outputsEQ[0].(register.BoolValue)) //nolint:forcetypeassert
	// Set the register
	scope.registers.Set(operands[0], result)
}

func opGT(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __gt__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodGt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __gt__", regA.Type()))

		return
	}

	// Execute the __gt__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(operands[0], result)
}

//nolint:dupl
func opGE(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __gt__ method for the register type
	methodGT, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodGt)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __gt__", regA.Type()))

		return
	}

	// Retrieve the __eq__ method for the register type
	methodEQ, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodEq)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __eq__", regA.Type()))

		return
	}

	// Execute the __gt__ method
	outputsGT := methodGT.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Execute the __eq__ method
	outputsEQ := methodEQ.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs gt OR eq (as booleans)
	result := outputsGT[0].(register.BoolValue).Or(outputsEQ[0].(register.BoolValue)) //nolint:forcetypeassert
	// Set the register
	scope.registers.Set(operands[0], result)
}

func opEQ(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __eq__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodEq)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __eq__", regA.Type()))

		return
	}

	// Execute the __eq__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs
	result := outputs[0]
	// Set the register
	scope.registers.Set(operands[0], result)
}

func opNEQ(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for the inputs
	a, b := operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
	}

	// Retrieve the __eq__ method for the register type
	method, ok := scope.engine.GetTypeMethod(regA.Type(), register.MethodEq)
	if !ok {
		scope.Throw(exception.Exceptionf(exception.MethodNotFound, "%v does not implement __eq__", regA.Type()))

		return
	}

	// Execute the __eq__ method
	outputs := method.Execute(scope, register.ValueTable{0: regA, 1: regB})

	if scope.ExceptionThrown() {
		return
	}

	// Get the result from the outputs and invert it (as a boolean)
	result := outputs[0].(register.BoolValue).Not() //nolint:forcetypeassert
	// Set the register
	scope.registers.Set(operands[0], result)
}

func opINVERT(scope *ExecutionScope, operands []byte) {
	regID := operands[0]

	// Retrieve the register
	reg, exists := scope.registers.Get(regID)
	if !exists {
		scope.Throw(exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID))

		return
	}

	// Check that register is Boolean
	if !reg.Type().Equals(engineio.TypeBool) {
		scope.Throw(exception.Exceptionf(exception.InvalidRegisterType, "cannot INVERT register of type %v", reg.Type()))

		return
	}

	// Cast the register value to Bool and flip
	inverted := reg.(register.BoolValue).Not() //nolint:forcetypeassert
	// Set the register
	scope.registers.Set(regID, inverted)
}

//nolint:dupl
func opADD(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
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

		return
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "addition"))

		return
	}

	// Set the register
	scope.registers.Set(out, result)
}

//nolint:dupl
func opSUB(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
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

		return
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "subtraction"))

		return
	}

	// Set the register
	scope.registers.Set(out, result)
}

//nolint:dupl
func opMUL(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
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

		return
	}

	// Throw an exception if overflow occurred
	if overflow != nil {
		scope.Throw(exception.Exception(exception.ArithmeticOverflow, "multiplication"))

		return
	}

	// Set the register
	scope.registers.Set(out, result)
}

func opDIV(scope *ExecutionScope, operands []byte) {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]
	// Get two values of the same type
	regA, regB, except := scope.GetSymmetricValues(a, b)
	if except != nil {
		scope.Throw(except)

		return
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

		return
	}

	// Throw an exception if overflow occurred
	if matherr != nil {
		if errors.Is(matherr, register.ErrIntegerOverflow) {
			scope.Throw(exception.Exception(exception.ArithmeticOverflow, "division"))
		} else if errors.Is(matherr, register.ErrDivideByZero) {
			scope.Throw(exception.Exception(exception.ArithmeticDivideByZero, "division"))
		}

		return
	}

	// Set the register
	scope.registers.Set(out, result)
}
