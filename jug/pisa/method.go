package pisa

import (
	"fmt"

	"github.com/pkg/errors"
)

const MaxSpecialMethod = MethodCode(0xF)

// Method is extension of the Runnable interface and
// represents runnable methods on primitive and class types.
type Method interface {
	Runnable

	code() MethodCode
	datatype() Datatype
}

// MethodCode represents a unique byte identifier for the method of a type.
// The first 16 bytes (0x00 - 0x0F) are reserved as special method codes.
type MethodCode byte

const (
	MethodBuild MethodCode = 0x0
	MethodThrow MethodCode = 0x1
	MethodEmit  MethodCode = 0x2
	MethodJoin  MethodCode = 0x3

	MethodLt MethodCode = 0x4
	MethodGt MethodCode = 0x5
	MethodEq MethodCode = 0x6

	MethodBool MethodCode = 0x7
	MethodStr  MethodCode = 0x8
	MethodAddr MethodCode = 0x9
	MethodLen  MethodCode = 0xA
)

var methodCodeToString = map[MethodCode]string{
	MethodBuild: "__build__",
	MethodThrow: "__throw__",
	MethodEmit:  "__emit__",
	MethodJoin:  "__join__",

	MethodLt: "__lt__",
	MethodGt: "__gt__",
	MethodEq: "__eq__",

	MethodBool: "__bool__",
	MethodStr:  "__str__",
	MethodAddr: "__addr__",
	MethodLen:  "__len__",
}

// String returns a string representation of the primitive.
// It implements the Stringer interface for primitive
func (method MethodCode) String() string {
	str, ok := methodCodeToString[method]
	if !ok {
		return fmt.Sprintf("method(%#x)", int(method))
	}

	return str
}

var methodValidators = map[MethodCode]func(Method) error{
	// MethodBuild
	MethodThrow: validateThrowMethod,
	// MethodEmit:
	MethodJoin: validateJoinMethod,
	MethodLt:   validateLtMethod,
	MethodGt:   validateGtMethod,
	MethodEq:   validateEqMethod,
	MethodBool: validateBoolMethod,
	MethodStr:  validateStrMethod,
	MethodAddr: validateAddrMethod,
	MethodLen:  validateLenMethod,
}

func validateMethodFieldSelf(method Method) error {
	if method.callfields().Inputs.Size() == 0 {
		return errors.New("invalid method inputs: missing 'self' field")
	}

	if method.callfields().Inputs.Get(0).Name != "self" {
		return errors.New("invalid method inputs: first field must be 'self'")
	}

	if !method.callfields().Inputs.Get(0).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method inputs: first field must be of type '%v'", method.datatype())
	}

	return nil
}

func validateThrowMethod(method Method) error {
	// Code must be MethodThrow [0x1]
	if method.code() != MethodThrow {
		return errors.Errorf("invalid method code: must be %#x", MethodThrow)
	}

	// Name must be __throw__
	if method.name() != MethodThrow.String() {
		return errors.Errorf("invalid method name: must be %v", MethodThrow.String())
	}

	// Must have exactly 1 input
	if method.callfields().Inputs.Size() != 1 {
		return errors.New("invalid method inputs: must have exactly 1 field")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have exactly 1 output, and it must be a string
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveString) {
		return errors.New("invalid method outputs: first field must be of type 'string'")
	}

	return nil
}

func validateJoinMethod(method Method) error {
	// Code must be MethodJoin [0x3]
	if method.code() != MethodJoin {
		return errors.Errorf("invalid method code: must be %#x", MethodThrow)
	}

	// Name must be __join__
	if method.name() != MethodJoin.String() {
		return errors.Errorf("invalid method name: must be %v", MethodThrow.String())
	}

	// Must have exactly 2 inputs
	if method.callfields().Inputs.Size() != 2 {
		return errors.New("invalid method inputs: must have exactly 2 fields")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have second input field be that of the method's datatype
	if !method.callfields().Inputs.Get(1).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method inputs: second field must be of type '%v'", method.datatype())
	}

	// Must have exactly 1 output, with type being that of the method's datatype
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method outputs: first field must be of type '%v'", method.datatype())
	}

	return nil
}

//nolint:dupl
func validateLtMethod(method Method) error {
	// Code must be MethodLt [0x4]
	if method.code() != MethodLt {
		return errors.Errorf("invalid method code: must be %#x", MethodLt)
	}

	// Name must be __lt__
	if method.name() != MethodLt.String() {
		return errors.Errorf("invalid method name: must be %v", MethodLt.String())
	}

	// Must have exactly 2 inputs
	if method.callfields().Inputs.Size() != 2 {
		return errors.New("invalid method inputs: must have exactly 2 fields")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have second input field be that of the method's datatype
	if !method.callfields().Inputs.Get(1).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method inputs: second field must be of type '%v'", method.datatype())
	}

	// Must have exactly 1 output, with type being bool
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveBool) {
		return errors.New("invalid method outputs: first field must be of type 'boolean'")
	}

	return nil
}

