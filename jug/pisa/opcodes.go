package pisa

import (
	"fmt"
)

// OpCode represents a PISA Opcode
type OpCode byte

// Control Flow Opcodes
const (
	// TERM [0] Terminate execution
	TERM OpCode = 0x0
	// DEST [0] Mark jump destination site
	DEST OpCode = 0x1
	// JUMP [1] Jump unconditionally to the instruction pointer in register $X
	// - JUMP [$X: ptr]
	JUMP OpCode = 0x2
	// JUMPI [2] Jump to the instruction pointer in register $X if the $Y.__bool__() is true
	// - JUMPI [$X: ptr][$Y: __bool__]
	JUMPI OpCode = 0x3

	// OBTAIN [2] Obtain a value from the accept slot &Y and store it in the register $X
	// - OBTAIN [$X][&Y]
	OBTAIN OpCode = 0x4
	// YIELD [2] Yield a value in the register $X to the return slot &Y
	// - YIELD [$X][&Y]
	YIELD OpCode = 0x5

	// CARGS [1] Create a call args object in register $X
	// - CARGS [$X: callargs]
	CARGS OpCode = 0xA
	// CALLB [2] Call a builtin routine defined at pointer $X with the call args in $Y
	// - CALLB [$X: ptr][$Y: callargs]
	CALLB OpCode = 0xB
	// CALLR [2] Call a routine defined at pointer $X with the call args in $Y
	// - CALLR [$X: ptr][$Y: callargs]
	CALLR OpCode = 0xC
	// CALLM [3] Call a method with code Y on the register $X with the call args in $Z
	// - CALLM [$X][Y: 0x00][$Z: callargs]
	CALLM OpCode = 0xD
)

// Pointer Loading & Constant Handling Opcodes
const (
	// CONST [2] Read a constant at pointer $Y into $X
	// - CONST [$X][$Y: ptr]
	CONST OpCode = 0x10

	// LDPTR1 [2] Load a pointer of size 1 byte into $X
	// - LDPTR1 [$X: ptr][0x00]
	LDPTR1 OpCode = 0x11
	// LDPTR2 [3] Load a pointer of size 2 bytes into $X
	// - LDPTR2 [$X: ptr][0x0000]
	LDPTR2 OpCode = 0x12
	// LDPTR3 [4] Load a pointer of size 3 bytes into $X
	// - LDPTR3 [$X: ptr][0x000000]
	LDPTR3 OpCode = 0x13
	// LDPTR4 [5] Load a pointer of size 4 bytes into $X
	// - LDPTR4 [$X: ptr][0x00000000]
	LDPTR4 OpCode = 0x14
	// LDPTR5 [6] Load a pointer of size 5 bytes into $X
	// - LDPTR5 [$X: ptr][0x0000000000]
	LDPTR5 OpCode = 0x15
	// LDPTR6 [7] Load a pointer of size 6 bytes into $X
	// - LDPTR6 [$X: ptr][0x000000000000]
	LDPTR6 OpCode = 0x16
	// LDPTR7 [8] Load a pointer of size 7 bytes into $X
	// - LDPTR7 [$X: ptr][0x00000000000000]
	LDPTR7 OpCode = 0x17
	// LDPTR8 [9] Load a pointer of size 8 bytes into $X
	// - LDPTR1 [$X: ptr][0x0000000000000000]
	LDPTR8 OpCode = 0x18
)

