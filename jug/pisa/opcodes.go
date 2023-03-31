package pisa

import (
	"bytes"
	"fmt"
)

// OpCode represents a PISA Opcode
type OpCode byte

// runtime operations
const (
	// TERM halts code execution
	TERM OpCode = 0x0
	// DEST marks a valid jump destination
	//  - JUMP <jump:ptr>
	DEST OpCode = 0x1
	// JUMP jumps and moves code execution to a jump destination.
	// The jump dest is a pointer representing the line of code to jump to.
	//  - JUMP <jump:ptr>
	JUMP OpCode = 0x2
	// JUMPI conditionally jumps and moves code execution to a jump destination.
	// The jump dest is a pointer representing the line of code to jump to.
	//  - JUMPI <reg:bool> <jump:ptr>
	JUMPI OpCode = 0x3

	// MAKE generates a register of a specified primitive type.
	// The type is specified with the primitive type ID
	//  - MAKE <reg:X> <byte:type-ID>
	MAKE OpCode = 0x4
	// LDPTR loads a pointer of specific size into a pointer register. Pointers can be between
	// 0 and 8 bytes long (max 64 bits). The pointer value si decoded from this data (big endian)
	// 	- LDPTR <byte:size of address> <reg:Ptr> <bytes[address]>
	LDPTR OpCode = 0x5

	// CONST converts a pointer register into a constant value. The value of the pointer is resolved
	// and the constant value is loaded into the same register replacing the pointer data.
	// 	- CONST <reg:Ptr>
	CONST OpCode = 0x6
	// BUILD converts a pointer register into a register of a certain type. The built type is resolved
	// from the type definition at the pointer. It is loaded into the same register replacing the pointer data
	// 	- TYPE <reg:Ptr>
	BUILD OpCode = 0x7
	// CALL calls a routine at the pointer with a specified number of args.
	// Each argument is a register whose type must match the routine calldata.
	// 	- CALL <reg:Ptr> <byte:no-of-inputs> <byte:no-of-outputs> <...registers:inputs...> <...registers:outputs...>
	CALL OpCode = 0x8
)

// environment operations
const (
	// ACCEPT accepts a value from an input slot into a register.
	// 	- ACCEPT <reg:X> <slot:byte>
	ACCEPT OpCode = 0x10
	// RETURN returns a value into an output slot from a register
	// 	- RETURN <reg:X> <slot:byte>
	RETURN OpCode = 0x11

	// EMIT emits an event to the logic event stream
	//  - EMIT <reg:emittable>
	EMIT OpCode = 0x12

	// LOAD loads a value from storage into a register
	//  - LOAD <reg:X> <slot:byte>
	LOAD OpCode = 0x13
	// STORE stores a values to storage from a register
	// 	- STORE <reg:X> <slot:byte>
	STORE OpCode = 0x14

	// CALLER loads the calling address into a register
	// 	- CALLER <reg:address>
	CALLER OpCode = 0x15
	// BALANCE loads the balance of an asset ID for the calling address into a register.
	//  - BALANCE <reg:bigint> <reg:string>
	BALANCE OpCode = 0x16
	// TIME loads the timestamp from the environment into a register
	// 	- TIME <reg:string>
	TIME OpCode = 0x17
)

// register methods
const (
	// INVOKE invokes a register method with a specified number of args.
	// Each argument is a register whose type must match the method calldata.
	//  - INVOKE <reg:X> <byte[method]> <byte[inputno]> <...registers:inputs...> <byte[outputno]> <...registers:outputs...>
	INVOKE OpCode = 0x20
	// THROW throws an exception to the VM.
	// The register must be class that implements __throw__. If there is no catch specified for the line
	// in which the instruction is thrown, it aborts execution and exits, otherwise execution moves to the
	// location specified by the catch
	//	- THROW <reg:throwable>
	THROW OpCode = 0x21

	// BOOL returns a boolean value for the register. The register type must implement __bool__.
	// string(if "" false, else true), bytes, address (if 0x0 false, else true), integer(if 0 false, else true),
	// boolean(direct value), sequence, mapping(if len==0 false, else true), class(value from __bool__ implementation)
	// 	- BOOL <reg:X>
	BOOL OpCode = 0x22
	// STR returns a string value for the register. The register type must implement __string__.
	// 	- STR <reg:X>
	STR OpCode = 0x23

	// JOIN joins the contents of two registers into another.
	// They must be of the same type and implement __join__. Classes will be joined per this method.
	// bytes and string (concatenation), integer (addition), boolean (and), sequence (append), mapping (merge).
	// 	- JOIN <reg:A> <reg:A> <reg:A>
	JOIN OpCode = 0x25
)

