package pisa

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestInstructionSet(t *testing.T) {
	runtime := NewRuntime()

	t.Run("TERM", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
		}

		continuity := opTERM(scope, nil)
		require.Equal(t, continueTerm{}, continuity)
	})

	t.Run("DEST", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
		}

		continuity := opDEST(scope, nil)
		require.Equal(t, continueOk{1}, continuity)
	})

	t.Run("JUMP", func(t *testing.T) {
		t.Run("valid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
				},
			}

			continuity := opJUMP(scope, []byte{0})
			require.Equal(t, continueJump{10, uint64(5)}, continuity)
		})

		t.Run("invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(5),
				},
			}

			continuity := opJUMP(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})
	})

	t.Run("JUMPI", func(t *testing.T) {
		t.Run("invalid_condition", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
					1: PtrValue(10),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("false_condition", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
					1: BoolValue(false),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueOk{10}, continuity)
		})

		t.Run("true_condition_invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(5),
					1: BoolValue(true),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("true_condition_valid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
					1: BoolValue(true),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueJump{20, uint64(5)}, continuity)
		})
	})

	t.Run("OBTAIN", func(t *testing.T) {
		t.Run("available_accept_data", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
				inputs: map[byte]RegisterValue{
					0: I64Value(-10),
				},
			}

			continuity := opOBTAIN(scope, []byte{0, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, I64Value(-10), scope.memory[0])
		})

		t.Run("missing_accept_data", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
				inputs: map[byte]RegisterValue{},
			}

			continuity := opOBTAIN(scope, []byte{0, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, NullValue{}, scope.memory[0])
		})
	})

	t.Run("YIELD", func(t *testing.T) {
		t.Run("available_data", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: AddressValue{0x10, 0x20},
				},
				outputs: map[byte]RegisterValue{},
			}

			continuity := opYIELD(scope, []byte{0, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, AddressValue{0x10, 0x20}, scope.outputs[0])
		})

		t.Run("empty_data", func(t *testing.T) {
			scope := &callscope{
				engine:  &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory:  map[byte]RegisterValue{},
				outputs: map[byte]RegisterValue{},
			}

			continuity := opYIELD(scope, []byte{0, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, NullValue{}, scope.outputs[0])
		})
	})

	t.Run("CONST", func(t *testing.T) {
		t.Run("invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(0),
				},
			}

			continuity := opCONST(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})

		// todo: need capability to disable loading elements from the logic driver
		// t.Run("missing_element", func(t *testing.T) {
		//	scope := &callscope{
		//		engine: &Engine{
		//			callstack: make(callstack, 0),
		//			runtime:   &runtime,
		//			elements:  map[engineio.ElementPtr]any{},
		//		},
		//		memory: map[byte]RegisterValue{
		//			0: PtrValue(0),
		//		},
		//	}
		//
		//	continuity := opCONST(scope, []byte{1, 0})
		//	require.Equal(t, continueException{0, &Exception{
		//		Class: "builtin.ReferenceError",
		//		Data:  "constant 0x0 not found: %v",
		//		Trace: []string{},
		//	}}, continuity)
		// })

		t.Run("malformed_value", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: &Constant{Type: PrimitiveU64, Data: []byte{0x6, 0xa, 0xb}},
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opCONST(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "malformed constant: data does not decode to a uint64",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: &Constant{Type: PrimitiveU64, Data: []byte{0x3, 0xa, 0xb}},
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opCONST(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(0x0a0b), scope.memory[1])
		})
	})

	t.Run("LDPTR", func(t *testing.T) {
		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLDPTR(scope, []byte{0, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.OverflowError",
				Error: "pointer value exceeds 8 bytes",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("8bit", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLDPTR(scope, []byte{0, 0x10})
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, PtrValue(0x10), scope.memory[0])
		})

		t.Run("32bit", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLDPTR(scope, []byte{0, 0x10, 0x10, 0x10, 0x10})
			require.Equal(t, continueOk{16}, continuity)
			require.Equal(t, PtrValue(0x10101010), scope.memory[0])
		})

		t.Run("64bit", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLDPTR(scope, []byte{0, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10})
			require.Equal(t, continueOk{24}, continuity)
			require.Equal(t, PtrValue(0x1010101010101010), scope.memory[0])
		})
	})

	t.Run("ISNULL", func(t *testing.T) {
		t.Run("null_value", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opISNULL(scope, []byte{1, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[1])
		})

		t.Run("reg_value", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(1000),
				},
			}

			continuity := opISNULL(scope, []byte{1, 0})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[1])
		})
	})

	t.Run("ZERO", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
			memory: map[byte]RegisterValue{
				0: U64Value(1000),
			},
		}

		continuity := opZERO(scope, []byte{0})
		require.Equal(t, continueOk{5}, continuity)
		require.Equal(t, U64Value(0), scope.memory[0])
	})

	t.Run("CLEAR", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
			memory: map[byte]RegisterValue{
				0: U64Value(1000),
			},
		}

		continuity := opCLEAR(scope, []byte{0})
		require.Equal(t, continueOk{5}, continuity)
		require.Nil(t, scope.memory[0])
	})

	t.Run("SAME", func(t *testing.T) {
		t.Run("same_type", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(1000),
					1: U64Value(1000),
				},
			}

			continuity := opSAME(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[2])
		})

		t.Run("diff_type", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(1000),
					1: I64Value(1000),
				},
			}

			continuity := opSAME(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{5}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	t.Run("COPY", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
			memory: map[byte]RegisterValue{
				0: U64Value(1000),
			},
		}

		continuity := opCOPY(scope, []byte{1, 0})
		require.Equal(t, continueOk{5}, continuity)
		require.Equal(t, U64Value(1000), scope.memory[0])
		require.Equal(t, U64Value(1000), scope.memory[1])
	})

	t.Run("SWAP", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
			memory: map[byte]RegisterValue{
				0: U64Value(1000),
				1: StringValue("foo"),
			},
		}

		continuity := opSWAP(scope, []byte{1, 0})
		require.Equal(t, continueOk{5}, continuity)
		require.Equal(t, StringValue("foo"), scope.memory[0])
		require.Equal(t, U64Value(1000), scope.memory[1])
	})

	t.Run("MAKE", func(t *testing.T) {
		t.Run("invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(0),
				},
			}

			continuity := opMAKE(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})

		// todo: need capability to disable loading elements from the logic driver
		// t.Run("missing_typedef", func(t *testing.T) {})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: NewArrayType(32, TypeU64),
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opMAKE(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, must(newListValue(NewArrayType(32, TypeU64), nil)), scope.memory[1])
		})
	})

	t.Run("PMAKE", func(t *testing.T) {
		t.Run("invalid_typeID", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opPMAKE(scope, []byte{0, 0x20})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "invalid primitive type: 0x20",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opPMAKE(scope, []byte{0, 3})
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, StringValue(""), scope.memory[0])
		})
	})

	t.Run("VMAKE", func(t *testing.T) {
		t.Run("invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(0),
				},
			}

			continuity := opVMAKE(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})

		// todo: need capability to disable loading elements from the logic driver
		// t.Run("missing_typedef", func(t *testing.T) {})

		t.Run("not_a_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: NewArrayType(32, TypeU64),
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: U64Value(4),
				},
			}

			continuity := opVMAKE(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "typedef 0x0 is not a varray",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_length", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: NewVarrayType(TypeU64),
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: I64Value(4),
				},
			}

			continuity := opVMAKE(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a uint64: $1",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: NewVarrayType(TypeU64),
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: U64Value(4),
				},
			}

			continuity := opVMAKE(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{10 + (5 * 4)}, continuity)
			require.Equal(t, &ListValue{
				values:   make([]RegisterValue, 4),
				datatype: NewVarrayType(TypeU64),
			}, scope.memory[2])
		})
	})

	t.Run("THROW", func(t *testing.T) {
		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(0),
				},
			}

			continuity := opTHROW(scope, []byte{0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "int64 does not implement __throw__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("str_exception", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("FAIL!"),
				},
			}

			continuity := opTHROW(scope, []byte{0})
			require.Equal(t, continueException{30, &Exception{
				Class: "string",
				Error: "FAIL!",
				Trace: []string{},
			}}, continuity)
		})
	})

	t.Run("JOIN", func(t *testing.T) {
		t.Run("non_symmetric_values", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(99),
					1: I64Value(66),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: randomAddressValue(t),
					1: randomAddressValue(t),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "address does not implement __join__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("uint64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, U64Value(165), scope.memory[2])
		})

		t.Run("int64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(-66),
					1: I64Value(99),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, I64Value(33), scope.memory[2])
		})

		t.Run("bool", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(false),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})

		t.Run("string", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, StringValue("foobar"), scope.memory[2])
		})

		t.Run("bytes", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: BytesValue{0x01, 0x78},
					1: BytesValue{0x75, 0x54},
				},
			}

			continuity := opJOIN(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, BytesValue{0x01, 0x78, 0x75, 0x54}, scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("LT", func(t *testing.T) {
		t.Run("non_symmetric_values", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(99),
					1: I64Value(66),
				},
			}

			continuity := opLT(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: randomAddressValue(t),
					1: randomAddressValue(t),
				},
			}

			continuity := opLT(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "address does not implement __lt__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			continuity := opLT(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("GT", func(t *testing.T) {
		t.Run("non_symmetric_values", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(99),
					1: I64Value(66),
				},
			}

			continuity := opGT(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: randomAddressValue(t),
					1: randomAddressValue(t),
				},
			}

			continuity := opGT(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "address does not implement __gt__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			continuity := opGT(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("EQ", func(t *testing.T) {
		t.Run("non_symmetric_values", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(99),
					1: I64Value(66),
				},
			}

			continuity := opEQ(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: randomAddressValue(t),
					1: randomAddressValue(t),
				},
			}

			continuity := opEQ(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "address does not implement __eq__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			continuity := opEQ(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	t.Run("BOOL", func(t *testing.T) {
		t.Run("implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			continuity := opBOOL(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[1])
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opBOOL(scope, []byte{1, 0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})
	})

	//nolint:dupl
	t.Run("STR", func(t *testing.T) {
		t.Run("implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(757),
				},
			}

			continuity := opSTR(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, StringValue("757"), scope.memory[1])
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(757),
				},
			}

			continuity := opSTR(scope, []byte{1, 0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __str__",
				Trace: []string{},
			}}, continuity)
		})
	})

	t.Run("ADDR", func(t *testing.T) {
		t.Run("implemented", func(t *testing.T) {
			addr := randomAddressValue(t)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: addr,
				},
			}

			continuity := opADDR(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, addr, scope.memory[1])
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(757),
				},
			}

			continuity := opADDR(scope, []byte{1, 0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __addr__",
				Trace: []string{},
			}}, continuity)
		})
	})

	//nolint:dupl
	t.Run("LEN", func(t *testing.T) {
		t.Run("implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("hello!"),
				},
			}

			continuity := opLEN(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(6), scope.memory[1])
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(757),
				},
			}

			continuity := opLEN(scope, []byte{1, 0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "uint64 does not implement __len__",
				Trace: []string{},
			}}, continuity)
		})
	})

	t.Run("SIZEOF", func(t *testing.T) {
		t.Run("unsupported", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(757),
				},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a sizeable: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("empty_reg", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "$0 is null",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("array", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListValue(NewArrayType(10, TypeI64), nil)),
				},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(10), scope.memory[1])
		})

		t.Run("varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newSizedList(NewVarrayType(TypeBool), 8)),
				},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(8), scope.memory[1])
		})

		t.Run("mapping", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newMapValue(NewMappingType(PrimitiveString, TypeString), nil)),
				},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(0), scope.memory[1])
		})

		t.Run("class", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newClassValue(NewClassType("Person", makefields([]*TypeField{
						{"Name", TypeString},
						{"Age", TypeU64},
					})), nil)),
				},
			}

			continuity := opSIZEOF(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(2), scope.memory[1])
		})
	})

	t.Run("GROW", func(t *testing.T) {
		t.Run("not_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("hello"),
				},
			}

			continuity := opGROW(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a varray: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_length", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListValue(NewVarrayType(TypeString), nil)),
					1: I64Value(4),
				},
			}

			continuity := opGROW(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a uint64: $1",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newSizedList(NewVarrayType(TypeU64), 4)),
					1: U64Value(6),
				},
			}

			continuity := opGROW(scope, []byte{0, 1})
			require.Equal(t, continueOk{35}, continuity)
			require.Equal(t, must(newSizedList(NewVarrayType(TypeU64), 10)), scope.memory[0])
		})
	})

	t.Run("APPEND", func(t *testing.T) {
		t.Run("not_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("hello"),
				},
			}

			continuity := opAPPEND(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a varray: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_value", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListValue(NewVarrayType(TypeString), nil)),
					1: U64Value(0),
				},
			}

			continuity := opAPPEND(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "invalid varray element: not a string",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListValue(NewVarrayType(TypeString), nil)),
					1: StringValue("hello"),
				},
			}

			continuity := opAPPEND(scope, []byte{0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, must(newListFromValues(NewVarrayType(TypeString), StringValue("hello"))), scope.memory[0])
		})
	})

	t.Run("POPEND", func(t *testing.T) {
		t.Run("not_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListFromValues(NewArrayType(2, TypeString), StringValue("foo"), StringValue("bar"))),
				},
			}

			continuity := opPOPEND(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a varray: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("empty_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListValue(NewVarrayType(TypeString), nil)),
				},
			}

			continuity := opPOPEND(scope, []byte{1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "varray is empty",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListFromValues(NewVarrayType(TypeString), StringValue("foo"), StringValue("bar"))),
				},
			}

			continuity := opPOPEND(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)

			require.Equal(t, StringValue("bar"), scope.memory[1])
			require.Equal(t, must(newListFromValues(NewVarrayType(TypeString), StringValue("foo"))), scope.memory[0])
		})
	})

	t.Run("HASKEY", func(t *testing.T) {
		t.Run("not_mapping", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newListFromValues(NewArrayType(2, TypeString), StringValue("foo"), StringValue("bar"))),
					1: StringValue("foo"),
				},
			}

			continuity := opHASKEY(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a mapping: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_key", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newMapValue(NewMappingType(PrimitiveString, TypeString), nil)),
					1: BoolValue(true),
				},
			}

			continuity := opHASKEY(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "invalid map key: not a string",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newMapValue(NewMappingType(PrimitiveString, TypeString), nil)),
					1: StringValue("hello"),
				},
			}

			continuity := opHASKEY(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{15}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("AND", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("unimplemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: PtrValue(12),
				},
			}

			continuity := opAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{5, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(true),
				},
			}

			continuity := opAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("OR", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("unimplemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: PtrValue(12),
				},
			}

			continuity := opOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{5, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(false),
				},
			}

			continuity := opOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, BoolValue(true), scope.memory[2])
		})
	})

	t.Run("NOT", func(t *testing.T) {
		t.Run("unimplemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opNOT(scope, []byte{1, 0})
			require.Equal(t, continueException{5, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
				},
			}

			continuity := opNOT(scope, []byte{1, 0})
			require.Equal(t, continueOk{15}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[1])
		})
	})

	//nolint:dupl
	t.Run("INCR", func(t *testing.T) {
		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			continuity := opINCR(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot increment with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(math.MaxUint64),
				},
			}

			continuity := opINCR(scope, []byte{0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.OverflowError",
				Error: "increment overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(500),
				},
			}

			continuity := opINCR(scope, []byte{0})
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, U64Value(501), scope.memory[0])
		})
	})

	//nolint:dupl
	t.Run("DECR", func(t *testing.T) {
		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			continuity := opDECR(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot decrement with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(math.MinInt64),
				},
			}

			continuity := opDECR(scope, []byte{0})
			require.Equal(t, continueException{10, &Exception{
				Class: "builtin.OverflowError",
				Error: "decrement overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(500),
				},
			}

			continuity := opDECR(scope, []byte{0})
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, U64Value(499), scope.memory[0])
		})
	})

	//nolint:dupl
	t.Run("ADD", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot add with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(math.MaxUint64),
					1: U64Value(10),
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{20, &Exception{
				Class: "builtin.OverflowError",
				Error: "addition overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(100),
					1: U64Value(10),
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(110), scope.memory[2])
		})
	})

	t.Run("SUB", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot subtract with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(10),
					1: U64Value(11),
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{20, &Exception{
				Class: "builtin.OverflowError",
				Error: "subtraction overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(100),
					1: U64Value(10),
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(90), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("MUL", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot multiply with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(math.MaxUint64),
					1: U64Value(100),
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.OverflowError",
				Error: "multiplication overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(100),
					1: U64Value(10),
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, U64Value(1000), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("DIV", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot divide with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(math.MinInt64),
					1: I64Value(-1),
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.OverflowError",
				Error: "division overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("div_by_zero", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(math.MaxUint64),
					1: U64Value(0),
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.DivideByZeroError",
				Error: "division by zero",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(100),
					1: U64Value(10),
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, U64Value(10), scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("MOD", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$0, $1]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot modulo divide with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("overflow", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(math.MinInt64),
					1: I64Value(-1),
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.OverflowError",
				Error: "modulo division overflow",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("div_by_zero", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(math.MaxUint64),
					1: U64Value(0),
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.DivideByZeroError",
				Error: "modulo division by zero",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(56),
					1: U64Value(11),
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, U64Value(1), scope.memory[2])
		})
	})
}