// Register Handling & Initialization Opcodes
const (
	// ISNULL [2] Check if $Y is null/empty and set to $X
	// - ISNULL [$X: bool][$Y]
	ISNULL OpCode = 0x20
	// ZERO [1] Set the value of $X to its zero value. The register retains its type.
	// - ZERO [$X]
	ZERO OpCode = 0x21
	// CLEAR [1] Clear the value of register $X. The register is completely discarded.
	// - CLEAR [$X]
	CLEAR OpCode = 0x22
	// SAME [3] Check if $Y and $Z have the same datatype and set to $X
	// - SAME [$X: bool][$Y][$Z]
	SAME OpCode = 0x23
	// COPY [2] Copy the value of register $Y to $X
	// - COPY [$X][$Y]
	COPY OpCode = 0x24
	// SWAP [2] Swap the value of registers $X and $Y
	// - SWAP [$X][$Y]
	SWAP OpCode = 0x25

	// SERIAL [2] Serialize $Y with POLO and set data to $X
	// - SERIAL [$X: bytes][$Y]
	SERIAL OpCode = 0x26
	// DESERIAL [3] Deserialize $Z with POLO into a type at pointer $Y and set it to register $X
	// - DESERIAL [$X][$Y: ptr][$Z: bytes]
	DESERIAL OpCode = 0x27

	// MAKE [2] Make $X with a type defined at pointer $Y. The register is initialized
	// with the zero value of the type. Can be used for any datatype.
	// - MAKE [$X][$Y: ptr]
	MAKE OpCode = 0x28
	// PMAKE [2] Make register $X with primitive datatype with given type ID. The primitive
	// type register is set to the zero value of the type.
	// - PMAKE [$X][Y: 0x00]
	PMAKE OpCode = 0x29
	// VMAKE [3] Make varray register $X with typedef defined at pointer $Y with size $Z.
	// - VMAKE [$X][$Y: ptr][$Z: U64]
	VMAKE OpCode = 0x2A
	// BMAKE [3] Make register $X with the type defined at pointer $Y by calling method code
	// 0x0 (__build__) with the call args in $Z. Can be used for primitives and classes
	// - BMAKE [$X][$Y: ptr][$Z: callargs]
	BMAKE OpCode = 0x2B
)

// Special (Dunder) Methods Calls Opcodes
const (
	// BUILD [2] Call the method code 0x0 (__build__) of $X with the call args in $Y
	// - BUILD [$X: __build__][$Y: callargs]
	BUILD OpCode = 0x40
	// THROW [1] Throw a runtime exception with the string returned by calling the
	// method code 0x1 (__throw__) on the register $X
	// - THROW [$X: __throw__]
	THROW OpCode = 0x41
	// EMIT [1] Emit the runtime log returned by calling the method code 0x2 (__emit__) on register $X
	// - EMIT [$X: __emit__]
	EMIT OpCode = 0x42
	// JOIN [2] Join $Y and $Z and set to $X. Calls the method code 0x3 (__join__)
	// of $Y and $Z, both of which must be of the same type
	// - JOIN [$X][$Y: __join__][$Z: __join__]
	JOIN OpCode = 0x43

	// LT [3] Check if $Y < $Z by calling the method code 0x4 (__lt__) of $Y and
	// setting it to $X. $Y and $Z must be of the same type.
	// - LT [$X: bool][$Y: __lt__][$Z: __lt__]
	LT OpCode = 0x44
	// GT [3] Check if $Y > $Z by calling the method code 0x5 (__gt__) of $Y and
	// setting it to $X. $Y and $Z must be of the same type.
	// - GT [$X: bool][$Y: __gt__][$Z: __gt__]
	GT OpCode = 0x45
	// EQ [3] Check if $Y == $Z by calling the method code 0x6 (__eq__) of $Y and
	// setting it to $X. $Y and $Z must be of the same type.
	// - EQ [$X: bool][$Y: __eq__][$Z: __eq__]
	EQ OpCode = 0x46

	// BOOL [2] Converts $Y into its boolean form and sets it to $X. Calls the method code 0x7 (__bool__)
	// - BOOL [$X: bool][$Y: __bool__]
	BOOL OpCode = 0x47
	// STR [2] Converts $Y into its string form and sets it to $X. Calls the method code 0x8 (__str__)
	// - STR [$X: string][$Y: __str__]
	STR OpCode = 0x48
	// ADDR [2] Converts $Y into its address form and sets it to $X. Call the method code 0x9 (__addr__)
	// - ADDR [$X: address][$Y: __addr__]
	ADDR OpCode = 0x49

	// LEN [2] Get the len of register $X by calling the method code 0xA (__len__) and setting it to $Y
	// - LEN [$X: U64][$Y: __len__]
	LEN OpCode = 0x4A
)