// register and binding operations
const (
	// TYPEOF loads the datatype of a register into another as a typedef
	TYPEOF OpCode = 0x32
	// ISNULL returns whether a register is empty (null)
	ISNULL OpCode = 0x33

	// COPY copies the contents of a register into another, retaining it in the original location.
	// 	- COPY <reg:dest> <reg:src>
	COPY OpCode = 0x3C
	// MOVE moves the contents of a register into another, removing it from the original location.
	// 	- MOVE <reg:dest> <reg:src>
	MOVE OpCode = 0x3D
	// SWAP swaps the contents of two registers.
	// 	- SWAP <reg:X> <reg:Y>
	SWAP OpCode = 0x3E
	// CLEAR removes the contents of a register
	// 	- CLEAR <reg:X>
	CLEAR OpCode = 0x3F
)

// collection operators
const (
	// GETIDX gets the value at a given index for the collection.
	// The collection may be sequence or mapping with the index being the appropriate index type.
	// 	- GETIDX <reg:A> <reg:sequence[A]> <reg:int64>
	// 	- GETIDX <reg:B> <reg:mapping[A->B]> <reg:A>
	GETIDX OpCode = 0x40
	// SETIDX sets the value at a given index in the collection.
	// Collection may be sequence or mapping with the index being the appropriate index type.
	// 	- SETIDX <reg:sequence[A]> <reg:int64> <reg:A>
	// 	- SETIDX <reg:mapping[A->B]> <reg:A> <reg:B>
	SETIDX OpCode = 0x41
)

// comparison and arithmetic operators
const (
	// LT compares two registers (less than) and returns a boolean.
	// Classes must implement __lt__.
	//	- LT <reg:bool> <reg:A> <reg:A>
	LT OpCode = 0x50
	// LE compares two registers (less than or equal) and returns a boolean.
	// Classes must implement __lt__ and __eq__.
	//	- LE <reg:bool> <reg:A> <reg:A>
	LE OpCode = 0x51
	// GT compares two registers (greater than) and returns a boolean.
	// Classes must implement __gt__.
	//	- GT <reg:bool> <reg:A> <reg:A>
	GT OpCode = 0x52
	// GE compares two registers (greater than or equal) and returns a boolean.
	// Classes must implement __gt__ and __eq__.
	//	- GE <reg:bool> <reg:A> <reg:A>
	GE OpCode = 0x53
	// EQ compares two registers (equals) and returns a boolean.
	// Classes must implement __eq__.
	//	- EQ <reg:bool> <reg:A> <reg:A>
	EQ OpCode = 0x54
	// NEQ compares two registers (not equals) and returns a boolean.
	// Classes must implement __eq__.
	//	- NEQ <reg:bool> <reg:A> <reg:A>
	NEQ OpCode = 0x55

	// INVERT flips a boolean value
	//  - INVERT <reg:bool>
	INVERT OpCode = 0x56
	// INCR increments the value of a numeric register by 1.
	//  - INCR <reg:numeric>
	INCR OpCode = 0x57
	// DECR decrements the value of a numeric register by 1.
	//  - DECR <reg:numeric>
	DECR OpCode = 0x58

	// ADD applies the add operation on two numeric registers (same type)
	// and returns another numeric of the same type.
	//  - ADD <reg:numeric> <reg:numeric> <reg:numeric>
	ADD OpCode = 0x59
	// SUB applies the subtract operation on two numeric registers (same type)
	// and returns another numeric of the same type.
	//  - SUB <reg:numeric> <reg:numeric> <reg:numeric>
	SUB OpCode = 0x5A
	// MUL applies the multiply operation on two numeric registers (same type)
	// and returns another numeric of the same type.
	//  - MUL <reg:numeric> <reg:numeric> <reg:numeric>
	MUL OpCode = 0x5B
	// DIV applies the division on two numeric registers (same type)
	// and returns another numeric of the same type.
	//  - DIV <reg:numeric> <reg:numeric> <reg:numeric>
	DIV OpCode = 0x5C
	// MOD applies the modulo division operation on two numeric registers (same type)
	// and returns another numeric of the same type.
	//  - MOD <reg:numeric> <reg:numeric> <reg:numeric>
	MOD OpCode = 0x5D
)

