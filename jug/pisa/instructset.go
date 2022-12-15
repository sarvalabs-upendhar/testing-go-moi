package pisa

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
)

// InstructionSet represents the opcode instructions for the PISA Runtime
type InstructionSet [256]*instructOp

// instructOp represents the runtime/compile logic for a single PISA OpCode.
type instructOp struct {
	// operand specifies a function for reading the operands for an OpCode from a bytes.Reader.
	// The function returns the slice of operands as well as a bool indicating a successful read.
	operand func(*bytes.Reader) ([]byte, bool)
	// execute specifies a function for executing an OpCode within a given Context for some operands.
	execute func(*ExecutionContext, []byte) error
	// expense specifies a function for calculating the fuel consumption of an OpCode.
	expense func(*ExecutionContext) uint64
}

// BaseInstructionSet returns an InstructionSet with all the base opcodes and their instructions initialized.
func BaseInstructionSet() InstructionSet {
	return InstructionSet{
		TERM:  {operand0, opTERM, fuel},
		DEST:  {operand0, opDEST, fuel},
		JUMPI: {operand2, opJUMPI, fuel},

		MAKE:  {operand2, opMAKE, fuel},
		LDPTR: {operandLDPTR, opLDPTR, fuel},
		CONST: {operand1, opCONST, fuel},
		BUILD: {operand1, opBUILD, fuel},

		ACCEPT: {operand2, opACCEPT, fuel},
		RETURN: {operand2, opRETURN, fuel},

		LOAD:  {operand2, opLOAD, fuel},
		STORE: {operand2, opSTORE, fuel},

		ISNULL: {operand2, opISNULL, fuel},

		COPY: {operand2, opCOPY, fuel},
		MOVE: {operand2, opMOVE, fuel},

		GETIDX: {operand3, opGETIDX, fuel},
		SETIDX: {operand3, opSETIDX, fuel},

		LT: {operand3, opLT, fuel},
		GT: {operand3, opGT, fuel},
		EQ: {operand3, opEQ, fuel},

		INVERT: {operand1, opINVERT, fuel},

		ADD: {operand3, opADD, fuel},
		SUB: {operand3, opSUB, fuel},
	}
}

// lookup returns the instructOp for a given OpCode.
// Returns nil, if there is no defined instruction for it.
func (instructs InstructionSet) lookup(opcode OpCode) *instructOp {
	return instructs[opcode]
}

// fuel is a standard fuel function that deducts 10 FUEL
func fuel(_ *ExecutionContext) uint64 {
	return 10
}

var ErrTerminate = errors.New("terminate execution")

func opTERM(ctx *ExecutionContext, operands []byte) error { return ErrTerminate }

func opDEST(_ *ExecutionContext, _ []byte) error {
	return nil
}

func opJUMPI(ctx *ExecutionContext, operands []byte) error {
	condition, destination := operands[0], operands[1]

	// Retrieve the condition register
	regCondition, exists := ctx.registers.get(condition)
	if !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", condition)
	}

	// Check that register is Boolean
	if !regCondition.Type().Equals(TypeBool) {
		return errors.Errorf("Exception: UnsupportedOp: cannot perform JUMPI with non-bool registers")
	}

	// If condition is false, no jump
	if !regCondition.Value.(BoolValue) { //nolint:forcetypeassert
		return nil
	}

	// Load the pointer value in the register
	pointer, err := ctx.GetPtrValue(destination)
	if err != nil {
		return err
	}

	if err = ctx.program.jump(pointer); err != nil {
		return err
	}

	return nil
}

func opMAKE(ctx *ExecutionContext, operands []byte) error {
	// Fetch the target register and the type ID
	output, typeID := operands[0], operands[1]

	// Check if type ID is valid
	if typeID > MaxTypeID {
		return errors.New("invalid type ID for MAKE")
	}

	// Create a datatype from the type ID
	datatype := PrimitiveType(typeID).Datatype()
	// Create a value for the datatype
	value, err := NewValue(datatype, nil)
	if err != nil {
		return err
	}

	// Set the register value
	ctx.registers.set(output, NewRegister(value))

	return nil
}