// Collection & Class Handling Opcodes
const (
	// SIZEOF [2] Get the size of the collection or class register $Y and set it to $X (always u64).
	// If $Y is class, the size represents the number of fields in the class
	// - SIZEOF [$X: u64][$Y: col/class]
	SIZEOF OpCode = 0x50

	// GETFLD [3] Get the field at slot &Z of a class register $Y and set it to $X
	// - GETFLD [$X][$Y: class][&Z: 0x00]
	GETFLD OpCode = 0x51
	// SETFLD [3] Set the field slot &Y of a class register $X to the value of $Z
	// - SETFLD [$X: class][&Y: 0x00][$Z]
	SETFLD OpCode = 0x52

	// GETIDX [3] Get the value of index $Z of a collection register $Y and set it to $X.
	// If  $Y is a (v)array, $Z must be u64. If it is a map, then $Z is the map key type
	// - GETIDX [$X][$Y:col][$Z: idx]
	GETIDX OpCode = 0x53
	// SETIDX [3] Set the value of index $Y of a collection register $X to the value of $Z.
	// If $X is a v/array, $Y must be u64. If it is a map, then $Y is the map key type
	// - SETIDX [$X: col][$Y: idx][$Z]
	SETIDX OpCode = 0x54

	// GROW [2] Grow the size of a varray register $X by $Y which specifies the growth delta
	// - GROW [$X: varray][$Y: u64]
	GROW OpCode = 0x55
	// SLICE [4] Slice the v/array register $Y between the indices $Z and $W and set it $X
	// which is always a varray register. The v/array is sliced as [Z: W)
	// - SLICE [$X: varray][$Y: v/array][$Z: u64][$W: u64]
	SLICE OpCode = 0x56

	// APPEND [2] Append value in $Y to the varray in $X. Grows the varray by 1
	// - APPEND [$X: varray][$Y]
	APPEND OpCode = 0x57
	// POPEND [2] Pop a value from the end of varray $Y to $X. Shrinks the varray by 1
	// - POPEND [$X][$Y: varray]
	POPEND OpCode = 0x58

	// HASKEY [2] Check if the map $Y has a given key $Z and set it to $X
	// - HASKEY [$X: bool][$Y: map][$Z]
	HASKEY OpCode = 0x59

	// MERGE [2] Merge collection $Y into $X. Arrays are not supported.
	// Appends values for varray. Combines keys for mappings overwriting the value from $Y onto $X for shared keys.
	// - MERGE [$X: col][$Y: col]
	MERGE OpCode = 0x5A
)

// Bitwise, Arithmetic & Logical Operator Opcodes
const (
	// AND [3] Perform logical AND ($Y && $Z) and set the result to $X. $Y and $Z must __bool__
	// - AND [$X: bool][$Y: __bool__][$Y: __bool__]
	AND OpCode = 0x60
	// OR [3] Perform logical OR ($Y || $Z) and set the result to $X. $Y and $Z must __bool__
	// - OR [$X: bool][$Y: __bool__][$Y: __bool__]
	OR OpCode = 0x61
	// NOT [2] Perform logical NOT on $Y and set it to $X by calling its __bool__ method and inverting
	// - NOT [$X: bool][$Y: __bool__]
	NOT OpCode = 0x62

	// INCR [1] Increment numeric register $X by 1
	// - INCR [$X]
	INCR OpCode = 0x63
	// DECR [1] Decrement numeric register $X by 1
	// - DECR [$Y]
	DECR OpCode = 0x64

	// ADD [3] Perform arithmetic ADD ($Y + $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - ADD [$X][$Y][$Z]
	ADD OpCode = 0x65
	// SUB [3] Perform arithmetic SUB ($Y - $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - SUB [$X][$Y][$Z]
	SUB OpCode = 0x66
	// MUL [3] Perform arithmetic MUL ($Y * $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - MUL [$X][$Y][$Z]
	MUL OpCode = 0x67
	// DIV [3] Perform arithmetic DIV ($Y // $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - DIV [$X][$Y][$Z]
	DIV OpCode = 0x68
	// MOD [3] Perform arithmetic MOD ($Y % $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - MOD [$X][$Y][$Z]
	MOD OpCode = 0x69

	// BXOR [3] Perform bitwise XOR ($Y ^ $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - BXOR [$X][$Y][$Z]
	BXOR OpCode = 0x6A

	// BAND [3] Perform bitwise AND ($Y & $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - BAND [$X][$Y][$Z]
	BAND OpCode = 0x6B

	// BOR [3] Perform bitwise OR ($Y | $Z) and set the result to $X.
	// $Y and $Z must be numeric and of the same type
	// - BOR [$X][$Y][$Z]
	BOR OpCode = 0x6C

	// BNOT [2] Perform bitwise NOT on $Y and set the result to $X. $Y must be numeric
	// - BNOT BNOT [$X][$Y]
	BNOT OpCode = 0x6D
)