//nolint:dupl
func validateGtMethod(method Method) error {
	// Code must be MethodGt [0x5]
	if method.code() != MethodGt {
		return errors.Errorf("invalid method code: must be %#x", MethodGt)
	}

	// Name must be __gt__
	if method.name() != MethodGt.String() {
		return errors.Errorf("invalid method name: must be %v", MethodGt.String())
	}

	// Must have exactly 2 inputs
	if method.callfields().Inputs.Size() != 2 {
		return errors.New("invalid method inputs: must have exactly 2 fields")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have second input field be that of the method's datatype
	if !method.callfields().Inputs.Get(1).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method inputs: second field must be of type '%v'", method.datatype())
	}

	// Must have exactly 1 output, with type being bool
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveBool) {
		return errors.New("invalid method outputs: first field must be of type 'boolean'")
	}

	return nil
}

//nolint:dupl
func validateEqMethod(method Method) error {
	// Code must be MethodEq [0x6]
	if method.code() != MethodEq {
		return errors.Errorf("invalid method code: must be %#x", MethodEq)
	}

	// Name must be __eq__
	if method.name() != MethodEq.String() {
		return errors.Errorf("invalid method name: must be %v", MethodEq.String())
	}

	// Must have exactly 2 inputs
	if method.callfields().Inputs.Size() != 2 {
		return errors.New("invalid method inputs: must have exactly 2 fields")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have second input field be that of the method's datatype
	if !method.callfields().Inputs.Get(1).Type.Equals(method.datatype()) {
		return errors.Errorf("invalid method inputs: second field must be of type '%v'", method.datatype())
	}

	// Must have exactly 1 output, with type being bool
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveBool) {
		return errors.New("invalid method outputs: first field must be of type 'boolean'")
	}

	return nil
}

func validateBoolMethod(method Method) error {
	// Code must be MethodBool [0x7]
	if method.code() != MethodBool {
		return errors.Errorf("invalid method code: must be %#x", MethodBool)
	}

	// Name must be __bool__
	if method.name() != MethodBool.String() {
		return errors.Errorf("invalid method name: must be %v", MethodBool.String())
	}

	// Must have exactly 1 input
	if method.callfields().Inputs.Size() != 1 {
		return errors.New("invalid method inputs: must have exactly 1 field")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have exactly 1 output, with type being bool
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveBool) {
		return errors.New("invalid method outputs: first field must be of type 'boolean'")
	}

	return nil
}

func validateStrMethod(method Method) error {
	// Code must be MethodStr [0x8]
	if method.code() != MethodStr {
		return errors.Errorf("invalid method code: must be %#x", MethodStr)
	}

	// Name must be __str__
	if method.name() != MethodStr.String() {
		return errors.Errorf("invalid method name: must be %v", MethodStr.String())
	}

	// Must have exactly 1 input
	if method.callfields().Inputs.Size() != 1 {
		return errors.New("invalid method inputs: must have exactly 1 field")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have exactly 1 output, with type being string
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveString) {
		return errors.New("invalid method outputs: first field must be of type 'string'")
	}

	return nil
}

func validateAddrMethod(method Method) error {
	// Code must be MethodAddr [0x9]
	if method.code() != MethodAddr {
		return errors.Errorf("invalid method code: must be %#x", MethodAddr)
	}

	// Name must be __addr__
	if method.name() != MethodAddr.String() {
		return errors.Errorf("invalid method name: must be %v", MethodAddr.String())
	}

	// Must have exactly 1 input
	if method.callfields().Inputs.Size() != 1 {
		return errors.New("invalid method inputs: must have exactly 1 field")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have exactly 1 output, with type being address
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveAddress) {
		return errors.New("invalid method outputs: first field must be of type 'address'")
	}

	return nil
}

func validateLenMethod(method Method) error {
	// Code must be MethodLen [0x9]
	if method.code() != MethodLen {
		return errors.Errorf("invalid method code: must be %#x", MethodLen)
	}

	// Name must be __len__
	if method.name() != MethodLen.String() {
		return errors.Errorf("invalid method name: must be %v", MethodLen.String())
	}

	// Must have exactly 1 input
	if method.callfields().Inputs.Size() != 1 {
		return errors.New("invalid method inputs: must have exactly 1 field")
	}

	// Must have 'self' as the first input with type being that of the method's datatype
	if err := validateMethodFieldSelf(method); err != nil {
		return err
	}

	// Must have exactly 1 output, with type being u64
	if method.callfields().Outputs.Size() != 1 {
		return errors.New("invalid method outputs: must have exactly 1 field")
	}

	if !method.callfields().Outputs.Get(0).Type.Equals(PrimitiveU64) {
		return errors.New("invalid method outputs: first field must be of type 'u64'")
	}

	return nil
}