func opLDPTR(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register ID and pointer value
	target, pointerData := operands[1], operands[2:]

	// Decipher constant ID into 64-bit address
	pointerVal, err := ptrdecode(pointerData)
	if err != nil {
		return err
	}

	// Create a new Pointer value
	pointer, err := NewPointer(pointerVal)
	if err != nil {
		return err
	}

	// Set the register value
	ctx.registers.set(target, NewRegister(pointer))

	return nil
}

func opCONST(ctx *ExecutionContext, operands []byte) error {
	// Fetch the registers ID
	regID := operands[0]

	// Load the pointer value in the register
	pointer, err := ctx.GetPtrValue(regID)
	if err != nil {
		return err
	}

	// Load the constant into the engine
	if err = ctx.engine.loadConstant(pointer); err != nil {
		return err
	}

	// Lookup the constant from the logic table
	constant, _ := ctx.engine.constants.fetch(pointer)
	// Create value from the constant definition
	constVal, err := NewConstantValue(constant)
	if err != nil {
		return err
	}

	// Set the constant value into the register
	ctx.registers.set(regID, NewRegister(constVal))

	return nil
}

func opBUILD(ctx *ExecutionContext, operands []byte) error {
	// Fetch the registers ID
	regID := operands[0]

	// Load the pointer value in the register
	pointer, err := ctx.GetPtrValue(regID)
	if err != nil {
		return err
	}

	// Load the constant into the engine
	if err = ctx.engine.loadTypedefs(pointer); err != nil {
		return err
	}

	typedef := ctx.engine.datatypes.symbolic[pointer]
	typevalue, _ := NewValue(typedef, nil)

	// Set the constant value into the register
	ctx.registers.set(regID, NewRegister(typevalue))

	return nil
}

func opACCEPT(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register ID and load slot
	regID, slot := operands[0], operands[1]

	// Retrieve the calldata value
	val := ctx.inputs[slot]
	// Set the register value
	ctx.registers.set(regID, NewRegister(val))

	return nil
}

func opRETURN(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register ID and return slot
	regID, slot := operands[0], operands[1]

	// Retrieve the register
	reg, exists := ctx.registers.get(regID)
	if !exists {
		return errors.Errorf("could not find register [%v] for RETURN", regID)
	}

	// Set the calldata value
	ctx.outputs[slot] = reg.Value

	return nil
}

func opLOAD(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register ID and storage slot
	regID, slot := operands[0], operands[1]

	slotType := ctx.engine.storage.layout.fetch(slot)
	if slotType == nil {
		return errors.New("invalid storage slot")
	}

	slotValue, err := ctx.engine.storage.callee.GetStorageEntry(ctx.engine.logic.LogicID(), SlotHash(slot))
	if err != nil {
		return errors.Errorf("no storage data found for slot [%v]", slot)
	}

	val, err := NewValue(slotType.Type, slotValue)
	if err != nil {
		return err
	}

	// Set the register value
	ctx.registers.set(regID, NewRegister(val))

	return nil
}

func opSTORE(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register ID and storage slot
	regID, slot := operands[0], operands[1]

	// Retrieve the register
	reg, exists := ctx.registers.get(regID)
	if !exists {
		return errors.Errorf("could not find register [%v] for STORE", regID)
	}

	if err := ctx.engine.storage.callee.SetStorageEntry(
		ctx.engine.logic.LogicID(), SlotHash(slot), reg.Value.Data()); err != nil {
		return errors.New("could not write to storage")
	}

	return nil
}

func opISNULL(ctx *ExecutionContext, operands []byte) error {
	// Fetch the registers IDs
	out, regID := operands[0], operands[1]

	// Retrieve the register
	reg, exists := ctx.registers.get(regID)
	if !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", regID)
	}

	// Set isnull to true if register has nil value
	var isnull BoolValue
	if reg.Value == nil {
		isnull = true
	}

	// Set the register
	ctx.registers.set(out, NewRegister(isnull))

	return nil
}

func opCOPY(ctx *ExecutionContext, operands []byte) error {
	// Fetch the source and destination registers IDs
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	if reg, exists := ctx.registers.get(source); exists {
		// Set a copy of the register to the destination
		ctx.registers.set(destination, reg.copy())
	}

	return nil
}

