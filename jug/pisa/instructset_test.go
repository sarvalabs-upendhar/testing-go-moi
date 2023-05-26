package pisa

import (
	"math"
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
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
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("false_condition", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
					1: BoolValue(false),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueOk{0}, continuity)
		})

		t.Run("true_condition_invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(5),
					1: BoolValue(true),
				},
			}

			continuity := opJUMPI(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("true_condition_valid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(5),
					1: BoolValue(true),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJUMPI(scope, []byte{0, 1})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueJump{10, uint64(5)}, continuity)
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

	t.Run("CARGS", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
			memory: map[byte]RegisterValue{},
		}

		continuity := opCARGS(scope, []byte{0})
		require.Equal(t, continueOk{5}, continuity)
		require.Equal(t, make(CargsValue), scope.memory[0])
	})

	t.Run("CALLR", func(t *testing.T) {
		sampleRoutine := &Routine{
			Ptr:  0,
			Name: "Doubler",
			Kind: engineio.InvokableCallsite,
			CallFields: CallFields{
				Inputs:  makefields([]*TypeField{{"number", PrimitiveU64}}),
				Outputs: makefields([]*TypeField{{"doubled", PrimitiveU64}}),
			},
			Instructs: Instructions{
				{OBTAIN, []byte{0, 0}},
				{ADD, []byte{1, 0, 0}},
				{YIELD, []byte{1, 0}},
			},
		}

		errorRoutine := &Routine{
			Ptr:  0,
			Name: "ThrowError",
			Kind: engineio.InvokableCallsite,
			CallFields: CallFields{
				Inputs:  makefields([]*TypeField{{"data", PrimitiveString}}),
				Outputs: makefields([]*TypeField{{"doubled", PrimitiveU64}}),
			},
			Instructs: Instructions{
				{OBTAIN, []byte{0, 0}},
				{ADD, []byte{1, 0, 0}},
				{YIELD, []byte{1, 0}},
			},
		}

		t.Run("invalid_pointer", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					1: I64Value(0),
				},
			}

			continuity := opCALLR(scope, []byte{2, 1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ReferenceError",
				Error: "not a pointer: $1",
				Trace: []string{},
			}}, continuity)
		})

		// todo: need capability to disable loading elements from the logic driver
		// t.Run("missing_element", func(t *testing.T) {}

		t.Run("invalid_args", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					elements: map[engineio.ElementPtr]any{
						0: sampleRoutine,
					},
				},
				memory: map[byte]RegisterValue{
					0: I64Value(10),
					1: PtrValue(0),
				},
			}

			continuity := opCALLR(scope, []byte{2, 1, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a cargs: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("exception_thrown", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
					elements: map[engineio.ElementPtr]any{
						0: errorRoutine,
					},
				},
				memory: map[byte]RegisterValue{
					0: CargsValue{0: StringValue("foo")},
					1: PtrValue(0),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opCALLR(scope, []byte{2, 1, 0})

			require.Equal(t, level-5, scope.engine.fueltank.Level())
			require.Equal(t, continueException{50, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot add with string registers",
				Trace: []string{
					"root.ThrowError() [0x0] ... [0x1: ADD 0x1 0x0 0x0]",
				},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
					elements: map[engineio.ElementPtr]any{
						0: sampleRoutine,
					},
				},
				memory: map[byte]RegisterValue{
					0: CargsValue{0: U64Value(50)},
					1: PtrValue(0),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opCALLR(scope, []byte{2, 1, 0})

			require.Equal(t, level-30, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{50}, continuity)
			require.Equal(t, CargsValue{0: U64Value(100)}, scope.memory[2])
		})
	})

	t.Run("CALLM", func(t *testing.T) {
		t.Run("invalid_args", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			continuity := opCALLM(scope, []byte{1, 0x3, 0}) // Call __join__
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a cargs: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("missing_method_register", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: CargsValue{},
				},
			}

			continuity := opCALLM(scope, []byte{1, 0x3, 0}) // Call __join__
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.CallError",
				Error: "missing method register",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: CargsValue{
						0: StringValue("foo"),
						1: StringValue("bar"),
					},
				},
			}

			continuity := opCALLM(scope, []byte{1, 0x5, 0}) // Call __gt__
			require.Equal(t, continueException{30, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "string does not implement __gt__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: CargsValue{
						0: StringValue("foo"),
						1: StringValue("bar"),
					},
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opCALLM(scope, []byte{1, 0x3, 0}) // Call __join__

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, CargsValue{0: StringValue("foobar")}, scope.memory[1])
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
				Error: "malformed constant: data does not decode to a u64",
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
						0: ArrayDatatype{PrimitiveU64, 32},
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
				},
			}

			continuity := opMAKE(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, must(newArrayValue(ArrayDatatype{PrimitiveU64, 32}, nil)), scope.memory[1])
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
						0: ArrayDatatype{PrimitiveU64, 32},
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
						0: VarrayDatatype{PrimitiveU64},
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
				Error: "not a u64: $1",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime:   &runtime,
					callstack: make(callstack, 0),
					elements: map[engineio.ElementPtr]any{
						0: VarrayDatatype{PrimitiveU64},
					},
				},
				memory: map[byte]RegisterValue{
					0: PtrValue(0),
					1: U64Value(4),
				},
			}

			continuity := opVMAKE(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{10 + (5 * 4)}, continuity)
			require.Equal(t, &VarrayValue{
				values:   make([]RegisterValue, 4),
				datatype: VarrayDatatype{PrimitiveU64},
			}, scope.memory[2])
		})
	})

	t.Run("THROW", func(t *testing.T) {
		t.Run("not_implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: I64Value(0),
				},
			}

			continuity := opTHROW(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "i64 does not implement __throw__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("str_exception", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: StringValue("FAIL!"),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opTHROW(scope, []byte{0})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueException{10, &Exception{
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJOIN(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, U64Value(165), scope.memory[2])
		})

		t.Run("int64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: I64Value(-66),
					1: I64Value(99),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJOIN(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, I64Value(33), scope.memory[2])
		})

		t.Run("bool", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(false),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJOIN(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})

		t.Run("string", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("bar"),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJOIN(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, StringValue("foobar"), scope.memory[2])
		})

		t.Run("bytes", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: BytesValue{0x01, 0x78},
					1: BytesValue{0x75, 0x54},
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opJOIN(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opLT(scope, []byte{2, 0, 1})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opGT(scope, []byte{2, 0, 1})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

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
					0: PtrValue(0),
					1: PtrValue(10),
				},
			}

			continuity := opEQ(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __eq__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(66),
					1: U64Value(99),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opEQ(scope, []byte{2, 0, 1})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	t.Run("BOOL", func(t *testing.T) {
		t.Run("implemented", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opBOOL(scope, []byte{1, 0})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: U64Value(757),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opSTR(scope, []byte{1, 0})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: addr,
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opADDR(scope, []byte{1, 0})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
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
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: StringValue("hello!"),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opLEN(scope, []byte{1, 0})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "u64 does not implement __len__",
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
					0: must(newArrayValue(ArrayDatatype{PrimitiveU64, 10}, nil)),
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
					0: newVarrayWithSize(VarrayDatatype{PrimitiveBool}, 8),
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
					0: must(newMapValue(MapDatatype{PrimitiveString, PrimitiveString}, nil)),
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
					0: must(newClassValue(ClassDatatype{
						name: "Person",
						fields: makefields([]*TypeField{
							{"Name", PrimitiveString},
							{"Age", PrimitiveU64},
						}),
					}, nil)),
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
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					1: I64Value(4),
				},
			}

			continuity := opGROW(scope, []byte{0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a u64: $1",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: newVarrayWithSize(VarrayDatatype{PrimitiveU64}, 4),
					1: U64Value(6),
				},
			}

			continuity := opGROW(scope, []byte{0, 1})
			require.Equal(t, continueOk{35}, continuity)
			require.Equal(t, newVarrayWithSize(VarrayDatatype{PrimitiveU64}, 10), scope.memory[0])
		})
	})

	t.Run("SLICE", func(t *testing.T) {
		t.Run("invalid_type", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(1),
					2: U64Value(2),
				},
			}

			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a primitive: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_index_type", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: StringValue("foo"),
					2: U64Value(2),
				},
			}

			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a primitive: $0",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_index_type2", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newArrayFromValues(
						ArrayDatatype{PrimitiveString, 4},
						StringValue("foo"), StringValue("bar"), StringValue("car"), StringValue("bat"),
					)),
					1: U64Value(2),
					2: StringValue("foo"),
				},
			}

			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "invalid array index for slice stop: not a u64",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("index_out_of_range", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newArrayFromValues(
						ArrayDatatype{PrimitiveString, 4},
						StringValue("foo"), StringValue("bar"), StringValue("car"), StringValue("bat"),
					)),
					1: U64Value(2),
					2: U64Value(6),
				},
			}

			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "invalid array index for slice: out of range",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newVarrayFromValues(
						VarrayDatatype{PrimitiveString},
						StringValue("foo"), StringValue("bar"), StringValue("car"), StringValue("bat"),
					)),
					1: U64Value(0),
					2: U64Value(2),
				},
			}
			res := must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar")))
			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, res, scope.memory[3])
		})

		t.Run("success_array", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newArrayFromValues(
						ArrayDatatype{PrimitiveString, 4},
						StringValue("foo"), StringValue("bar"), StringValue("car"), StringValue("bat"),
					)),
					1: U64Value(0),
					2: U64Value(2),
				},
			}
			res := must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar")))
			continuity := opSLICE(scope, []byte{3, 0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, res, scope.memory[3])
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
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
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
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					1: StringValue("hello"),
				},
			}

			continuity := opAPPEND(scope, []byte{0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("hello"))), scope.memory[0])
		})
	})

	t.Run("POPEND", func(t *testing.T) {
		t.Run("not_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newArrayFromValues(ArrayDatatype{PrimitiveString, 2}, StringValue("foo"), StringValue("bar"))),
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
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
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
					0: must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar"))),
				},
			}

			continuity := opPOPEND(scope, []byte{1, 0})
			require.Equal(t, continueOk{20}, continuity)

			require.Equal(t, StringValue("bar"), scope.memory[1])
			require.Equal(t, must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"))), scope.memory[0])
		})
	})

	t.Run("HASKEY", func(t *testing.T) {
		t.Run("not_mapping", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newArrayFromValues(ArrayDatatype{PrimitiveString, 2}, StringValue("foo"), StringValue("bar"))),
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
					0: must(newMapValue(MapDatatype{PrimitiveString, PrimitiveString}, nil)),
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
					0: must(newMapValue(MapDatatype{PrimitiveString, PrimitiveString}, nil)),
					1: StringValue("hello"),
				},
			}

			continuity := opHASKEY(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{15}, continuity)
			require.Equal(t, BoolValue(false), scope.memory[2])
		})
	})

	t.Run("MERGE", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("hoo"),
					1: StringValue("foo"),
					2: U64Value(56),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "not symmetric: [$1, $2]",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("invalid_type", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("hoo"),
					1: StringValue("foo"),
					2: StringValue("too"),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.TypeError",
				Error: "not a primitive: $1",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("empty_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					1: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					2: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, nil)), scope.memory[0])
		})

		t.Run("success_varray", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					1: must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar"))),
					2: must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("koo"), StringValue("car"))),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, must(newVarrayFromValues(VarrayDatatype{PrimitiveString},
				StringValue("foo"), StringValue("bar"), StringValue("koo"), StringValue("car")),
			), scope.memory[0])
		})

		t.Run("success_maps", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					1: must(newMapFromValues(
						MapDatatype{PrimitiveString, PrimitiveString},
						map[RegisterValue]RegisterValue{
							StringValue("I"): StringValue("am"),
						},
					)),
					2: must(newMapFromValues(
						MapDatatype{PrimitiveString, PrimitiveString},
						map[RegisterValue]RegisterValue{
							StringValue("hello"): StringValue("yes"),
							StringValue("yo"):    StringValue("you"),
						},
					)),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, must(newMapFromValues(
				MapDatatype{PrimitiveString, PrimitiveString},
				map[RegisterValue]RegisterValue{
					StringValue("I"):     StringValue("am"),
					StringValue("hello"): StringValue("yes"),
					StringValue("yo"):    StringValue("you"),
				}),
			), scope.memory[0])
		})

		t.Run("success_maps_overwrite", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
					1: must(newMapFromValues(
						MapDatatype{PrimitiveString, PrimitiveString},
						map[RegisterValue]RegisterValue{
							StringValue("I"): StringValue("am"),
						},
					)),
					2: must(newMapFromValues(
						MapDatatype{PrimitiveString, PrimitiveString},
						map[RegisterValue]RegisterValue{
							StringValue("I"):  StringValue("am"),
							StringValue("yo"): StringValue("you"),
						},
					)),
				},
			}

			continuity := opMERGE(scope, []byte{0, 1, 2})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, must(newMapFromValues(
				MapDatatype{PrimitiveString, PrimitiveString},
				map[RegisterValue]RegisterValue{
					StringValue("I"):  StringValue("am"),
					StringValue("yo"): StringValue("you"),
				}),
			), scope.memory[0])
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
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(true),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opAND(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
					1: BoolValue(false),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opOR(scope, []byte{2, 0, 1})

			require.Equal(t, level-20, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.NotImplementedError",
				Error: "ptr does not implement __bool__",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{
					callstack: make(callstack, 0), runtime: &runtime,
					fueltank: engineio.NewFuelTank(1000),
				},
				memory: map[byte]RegisterValue{
					0: BoolValue(true),
				},
			}

			level := scope.engine.fueltank.Level()
			continuity := opNOT(scope, []byte{1, 0})

			require.Equal(t, level-10, scope.engine.fueltank.Level())
			require.Equal(t, continueOk{10}, continuity)
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

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
					1: &U256Value{uint256.NewInt(11)},
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(67)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			res := big.NewInt(-45)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opADD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
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

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
					1: &U256Value{uint256.NewInt(11)},
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(45)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			res := big.NewInt(-67)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opSUB(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
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

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(20)},
					1: &U256Value{uint256.NewInt(2)},
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(40)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-5)
			res := big.NewInt(-55)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opMUL(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
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

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(20)},
					1: &U256Value{uint256.NewInt(2)},
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(10)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-10)
			res := big.NewInt(-5)
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(2)},
				},
			}

			continuity := opDIV(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
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

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(20)},
					1: &U256Value{uint256.NewInt(2)},
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(0)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-10)
			res := big.NewInt(-3)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(7)},
				},
			}

			continuity := opMOD(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("BXOR", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opBXOR(scope, []byte{2, 0, 1})
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

			continuity := opBXOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot bxor with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success_u64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(56),
					1: U64Value(11),
				},
			}

			continuity := opBXOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(51), scope.memory[2])
		})

		t.Run("success_i64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(-56),
					1: I64Value(11),
				},
			}

			continuity := opBXOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, I64Value(-61), scope.memory[2])
		})

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
					1: &U256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBXOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(51)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			res := big.NewInt(-61)
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBXOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
		})
	})

	t.Run("BAND", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opBAND(scope, []byte{2, 0, 1})
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

			continuity := opBAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot band with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success_u64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(56),
					1: U64Value(11),
				},
			}

			continuity := opBAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(8), scope.memory[2])
		})

		t.Run("success_i64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(-56),
					1: I64Value(11),
				},
			}

			continuity := opBAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, I64Value(8), scope.memory[2])
		})

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
					1: &U256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(8)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBAND(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.NewInt(8)}, scope.memory[2])
		})
	})

	//nolint:dupl
	t.Run("BOR", func(t *testing.T) {
		t.Run("non_symmetric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
					1: U64Value(56),
				},
			}

			continuity := opBOR(scope, []byte{2, 0, 1})
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

			continuity := opBOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot bor with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success_u64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(56),
					1: U64Value(11),
				},
			}

			continuity := opBOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(59), scope.memory[2])
		})

		t.Run("success_i64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(-56),
					1: I64Value(11),
				},
			}

			continuity := opBOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, I64Value(-53), scope.memory[2])
		})

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
					1: &U256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.NewInt(59)}, scope.memory[2])
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			res := big.NewInt(-53)

			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
					1: &I256Value{uint256.NewInt(11)},
				},
			}

			continuity := opBOR(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromBig(res)}, scope.memory[2])
		})
	})

	t.Run("BNOT", func(t *testing.T) {
		t.Run("non_numeric", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: StringValue("foo"),
				},
			}

			continuity := opBNOT(scope, []byte{2, 0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.ValueError",
				Error: "cannot bnot with string registers",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("success_u64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: U64Value(56),
				},
			}

			continuity := opBNOT(scope, []byte{2, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, U64Value(18446744073709551559), scope.memory[2])
		})

		t.Run("success_i64", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: I64Value(-56),
				},
			}

			continuity := opBNOT(scope, []byte{2, 0})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, I64Value(55), scope.memory[2])
		})

		t.Run("success_u256", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &U256Value{uint256.NewInt(56)},
				},
			}

			continuity := opBNOT(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &U256Value{uint256.MustFromDecimal("115792089237316195423570985008687907853269984665640564039457584007913129639879")}, scope.memory[2]) //nolint:lll
		})

		t.Run("success_i256", func(t *testing.T) {
			ip1 := big.NewInt(-56)
			scope := &callscope{
				engine: &Engine{callstack: make(callstack, 0), runtime: &runtime},
				memory: map[byte]RegisterValue{
					0: &I256Value{uint256.MustFromBig(ip1)},
				},
			}

			continuity := opBNOT(scope, []byte{2, 0, 1})
			require.Equal(t, continueOk{20}, continuity)
			require.Equal(t, &I256Value{uint256.MustFromDecimal("55")}, scope.memory[2])
		})
	})

	t.Run("LOGIC", func(t *testing.T) {
		t.Run("unavailable", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{runtime: &runtime, callstack: make(callstack, 0)},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLOGIC(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.AccessError",
				Error: "persistent state is unavailable",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("available", func(t *testing.T) {
			logicAddress := types.Address(randomAddressValue(t))
			logicID := types.NewLogicIDv0(true, false, false, false, 0, logicAddress)

			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					persistent: engineio.NewDebugContextDriver(logicAddress, logicID),
				},
				memory: map[byte]RegisterValue{},
			}

			continuity := opLOGIC(scope, []byte{0})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, LogicContextValue{addr: AddressValue(logicAddress)}, scope.memory[0])
		})
	})

	t.Run("SENDER", func(t *testing.T) {
		t.Run("unavailable", func(t *testing.T) {
			scope := &callscope{
				engine: &Engine{runtime: &runtime, callstack: make(callstack, 0)},
				memory: map[byte]RegisterValue{},
			}

			continuity := opSENDER(scope, []byte{0})
			require.Equal(t, continueException{0, &Exception{
				Class: "builtin.AccessError",
				Error: "sender ephemeral state is unavailable",
				Trace: []string{},
			}}, continuity)
		})

		t.Run("available", func(t *testing.T) {
			logicAddress := types.Address(randomAddressValue(t))
			senderAddress := types.Address(randomAddressValue(t))
			logicID := types.NewLogicIDv0(false, false, false, false, 0, logicAddress)

			scope := &callscope{
				engine: &Engine{
					runtime: &runtime, callstack: make(callstack, 0),
					sephemeral: engineio.NewDebugContextDriver(senderAddress, logicID),
				},
				memory: map[byte]RegisterValue{},
			}

			continuity := opSENDER(scope, []byte{0})
			require.Equal(t, continueOk{30}, continuity)
			require.Equal(t, ParticipantContextValue{addr: AddressValue(senderAddress)}, scope.memory[0])
		})
	})
}