// Environment Access Opcodes
const (
	// IXN [1] Obtain the interaction object and set it to $X
	// - IXN [$X]
	IXN OpCode = 0x0
	// LOGIC [1] Obtain the context object of the logic and set it to $X
	// - LOGIC [$X]
	LOGIC OpCode = 0x71
	// SENDER [1] Obtain the context object of the sender and set it to $X
	// - SENDER [$X]
	SENDER OpCode = 0x72
)

// Persistent Context Handling Opcodes
const (
	// PLOAD [1] Load a stored value into $X from the &Y slot of the persistent logic state
	// - PLOAD [$X: stored][&Y: 0x00]
	PLOAD OpCode = 0x80
	// PSAVE [1] Save a stored valued from $X into the &Y slot of the persistent logic state
	// - PSAVE [$X: stored][&Y: 0x00]
	PSAVE OpCode = 0x81
)

type opcodeMetadata struct {
	str  string
	args int
}

var opcodeMetadataTable = map[OpCode]*opcodeMetadata{
	TERM:   {"TERM", 0},
	DEST:   {"DEST", 0},
	JUMP:   {"JUMP", 1},
	JUMPI:  {"JUMPI", 2},
	OBTAIN: {"OBTAIN", 2},
	YIELD:  {"YIELD", 2},

	// CARGS: nil,
	// CALLB: nil,
	// CALLR: nil,
	// CALLM: nil,

	CONST:  {"CONST", 2},
	LDPTR1: {"LDPTR1", 2},
	LDPTR2: {"LDPTR2", 3},
	LDPTR3: {"LDPTR3", 4},
	LDPTR4: {"LDPTR4", 5},
	LDPTR5: {"LDPTR5", 6},
	LDPTR6: {"LDPTR6", 7},
	LDPTR7: {"LDPTR7", 8},
	LDPTR8: {"LDPTR8", 9},

	ISNULL: {"ISNULL", 2},
	ZERO:   {"ZERO", 1},
	CLEAR:  {"CLEAR", 1},
	SAME:   {"SAME", 3},
	COPY:   {"COPY", 2},
	SWAP:   {"SWAP", 2},

	// SERIAL:   nil,
	// DESERIAL: nil,

	MAKE:  {"MAKE", 2},
	PMAKE: {"PMAKE", 2},
	VMAKE: {"VMAKE", 3},
	// BMAKE: nil,

	// BUILD: nil,
	THROW: {"THROW", 1},
	// EMIT:  nil,
	JOIN: {"JOIN", 3},

	LT: {"LT", 3},
	GT: {"GT", 3},
	EQ: {"EQ", 3},

	BOOL: {"BOOL", 2},
	STR:  {"STR", 2},
	ADDR: {"ADDR", 2},
	LEN:  {"LEN", 2},

	SIZEOF: {"SIZEOF", 2},
	GETFLD: {"GETFLD", 3},
	SETFLD: {"SETFLD", 3},
	GETIDX: {"GETIDX", 3},
	SETIDX: {"SETIDX", 3},

	GROW:   {"GROW", 2},
	SLICE:  {"SLICE", 4},
	APPEND: {"APPEND", 2},
	POPEND: {"POPEND", 2},
	HASKEY: {"HASKEY", 3},
	MERGE:  {"MERGE", 3},

	AND: {"AND", 3},
	OR:  {"OR", 3},
	NOT: {"NOT", 2},

	INCR: {"INCR", 1},
	DECR: {"DECR", 1},

	ADD: {"ADD", 3},
	SUB: {"SUB", 3},
	MUL: {"MUL", 3},
	DIV: {"DIV", 3},
	MOD: {"MOD", 3},

	BXOR: {"BXOR", 3},
	BAND: {"BAND", 3},
	BOR:  {"BOR", 3},
	BNOT: {"BNOT", 2},

	// IXN: {"IXN", 1}
	LOGIC:  {"LOGIC", 1},
	SENDER: {"SENDER", 1},

	PLOAD: {"PLOAD", 2},
	PSAVE: {"PSAVE", 2},
}