func opMOVE(env *ExecutionContext, operands []byte) error {
	// Fetch the source and destination registers IDs
	destination, source := operands[0], operands[1]

	// Retrieve the register at source
	if reg, exists := env.registers.get(source); exists {
		// Set a copy of the register to the destination
		env.registers.set(destination, reg.copy())
		// Unset the register at source
		env.registers.unset(source)
	}

	return nil
}

func opGETIDX(ctx *ExecutionContext, operands []byte) error {
	// <reg:B> <reg:map[A]B> <reg:A>
	output, collection, index := operands[0], operands[1], operands[2]

	var (
		exists         bool
		regCol, regIdx Register
	)

	// Retrieve the register for collection
	if regCol, exists = ctx.registers.get(collection); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", collection)
	}

	// Retrieve the register for index
	if regIdx, exists = ctx.registers.get(index); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", index)
	}

	// temporary: only mapping types supported
	if regCol.Type().Kind() != Hashmap {
		return errors.Errorf("Exception: InvalidType: Non Mapping Type [%v]", collection)
	}

	// Cast the collection into a MapValue
	mapping := regCol.Value.(*MapValue) //nolint:forcetypeassert
	// Get the value of the key from the map
	value, err := mapping.Get(regIdx.Value)
	if err != nil {
		return errors.Errorf("Exception: InvalidMapRead: %v", err)
	}

	// Set the output register
	ctx.registers.set(output, NewRegister(value))

	return nil
}

func opSETIDX(ctx *ExecutionContext, operands []byte) error {
	// <reg:map[A]B> <reg:A> <reg:B>
	collection, index, value := operands[0], operands[1], operands[2]

	var (
		exists                 bool
		regCol, regIdx, regVal Register
	)

	// Retrieve the register for collection
	if regCol, exists = ctx.registers.get(collection); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", collection)
	}

	// Retrieve the register for index
	if regIdx, exists = ctx.registers.get(index); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", index)
	}

	// Retrieve the register for value
	if regVal, exists = ctx.registers.get(value); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", value)
	}

	// Check if collection value has been initialized
	if regCol.Value == nil {
		return errors.Errorf("Exception: NilAccess: Cannot SETIDX on Nil Collection")
	}

	// temporary: only mapping types supported
	if regCol.Type().Kind() != Hashmap {
		return errors.Errorf("Exception: InvalidType: Non Mapping Type [%v]", collection)
	}

	// Cast the collection into a MapValue
	mapping := regCol.Value.(*MapValue) //nolint:forcetypeassert
	// Set the value of the key to the map
	if err := mapping.Set(regIdx.Value, regVal.Value); err != nil {
		return errors.Errorf("Exception: InvalidMapWrite: %v", err)
	}

	// Update the collection register
	ctx.registers.set(collection, NewRegister(mapping))

	return nil
}

func opLT(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]

	var (
		exists bool
		regA   Register
		regB   Register
	)

	// Retrieve the register for A
	if regA, exists = ctx.registers.get(a); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Retrieve the register for B
	if regB, exists = ctx.registers.get(b); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return errors.Errorf("Exception: UnequalType: [%v, %v]", a, b)
	}

	// temporary check: other types are unimplemented
	// todo: this will instead call __lt__ method for types
	if !regA.Type().Equals(TypeU64) {
		return errors.Errorf("Exception: UnsupportedCompare: cannot perform LT on registers with unsupported type")
	}

	// Cast register values to U64 and call Lt
	result := regA.Value.(U64Value).Lt(regB.Value.(U64Value)) //nolint:forcetypeassert
	// Set the register
	ctx.registers.set(out, NewRegister(result))

	return nil
}

func opGT(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]

	var (
		exists bool
		regA   Register
		regB   Register
	)

	// Retrieve the register for A
	if regA, exists = ctx.registers.get(a); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Retrieve the register for B
	if regB, exists = ctx.registers.get(b); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	fmt.Println(regA.Value, regB.Value)

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return errors.Errorf("Exception: UnequalType: [%v, %v]", a, b)
	}

	// temporary check: other types are unimplemented
	// todo: this will instead call __lt__ method for types
	if !regA.Type().Equals(TypeU64) {
		return errors.Errorf("Exception: UnsupportedCompare: cannot perform GT on registers with unsupported type")
	}

	// Cast register values to U64 and call Gt
	result := regA.Value.(U64Value).Gt(regB.Value.(U64Value)) //nolint:forcetypeassert
	// Set the register
	ctx.registers.set(out, NewRegister(result))

	return nil
}

