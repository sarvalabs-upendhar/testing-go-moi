package pisa

import (
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
				Data:  "register $0 is not a pointer",
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
				Data:  "ptr does not implement __bool__",
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
				Data:  "register $0 is not a pointer",
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
				Data:  "register $0 is not a pointer",
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
				Data:  "not uint64", // todo: this needs to be more clear
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

	// todo: implement this
	t.Run("LDPTR", func(t *testing.T) {})

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

	// todo: implement this
	t.Run("ZERO", func(t *testing.T) {})

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
				Data:  "register $0 is not a pointer",
				Trace: []string{},
			}}, continuity)
		})

		// todo: need capability to disable loading elements from the logic driver
		// t.Run("missing_typedef", func(t *testing.T) {
		//
		// })

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
			require.Equal(t, must(NewListValue(NewArrayType(32, TypeU64), nil)), scope.memory[1])
		})
	})

	t.Run("PMAKE", func(t *testing.T) {
		t.Run("invalid_typeID", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{},
			}

			continuity := opPMAKE(scope, []byte{0, 20})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Data:  "invalid type ID: 20",
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
				Data:  "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})
	})

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
				Data:  "ptr does not implement __str__",
				Trace: []string{},
			}}, continuity)
		})
	})
}