var opCodeToString = map[OpCode]string{
	TERM:  "TERM",
	DEST:  "DEST",
	JUMP:  "JUMP",
	JUMPI: "JUMPI",
	MAKE:  "MAKE",
	LDPTR: "LDPTR",
	CONST: "CONST",
	BUILD: "BUILD",
	CALL:  "CALL",

	ACCEPT:  "ACCEPT",
	RETURN:  "RETURN",
	EMIT:    "EMIT",
	LOAD:    "LOAD",
	STORE:   "STORE",
	CALLER:  "CALLER",
	BALANCE: "BALANCE",
	TIME:    "TIME",

	INVOKE: "INVOKE",
	THROW:  "THROW",
	BOOL:   "BOOL",
	STR:    "STR",
	JOIN:   "JOIN",

	TYPEOF: "TYPEOF",
	ISNULL: "ISNULL",

	COPY:  "COPY",
	MOVE:  "MOVE",
	SWAP:  "SWAP",
	CLEAR: "CLEAR",

	GETIDX: "GETIDX",
	SETIDX: "SETIDX",

	LT:  "LT",
	LE:  "LE",
	GT:  "GT",
	GE:  "GE",
	EQ:  "EQ",
	NEQ: "NEQ",

	INVERT: "INVERT",
	INCR:   "INCR",
	DECR:   "DECR",

	ADD: "ADD",
	SUB: "SUB",
	MUL: "MUL",
	DIV: "DIV",
	MOD: "MOD",
}

// String returns the string representation of OpCode.
// Returns an empty string for an undefined opcode.
// It implements the Stringer interface for OpCode.
func (op OpCode) String() string {
	str := opCodeToString[op]
	if len(str) == 0 {
		return fmt.Sprintf("undefined opcode [%#x]", int(op))
	}

	return str
}

var stringToOpCode = map[string]OpCode{
	"TERM":  TERM,
	"DEST":  DEST,
	"JUMP":  JUMP,
	"JUMPI": JUMPI,
	"MAKE":  MAKE,
	"LDPTR": LDPTR,
	"CONST": CONST,
	"BUILD": BUILD,
	"CALL":  CALL,

	"ACCEPT":  ACCEPT,
	"RETURN":  RETURN,
	"EMIT":    EMIT,
	"LOAD":    LOAD,
	"STORE":   STORE,
	"CALLER":  CALLER,
	"BALANCE": BALANCE,
	"TIME":    TIME,

	"INVOKE": INVOKE,
	"THROW":  THROW,
	"BOOL":   BOOL,
	"STR":    STR,
	"JOIN":   JOIN,

	"TYPEOF": TYPEOF,
	"ISNULL": ISNULL,

	"COPY":  COPY,
	"MOVE":  MOVE,
	"SWAP":  SWAP,
	"CLEAR": CLEAR,

	"GETIDX": GETIDX,
	"SETIDX": SETIDX,

	"LT":  LT,
	"LE":  LE,
	"GT":  GT,
	"GE":  GE,
	"EQ":  EQ,
	"NEQ": NEQ,

	"INVERT": INVERT,
	"INCR":   INCR,
	"DECR":   DECR,

	"ADD": ADD,
	"SUB": SUB,
	"MUL": MUL,
	"DIV": DIV,
	"MOD": MOD,
}

// StringToOpCode finds the opcode whose name is stored in str.
func StringToOpCode(str string) OpCode {
	return stringToOpCode[str]
}

// operand0 reads 0 bytes from the reader as operands.
func operand0(_ *bytes.Reader) ([]byte, bool) {
	return nil, true
}

// operand1 reads 1 bytes from the reader as operands.
func operand1(reader *bytes.Reader) ([]byte, bool) {
	operand, err := reader.ReadByte()
	if err != nil {
		return nil, false
	}

	return []byte{operand}, true
}

// operand2 reads 2 bytes from the reader as operands.
func operand2(reader *bytes.Reader) ([]byte, bool) {
	operands := make([]byte, 2)
	read, err := reader.Read(operands)

	if read != 2 || err != nil {
		return nil, false
	}

	return operands, true
}

// operand3 reads 3 bytes from the reader as operands.
func operand3(reader *bytes.Reader) ([]byte, bool) {
	operands := make([]byte, 3)
	read, err := reader.Read(operands)

	if read != 3 || err != nil {
		return nil, false
	}

	return operands, true
}

// operandLDPTR reads bytes from the reader for the LDPTR opcode.
// LDPTR takes a variable number of operands. The first operand specifies the size of the pointer to read.
// The second operand specifies the register to load the pointer into, followed by n bytes for the pointer.
func operandLDPTR(reader *bytes.Reader) ([]byte, bool) {
	var (
		err     error
		regID   byte
		ptrSize byte
	)

	if ptrSize, err = reader.ReadByte(); err != nil {
		return nil, false
	}

	if regID, err = reader.ReadByte(); err != nil {
		return nil, false
	}

	if ptrSize == 0 {
		return []byte{regID, 1, 0}, true
	}

	address := make([]byte, ptrSize)
	if n, err := reader.Read(address); n != int(ptrSize) || err != nil {
		return nil, false
	}

	operands := make([]byte, 0, ptrSize+2)
	operands = append(operands, ptrSize, regID)
	operands = append(operands, address...)

	return operands, true
}
