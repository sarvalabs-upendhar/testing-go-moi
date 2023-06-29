package pisa

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	fuzz "github.com/google/gofuzz"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func fuzzU256Value(value *U256Value, c fuzz.Continue) {
	var x uint256.Int

	// Fuzz the u256 object
	c.Fuzz(&x)
	// Set it into a U256Value
	*value = U256Value{&x}
}

func fuzzI256Value(value *I256Value, c fuzz.Continue) {
	var x uint256.Int

	// Fuzz the u256 object
	c.Fuzz(&x)
	// Flip polarity (50%)
	if c.RandBool() {
		x = *new(uint256.Int).Neg(&x)
	}

	// Set it into a U256Value
	*value = I256Value{&x}
}

func TestU64Value(t *testing.T) {
	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x10]

			tests := []struct {
				input U64Value
				res   U64Value
				err   *Exception
			}{
				{10, 10, nil},
				{100, 100, nil},
				{5, 5, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToBytes [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x11]

			tests := []struct {
				input U64Value
				res   BytesValue
				err   *Exception
			}{
				{10, []byte{0x0a}, nil},
				{100, []byte{0x64}, nil},
				{18446744073709551615, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToI64 [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x12]

			tests := []struct {
				input U64Value
				res   I64Value
				err   *Exception
			}{
				{10, 10, nil},
				{100, 100, nil},
				{5, 5, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("ToU256 [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x13]

			tests := []struct {
				input U64Value
				res   *U256Value
				err   *Exception
			}{
				{10, &U256Value{uint256.NewInt(10)}, nil},
				{100, &U256Value{uint256.NewInt(100)}, nil},
				{18446744073709551615, &U256Value{uint256.NewInt(18446744073709551615)}, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("ToI256 [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x14]

			tests := []struct {
				input U64Value
				res   *I256Value
				err   *Exception
			}{
				{10, &I256Value{uint256.NewInt(10)}, nil},
				{100, &I256Value{uint256.NewInt(100)}, nil},
				{18446744073709551615, &I256Value{uint256.NewInt(18446744073709551615)}, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
	})
}

func TestI64Value(t *testing.T) {
	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x10]

			tests := []struct {
				input I64Value
				res   I64Value
				err   *Exception
			}{
				{-10, 10, nil},
				{100, 100, nil},
				{-5, 5, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToBytes [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x11]

			tests := []struct {
				input I64Value
				res   BytesValue
				err   *Exception
			}{
				{10, []byte{0x0a}, nil},
				{100, []byte{0x64}, nil},
				{-1, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToU64 [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x12]

			tests := []struct {
				input I64Value
				res   U64Value
				err   *Exception
			}{
				{10, 10, nil},
				{100, 100, nil},
				{-5, 0, exception(OverflowError, "conversion overflow")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToU256 [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x13]

			tests := []struct {
				input I64Value
				res   *U256Value
				err   *Exception
			}{
				{10, &U256Value{uint256.NewInt(10)}, nil},
				{100, &U256Value{uint256.NewInt(100)}, nil},
				{-5, &U256Value{uint256.NewInt(0)}, exception(OverflowError, "negative conversion to U256")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToI256 [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x14]

			tests := []struct {
				input I64Value
				res   *I256Value
				err   *Exception
			}{
				{10, &I256Value{uint256.NewInt(10)}, nil},
				{100, &I256Value{uint256.NewInt(100)}, nil},
				{-5, &I256Value{uint256.MustFromBig(big.NewInt(-5))}, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
	})
}

func TestU256Methods(t *testing.T) {
	t.Run("Numeric Methods", func(t *testing.T) {
		f := fuzz.New().NilChance(0).Funcs(fuzzU256Value, fuzzI256Value)

		t.Run("Add", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(20)},
					nil,
				},
				{
					MaxU256,
					&U256Value{uint256.NewInt(10)},
					nil,
					exception(OverflowError, "addition overflow"),
				},
				{
					&U256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe")},
					&U256Value{uint256.NewInt(1)},
					MaxU256,
					nil,
				},
			}

			for _, test := range tests {
				result, except := test.input.Add(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Add (Fuzzy)", func(t *testing.T) {
			var x, y U256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX, bigY := x.value.ToBig(), y.value.ToBig()

				bigSum := new(big.Int).Add(bigX, bigY)
				bigRes, overflow := uint256.FromBig(bigSum)

				result, except := x.Add(&y)

				if overflow {
					require.Equal(t, exception(OverflowError, "addition overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &U256Value{bigRes}, result)
				}
			}
		})

		t.Run("Sub", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(0)},
					nil,
				},
			}

			for _, test := range tests {
				result, except := test.input.Sub(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Sub (Fuzzy)", func(t *testing.T) {
			var x, y U256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX, bigY := x.value.ToBig(), y.value.ToBig()

				bigRes, _ := uint256.FromBig(new(big.Int).Sub(bigX, bigY))
				result, except := x.Sub(&y)

				// If x<y big.Sub() returns X, while Sub function returns overflow exception
				if except != nil {
					require.Equal(t, exception(OverflowError, "subtraction overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &U256Value{bigRes}, result)
				}
			}
		})

		t.Run("Mul", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(100)},
					nil,
				},
				{
					&U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")},
					&U256Value{uint256.NewInt(10)},
					nil,
					exception(OverflowError, "multiplication overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Mul(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Mul (Fuzzy)", func(t *testing.T) {
			var x, y U256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX, bigY := x.value.ToBig(), y.value.ToBig()

				bigRes, overflow := uint256.FromBig(new(big.Int).Mul(bigX, bigY))
				result, except := x.Mul(&y)

				if overflow {
					require.Equal(t, exception(OverflowError, "multiplication overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &U256Value{bigRes}, result)
				}
			}
		})

		t.Run("Div", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(1)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(0)},
					nil, exception(DivideByZeroError, "division by zero"),
				},
				{
					&U256Value{uint256.MustFromDecimal("72822368621905888528105189790620129548345576293625510977940121192476616687616")}, //nolint:lll
					&U256Value{uint256.NewInt(2)},
					&U256Value{uint256.MustFromDecimal("36411184310952944264052594895310064774172788146812755488970060596238308343808")}, //nolint:lll
					nil,
				},
				{
					&U256Value{uint256.NewInt(2)},
					&U256Value{uint256.MustFromDecimal("72822368621905888528105189790620129548345576293625510977940121192476616687616")}, //nolint:lll
					&U256Value{uint256.NewInt(0)},
					nil,
				},
			}

			for _, test := range tests {
				result, except := test.input.Div(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Div (Fuzzy)", func(t *testing.T) {
			var x, y U256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX, bigY := x.value.ToBig(), y.value.ToBig()

				result, except := x.Div(&y)

				// Big division raises an exception if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "division by zero"), except)
					require.Nil(t, result)
				} else {
					bigRes, _ := uint256.FromBig(bigX.Div(bigX, bigY))

					require.Nil(t, except)
					require.Equal(t, &U256Value{bigRes}, result)
				}
			}
		})

		t.Run("Mod", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(0)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(0)},
					nil,
					exception(DivideByZeroError, "modulo division by zero"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Mod(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Mod (Fuzzy)", func(t *testing.T) {
			var x, y U256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX, bigY := x.value.ToBig(), y.value.ToBig()

				result, except := x.Mod(&y)

				// Big division panics if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "modulo division by zero"), except)
					require.Nil(t, result)
				} else {
					bigRes, _ := uint256.FromBig(bigX.Mod(bigX, bigY))

					require.Nil(t, except)
					require.Equal(t, &U256Value{bigRes}, result)
				}
			}
		})

		t.Run("Incr", func(t *testing.T) {
			tests := []struct {
				input  *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(11)},
					nil,
				},
				{
					&U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")},
					nil,
					exception(OverflowError, "increment overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Incr()

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})
	})

	t.Run("Register Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__join__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodJoin]

			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(30)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(1000)},
					&U256Value{uint256.NewInt(1020)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(0)},
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					nil,
				},
				{
					MaxU256, &U256Value{uint256.NewInt(10)},
					nil, exception(OverflowError, "addition overflow").traced([]string{}),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__lt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodLt]

			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result BoolValue
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(20)},
					true, nil,
				},
				{
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(1000)},
					true, nil,
				},
				{
					&U256Value{uint256.NewInt(50)},
					&U256Value{uint256.NewInt(10)},
					false, nil,
				},
				{
					MaxU256, &U256Value{uint256.NewInt(10)},
					false, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__gt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodGt]

			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result BoolValue
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(20)},
					false, nil,
				},
				{
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(1)},
					true, nil,
				},
				{
					&U256Value{uint256.NewInt(50)},
					&U256Value{uint256.NewInt(50)},
					false, nil,
				},
				{
					MaxU256, &U256Value{uint256.NewInt(10)},
					true, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__eq__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodEq]

			tests := []struct {
				input  *U256Value
				input2 *U256Value
				result BoolValue
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(20)},
					false, nil,
				},
				{
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(1)},
					false, nil,
				},
				{
					&U256Value{uint256.NewInt(50)},
					&U256Value{uint256.NewInt(50)},
					true, nil,
				},
				{
					MaxU256, &U256Value{uint256.NewInt(10)},
					false, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("__bool__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodBool]

			tests := []struct {
				input  *U256Value
				result BoolValue
				err    *Exception
			}{
				{&U256Value{uint256.NewInt(10)}, true, nil},
				{&U256Value{uint256.NewInt(20)}, true, nil},
				{&U256Value{uint256.NewInt(0)}, false, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("__str__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodStr]

			tests := []struct {
				input  *U256Value
				result StringValue
				err    *Exception
			}{
				{&U256Value{uint256.NewInt(100)}, "100", nil},
				{&U256Value{uint256.NewInt(20)}, "20", nil},
				{MaxU256, "115792089237316195423570985008687907853269984665640564039457584007913129639935", nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("__addr__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodAddr]

			tests := []struct {
				input  *U256Value
				result AddressValue
				err    *Exception
			}{
				{&U256Value{uint256.NewInt(100)}, AddressValue([32]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64}), nil},                  //nolint:lll
				{&U256Value{uint256.NewInt(18446744073709551615)}, AddressValue([32]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}), nil}, //nolint:lll
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x10]

			tests := []struct {
				input  *U256Value
				result *U256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(20)},
					&U256Value{uint256.NewInt(20)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(0)},
					&U256Value{uint256.NewInt(0)},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToBytes [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x11]

			tests := []struct {
				input  *U256Value
				result BytesValue
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					BytesValue([]byte{10}),
					nil,
				},
				{
					&U256Value{uint256.NewInt(2000)},
					BytesValue([]byte{0x7, 0xd0}),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("ToU64 [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x12]

			tests := []struct {
				input  *U256Value
				result U64Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					U64Value(10),
					nil,
				},
				{
					&U256Value{uint256.NewInt(2000)},
					U64Value(2000),
					nil,
				},
				{
					&U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffff")},
					U64Value(0),
					exception(OverflowError, "U64 overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("ToI64 [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x13]

			tests := []struct {
				input  *U256Value
				result I64Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					I64Value(10),
					nil,
				},
				{
					&U256Value{uint256.NewInt(2000)},
					I64Value(2000),
					nil,
				},
				{
					&U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffff")},
					I64Value(0),
					exception(OverflowError, "I64 overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToI256 [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x14]

			tests := []struct {
				input  *U256Value
				result *I256Value
				err    *Exception
			}{
				{
					&U256Value{uint256.NewInt(10)},
					&I256Value{uint256.NewInt(10)},
					nil,
				},
				{
					&U256Value{uint256.NewInt(2000)},
					&I256Value{uint256.NewInt(2000)},
					nil,
				},
				{
					&U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")},
					nil,
					exception(OverflowError, "I256 overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})
	})
}

func TestI256Methods(t *testing.T) {
	t.Run("Numeric Methods", func(t *testing.T) {
		f := fuzz.New().NilChance(0).Funcs(fuzzU256Value, fuzzI256Value)

		t.Run("Add", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(8))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(8))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					&I256Value{uint256.MustFromBig(big.NewInt(12))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					nil,
				},
				{
					MaxI256, &I256Value{uint256.MustFromBig(big.NewInt(1))},
					nil, exception(OverflowError, "addition overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Add(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		//nolint:dupl
		t.Run("Add (Fuzzy)", func(t *testing.T) {
			var x, y I256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := new(uint256.Int).Abs(x.value).ToBig()
				if x.value.Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}

				bigY := new(uint256.Int).Abs(y.value).ToBig()
				if y.value.Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}

				bigRes, _ := uint256.FromBig(new(big.Int).Add(bigX, bigY))
				result, except := x.Add(&y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "addition overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &I256Value{bigRes}, result)
				}
			}
		})

		t.Run("Sub", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(12))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(22))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-22))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					&I256Value{uint256.MustFromBig(big.NewInt(8))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(-8))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))}, MaxI256,
					nil, exception(OverflowError, "subtraction overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Sub(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		//nolint:dupl
		t.Run("Sub (Fuzzy)", func(t *testing.T) {
			var x, y I256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := new(uint256.Int).Abs(x.value).ToBig()
				if x.value.Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}

				bigY := new(uint256.Int).Abs(y.value).ToBig()
				if y.value.Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}

				bigRes, _ := uint256.FromBig(new(big.Int).Sub(bigX, bigY))
				result, except := x.Sub(&y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "subtraction overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &I256Value{bigRes}, result)
				}
			}
		})

		t.Run("Mul", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(-120))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-120))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))}, MaxI256,
					nil, exception(OverflowError, "multiplication overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Mul(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		//nolint:dupl
		t.Run("Mul (Fuzzy)", func(t *testing.T) {
			var x, y I256Value

			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := new(uint256.Int).Abs(x.value).ToBig()
				if x.value.Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}

				bigY := new(uint256.Int).Abs(y.value).ToBig()
				if y.value.Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}

				bigRes, _ := uint256.FromBig(new(big.Int).Mul(bigX, bigY))
				result, except := x.Mul(&y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "multiplication overflow"), except)
					require.Nil(t, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, &I256Value{bigRes}, result)
				}
			}
		})

		t.Run("Div", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("2822368621905888528105189790620129548345576293625510977940121192476616687616"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromDecimal("1411184310952944264052594895310064774172788146812755488970060596238308343808")}, //nolint:lll
					nil,
				},
				{
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromDecimal("3911184310952944264052594895310064774172788146812755488970060596238308343805")}, //nolint:lll
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(-5))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					&I256Value{uint256.MustFromBig(big.NewInt(5))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(5))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
					exception(DivideByZeroError, "division by zero"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Div(test.input2)
				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Mod", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("2822368621905888528105189790620129548345576293625510977940121192476616687616"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromDecimal("0")},
					nil,
				},
				{
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(-7))},
					&I256Value{uint256.MustFromBig(big.NewInt(-3))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-7))},
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.MustFromBig(big.NewInt(-7))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					&I256Value{new(uint256.Int).Neg(uint256.MustFromDecimal("7822368621905888528105189790620129548345576293625510977940121192476616687610"))}, //nolint:lll
					&I256Value{uint256.NewInt(0)},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-12))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-2))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
					exception(DivideByZeroError, "modulo division by zero"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Mod(test.input2)

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Incr", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					&I256Value{uint256.MustFromBig(big.NewInt(1))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					&I256Value{uint256.MustFromBig(big.NewInt(21))},
					nil,
				},
				{
					MaxI256, nil, exception(OverflowError, "increment overflow"),
				},
			}

			for tno, test := range tests {
				result, except := test.input.Incr()

				require.Equal(t, test.err, except, "%v", tno)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Decr", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(3))},
					&I256Value{uint256.MustFromBig(big.NewInt(2))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					&I256Value{uint256.MustFromBig(big.NewInt(-21))},
					nil,
				},
				{
					MinI256, nil, exception(OverflowError, "decrement overflow"),
				},
			}

			for _, test := range tests {
				result, except := test.input.Decr()

				require.Equal(t, test.err, except)
				require.Equal(t, test.result, result)
			}
		})

		t.Run("Gt", func(t *testing.T) {
			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result BoolValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					false, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					true, nil,
				},
			}

			for _, test := range tests {
				result := test.input.Gt(test.input2)
				require.Equal(t, test.result, result)
			}
		})
	})

	t.Run("Register Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__join__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodJoin]

			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					nil,
				},
				{
					MaxI256, &I256Value{uint256.MustFromBig(big.NewInt(20))},
					nil, exception(OverflowError, "addition overflow").traced([]string{}),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__lt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodLt]

			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result BoolValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					false, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					&I256Value{uint256.MustFromBig(big.NewInt(200))},
					true, nil,
				},
				{
					MaxI256, &I256Value{uint256.MustFromBig(big.NewInt(20))},
					false, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__gt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodGt]

			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result BoolValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					false, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					&I256Value{uint256.MustFromBig(big.NewInt(200))},
					false, nil,
				},
				{
					MaxI256, &I256Value{uint256.MustFromBig(big.NewInt(20))},
					true, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		//nolint:dupl
		t.Run("__eq__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodEq]

			tests := []struct {
				input  *I256Value
				input2 *I256Value
				result BoolValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(20))},
					false, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					&I256Value{uint256.MustFromBig(big.NewInt(200))},
					false, nil,
				},
				{
					MaxI256, &I256Value{uint256.MustFromBig(big.NewInt(20))},
					false, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}

				outputs, except := method.Builtin.runner(
					scope.engine,
					RegisterSet{0: test.input, 1: test.input2},
				)

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("__bool__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodBool]

			tests := []struct {
				input  *I256Value
				result BoolValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-20))},
					true, nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					false, nil,
				},
				{
					MaxI256, true, nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("__str__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodStr]

			tests := []struct {
				input  *I256Value
				result StringValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					"-10", nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					"0", nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					"100", nil,
				},
				{
					MaxI256, "57896044618658097711785492504343953926634992332820282019728792003956564819967", nil,
				},
				{
					MinI256, "-57896044618658097711785492504343953926634992332820282019728792003956564819968", nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x10]

			tests := []struct {
				input  *I256Value
				result *I256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-10))},
					&I256Value{uint256.MustFromBig(big.NewInt(10))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					&I256Value{uint256.MustFromBig(big.NewInt(0))},
					nil,
				},
				{
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					&I256Value{uint256.MustFromBig(big.NewInt(100))},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToBytes [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x11]

			tests := []struct {
				input  *I256Value
				result BytesValue
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					BytesValue([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}), //nolint:lll
					nil,
				},
				{
					&I256Value{uint256.NewInt(10)},
					BytesValue([]byte{10}),
					nil,
				},
				{
					&I256Value{uint256.NewInt(2000)},
					BytesValue([]byte{0x7, 0xd0}),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToU64 [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x12]

			tests := []struct {
				input  *I256Value
				result U64Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					U64Value(0),
					exception(OverflowError, "U64 overflow error"),
				},
				{
					&I256Value{uint256.NewInt(10)},
					U64Value(10),
					nil,
				},
				{
					&I256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffff")},
					U64Value(0),
					exception(OverflowError, "U64 overflow error"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToI64 [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x13]

			tests := []struct {
				input  *I256Value
				result I64Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					I64Value(-1),
					nil,
				},
				{
					&I256Value{uint256.NewInt(10)},
					I64Value(10),
					nil,
				},
				{
					&I256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffff")},
					I64Value(0),
					exception(OverflowError, "I64 overflow error"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})

		t.Run("ToU256 [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x14]

			tests := []struct {
				input  *I256Value
				result *U256Value
				err    *Exception
			}{
				{
					&I256Value{uint256.MustFromBig(big.NewInt(-1))},
					&U256Value{Zero256},
					exception(OverflowError, "U256 overflow error"),
				},
				{
					&I256Value{uint256.NewInt(10)},
					&U256Value{uint256.NewInt(10)},
					nil,
				},
				{
					&I256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffff")},
					&U256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffff")},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				result := outputs.Get(0)

				if test.err != nil {
					require.Equal(t, test.err, except)
					require.Equal(t, NullValue{}, result)
				} else {
					require.Nil(t, except)
					require.Equal(t, test.result, result)
				}
			}
		})
	})
}