func opEQ(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]

	var (
		exists bool
		regA   Register
		regB   Register
	)

	// Retrieve the register for A
	if regA, exists = ctx.registers.get(a); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Retrieve the register for B
	if regB, exists = ctx.registers.get(b); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return errors.Errorf("Exception: UnequalType: [%v, %v]", a, b)
	}

	// temporary check: other types are unimplemented
	// todo: this will instead call __lt__ method for types
	if !regA.Type().Equals(TypeU64) {
		return errors.Errorf("Exception: UnsupportedCompare: cannot perform Eq on registers with unsupported type")
	}

	// Cast register values to U64 and call Eq
	result := regA.Value.(U64Value).Eq(regB.Value.(U64Value)) //nolint:forcetypeassert
	// Set the register
	ctx.registers.set(out, NewRegister(result))

	return nil
}

func opINVERT(ctx *ExecutionContext, operands []byte) error {
	regID := operands[0]

	// Retrieve the register
	reg, exists := ctx.registers.get(regID)
	if !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", regID)
	}

	// Check that register is Boolean
	if !reg.Type().Equals(TypeBool) {
		return errors.Errorf("Exception: UnsupportedOp: cannot perform INVERT on non-bool registers")
	}

	// Cast the register value to Bool and flip
	invert := reg.Value.(BoolValue).Not() //nolint:forcetypeassert
	// Set the register
	ctx.registers.set(regID, NewRegister(invert))

	return nil
}

//nolint:dupl
func opADD(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]

	var (
		exists bool
		regA   Register
		regB   Register
	)

	// Retrieve the register for A
	if regA, exists = ctx.registers.get(a); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Retrieve the register for B
	if regB, exists = ctx.registers.get(b); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return errors.Errorf("Exception: UnequalType: [%v, %v]", a, b)
	}

	// Check that register types are numeric
	if !regA.Type().P.Numeric() {
		return errors.Errorf("Exception: InvalidType: Non Numeric [%v, %v]", a, b)
	}

	// temporary check: other numeric types are unimplemented
	if !regA.Type().Equals(TypeU64) {
		return errors.Errorf(
			"Exception: UnsupportedArithmetic: cannot perform ADD on registers with unsupported numeric type",
		)
	}

	// Cast register values to U64 and call Add (check for overflow)
	result, err := regA.Value.(U64Value).Add(regB.Value.(U64Value))
	if err != nil {
		return errors.Errorf("Exception: IntegerOverflow: ADD on registers [%v, %v]", a, b)
	}

	// Set the register
	ctx.registers.set(out, NewRegister(result))

	return nil
}

//nolint:dupl
func opSUB(ctx *ExecutionContext, operands []byte) error {
	// Fetch the register IDs for output and inputs
	out, a, b := operands[0], operands[1], operands[2]

	var (
		exists bool
		regA   Register
		regB   Register
	)

	// Retrieve the register for A
	if regA, exists = ctx.registers.get(a); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Retrieve the register for B
	if regB, exists = ctx.registers.get(b); !exists {
		return errors.Errorf("Exception: EmptyRegister: [%v]", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return errors.Errorf("Exception: UnequalType: [%v, %v]", a, b)
	}

	// Check that register types are numeric
	if !regA.Type().P.Numeric() {
		return errors.Errorf("Exception: InvalidType: Non Numeric [%v, %v]", a, b)
	}

	// temporary check: other numeric types are unimplemented
	if !regA.Type().Equals(TypeU64) {
		return errors.Errorf(
			"Exception: UnsupportedArithmetic: cannot perform ADD on registers with unsupported numeric type",
		)
	}

	// Cast register values to U64 and call Sub (check for overflow)
	result, err := regA.Value.(U64Value).Sub(regB.Value.(U64Value))
	if err != nil {
		return errors.Errorf("Exception: IntegerOverflow: SUB on registers [%v, %v]", a, b)
	}

	// Set the register
	ctx.registers.set(out, NewRegister(result))

	return nil
}