// String returns the string representation of OpCode.
// Returns an empty string for an undefined opcode.
// It implements the Stringer interface for OpCode.
func (op OpCode) String() string {
	opcode := opcodeMetadataTable[op]
	if opcode == nil {
		return fmt.Sprintf("undefined opcode [%#x]", int(op))
	}

	return opcode.str
}

// Operands returns the number of operands to expect for the opcode.
// Returns false for an undefined opcode.
func (op OpCode) Operands() (int, bool) {
	opcode := opcodeMetadataTable[op]
	if opcode == nil {
		return 0, false
	}

	return opcode.args, true
}

var stringToOpCode = map[string]OpCode{
	"TERM":  TERM,
	"DEST":  DEST,
	"JUMP":  JUMP,
	"JUMPI": JUMPI,

	"OBTAIN": OBTAIN,
	"YIELD":  YIELD,

	"CARGS": CARGS,
	"CALLB": CALLB,
	"CALLR": CALLR,
	"CALLM": CALLM,

	"CONST": CONST,

	"LDPTR1": LDPTR1,
	"LDPTR2": LDPTR2,
	"LDPTR3": LDPTR3,
	"LDPTR4": LDPTR4,
	"LDPTR5": LDPTR5,
	"LDPTR6": LDPTR6,
	"LDPTR7": LDPTR7,
	"LDPTR8": LDPTR8,

	"ISNULL":   ISNULL,
	"ZERO":     ZERO,
	"CLEAR":    CLEAR,
	"SAME":     SAME,
	"COPY":     COPY,
	"SWAP":     SWAP,
	"SERIAL":   SERIAL,
	"DESERIAL": DESERIAL,

	"MAKE":  MAKE,
	"PMAKE": PMAKE,
	"VMAKE": VMAKE,
	"BMAKE": BMAKE,

	"BUILD": BUILD,
	"THROW": THROW,
	"EMIT":  EMIT,
	"JOIN":  JOIN,

	"LT": LT,
	"GT": GT,
	"EQ": EQ,

	"BOOL": BOOL,
	"STR":  STR,
	"ADDR": ADDR,
	"LEN":  LEN,

	"SIZEOF": SIZEOF,
	"GETFLD": GETFLD,
	"SETFLD": SETFLD,
	"GETIDX": GETIDX,
	"SETIDX": SETIDX,

	"GROW":   GROW,
	"SLICE":  SLICE,
	"APPEND": APPEND,
	"POPEND": POPEND,
	"HASKEY": HASKEY,
	"MERGE":  MERGE,

	"AND": AND,
	"OR":  OR,
	"NOT": NOT,

	"INCR": INCR,
	"DECR": DECR,

	"ADD": ADD,
	"SUB": SUB,
	"MUL": MUL,
	"DIV": DIV,
	"MOD": MOD,

	"BXOR": BXOR,
	"BAND": BAND,
	"BOR":  BOR,
	"BNOT": BNOT,

	"IXN":    IXN,
	"LOGIC":  LOGIC,
	"SENDER": SENDER,

	"PLOAD": PLOAD,
	"PSAVE": PSAVE,
}

// StringToOpCode finds the opcode whose name is stored in str.
func StringToOpCode(str string) OpCode {
	return stringToOpCode[str]
}
