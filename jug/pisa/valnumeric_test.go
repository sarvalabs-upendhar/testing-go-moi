package pisa

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	fuzz "github.com/google/gofuzz"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

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
	})
}

func TestU256Methods(t *testing.T) {
	t.Run("Numeric Methods", func(t *testing.T) {
		f := fuzz.New()

		t.Run("Add", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(20)), nil},
				{U256Value(*uint256.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})), U256Value(*uint256.NewInt(0x10)), U256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")},                                                                                                                                                                        //nolint:lll
				{U256Value(*uint256.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe})), U256Value(*uint256.NewInt(0x01)), U256Value(*uint256.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})), nil}, //nolint:lll
			}
			for _, test := range tests {
				res, except := test.input.Add(test.input2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		t.Run("Add (Fuzzy)", func(t *testing.T) {
			var x, y U256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).ToBig()
				bigY := (*uint256.Int)(&y).ToBig()

				bigSum := bigX.Add(bigX, bigY)
				bigRes, overflow := uint256.FromBig(bigSum)

				result, except := x.Add(y)

				if overflow {
					require.Equal(t, exception(OverflowError, "addition overflow"), except)
					require.Equal(t, U256Value(*uint256.NewInt(0)), result)
				} else {
					require.Nil(t, except)
					require.Equal(t, U256Value(*bigRes), result)
				}
			}
		})

		t.Run("Sub", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), nil},
			}
			for _, test := range tests {
				res, except := test.input.Sub(test.input2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		t.Run("Sub (Fuzzy)", func(t *testing.T) {
			var x, y U256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).ToBig()
				bigY := (*uint256.Int)(&y).ToBig()

				bigDiff := bigX.Sub(bigX, bigY)
				bigRes, _ := uint256.FromBig(bigDiff)

				result, except := x.Sub(y)

				// If x<y big.Sub() returns X, while Sub function returns overflow exception
				if except != nil {
					require.Equal(t, exception(OverflowError, "subtraction overflow"), except)
					require.Equal(t, U256Value(*uint256.NewInt(0)), result)
					require.Equal(t, bigX, bigDiff)
				} else {
					require.Nil(t, except)
					require.Equal(t, U256Value(*bigRes), result)
				}
			}
		})

		t.Run("Mul", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(100)), nil},
				{U256Value(*uint256.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})), U256Value(*uint256.NewInt(0x10)), U256Value(*uint256.NewInt(0)), exception(OverflowError, "multiplication overflow")}, //nolint:lll
			}
			for _, test := range tests {
				res, except := test.input.Mul(test.input2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		t.Run("Mul (Fuzzy)", func(t *testing.T) {
			var x, y U256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).ToBig()
				bigY := (*uint256.Int)(&y).ToBig()

				bigSum := bigX.Mul(bigX, bigY)
				bigRes, overflow := uint256.FromBig(bigSum)

				result, except := x.Mul(y)

				if overflow {
					require.Equal(t, exception(OverflowError, "multiplication overflow"), except)
					require.Equal(t, U256Value(*uint256.NewInt(0)), result)
				} else {
					require.Nil(t, except)
					require.Equal(t, U256Value(*bigRes), result)
				}
			}
		})

		t.Run("Div", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(1)), nil},
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), U256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "division by zero")}, //nolint:lll
			}
			for _, test := range tests {
				res, except := test.input.Div(test.input2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		//nolint:dupl
		t.Run("Div (Fuzzy)", func(t *testing.T) {
			var x, y U256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).ToBig()
				bigY := (*uint256.Int)(&y).ToBig()

				result, except := x.Div(y)

				// Big division panics if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "division by zero"), except)
					require.Equal(t, U256Value(*uint256.NewInt(0)), result)
				} else {
					bigQuotient := bigX.Div(bigX, bigY)
					bigRes, _ := uint256.FromBig(bigQuotient)
					require.Nil(t, except)
					require.Equal(t, U256Value(*bigRes), result)
				}
			}
		})

		t.Run("Mod", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), nil},
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), U256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "modulo division by zero")}, //nolint:lll
			}
			for _, test := range tests {
				res, except := test.input.Mod(test.input2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		//nolint:dupl
		t.Run("Mod (Fuzzy)", func(t *testing.T) {
			var x, y U256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).ToBig()
				bigY := (*uint256.Int)(&y).ToBig()

				result, except := x.Mod(y)

				// Big division panics if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "modulo division by zero"), except)
					require.Equal(t, U256Value(*uint256.NewInt(0)), result)
				} else {
					bigRemainder := bigX.Mod(bigX, bigY)
					bigRes, _ := uint256.FromBig(bigRemainder)
					require.Nil(t, except)
					require.Equal(t, U256Value(*bigRes), result)
				}
			}
		})

		t.Run("Incr", func(t *testing.T) {
			tests := []struct {
				input U256Value
				res   U256Value
				err   *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(11)), nil},
				{U256Value(*uint256.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})), U256Value(*uint256.NewInt(0x01)), exception(OverflowError, "increment overflow")}, //nolint:lll
			}
			for _, test := range tests {
				res, except := test.input.Incr()
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, res)
				}
			}
		})

		t.Run("Gt", func(t *testing.T) {
			tests := []struct {
				input  U256Value
				input2 U256Value
				res    BoolValue
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), false, nil},
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), true, nil},
			}
			for _, test := range tests {
				res := test.input.Gt(test.input2)
				assert.Equal(t, test.res, res)
			}
		})
	})

	t.Run("Register Methods", func(t *testing.T) {
		runtime := NewRuntime()
		maxValTemp, _ := uint256.FromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		maxVal := U256Value(*maxValTemp)
		t.Run("__join__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodJoin]

			tests := []struct {
				input  U256Value
				input2 U256Value
				res    U256Value
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(30)), nil},
				{U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(1000)), U256Value(*uint256.NewInt(1020)), nil},
				{U256Value(*uint256.NewInt(0)), U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), nil},
				{maxVal, U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")}, //nolint:lll
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input, 1: test.input2})

				if test.err != nil {
					assert.Equal(t, test.err.Class, except.Class)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__lt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodLt]

			tests := []struct {
				input  U256Value
				input2 U256Value
				res    BoolValue
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(20)), true, nil},
				{U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(1000)), true, nil},
				{U256Value(*uint256.NewInt(50)), U256Value(*uint256.NewInt(10)), false, nil},
				{maxVal, U256Value(*uint256.NewInt(10)), false, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input, 1: test.input2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__gt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodGt]

			tests := []struct {
				input  U256Value
				input2 U256Value
				res    BoolValue
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(20)), false, nil},
				{U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(1)), true, nil},
				{U256Value(*uint256.NewInt(50)), U256Value(*uint256.NewInt(50)), false, nil},
				{maxVal, U256Value(*uint256.NewInt(10)), true, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input, 1: test.input2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__eq__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodEq]

			tests := []struct {
				input  U256Value
				input2 U256Value
				res    BoolValue
				err    *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(20)), false, nil},
				{U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(1)), false, nil},
				{U256Value(*uint256.NewInt(50)), U256Value(*uint256.NewInt(50)), true, nil},
				{maxVal, U256Value(*uint256.NewInt(10)), false, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input, 1: test.input2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__bool__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodBool]

			tests := []struct {
				input U256Value
				res   BoolValue
				err   *Exception
			}{
				{U256Value(*uint256.NewInt(10)), true, nil},
				{U256Value(*uint256.NewInt(20)), true, nil},
				{U256Value(*uint256.NewInt(0)), false, nil},
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

		t.Run("__str__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][MethodStr]

			tests := []struct {
				input U256Value
				res   StringValue
				err   *Exception
			}{
				{U256Value(*uint256.NewInt(100)), "100", nil},
				{U256Value(*uint256.NewInt(20)), "20", nil},
				{maxVal, "115792089237316195423570985008687907853269984665640564039457584007913129639935", nil},
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

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU256][0x10]

			tests := []struct {
				input U256Value
				res   U256Value
				err   *Exception
			}{
				{U256Value(*uint256.NewInt(10)), U256Value(*uint256.NewInt(10)), nil},
				{U256Value(*uint256.NewInt(20)), U256Value(*uint256.NewInt(20)), nil},
				{U256Value(*uint256.NewInt(0)), U256Value(*uint256.NewInt(0)), nil},
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

func TestI256Methods(t *testing.T) {
	t.Run("Numeric Methods", func(t *testing.T) {
		f := fuzz.New()

		t.Run("Add", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(10), big.NewInt(-2), big.NewInt(8), nil},
				{big.NewInt(10), big.NewInt(-12), big.NewInt(-2), nil},
				{big.NewInt(-2), big.NewInt(10), big.NewInt(8), nil},
				{big.NewInt(-12), big.NewInt(10), big.NewInt(-2), nil},
				{big.NewInt(10), big.NewInt(2), big.NewInt(12), nil},
				{big.NewInt(-10), big.NewInt(-2), big.NewInt(-12), nil},
				{big.NewInt(0).SetBytes([]byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}), big.NewInt(0x01), big.NewInt(0), exception(OverflowError, "addition overflow")}, //nolint:lll
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				res := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				_ = res.SetFromBig(test.res)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt, except := test1.Add(test2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, res, (*uint256.Int)(&resobt))
				}
			}
		})

		//nolint:dupl
		t.Run("Add (Fuzzy)", func(t *testing.T) {
			var x, y I256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).Abs((*uint256.Int)(&x)).ToBig()
				if (*uint256.Int)(&x).Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}
				bigY := (*uint256.Int)(&y).Abs((*uint256.Int)(&y)).ToBig()
				if (*uint256.Int)(&y).Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}
				bigSum := bigX.Add(bigX, bigY)
				bigRes, _ := uint256.FromBig(bigSum)

				result, except := x.Add(y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "addition overflow"), except)
					require.Equal(t, I256Value(*uint256.NewInt(0)), result)
					require.Equal(t, bigX, bigSum)
				} else {
					require.Nil(t, except)
					require.Equal(t, I256Value(*bigRes), result)
				}
			}
		})

		t.Run("Sub", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(10), big.NewInt(-2), big.NewInt(12), nil},
				{big.NewInt(10), big.NewInt(-12), big.NewInt(22), nil},
				{big.NewInt(-2), big.NewInt(10), big.NewInt(-12), nil},
				{big.NewInt(-12), big.NewInt(10), big.NewInt(-22), nil},
				{big.NewInt(10), big.NewInt(2), big.NewInt(8), nil},
				{big.NewInt(-10), big.NewInt(-2), big.NewInt(-8), nil},
				{big.NewInt(-1), big.NewInt(0).SetBytes([]byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}), big.NewInt(0), exception(OverflowError, "subtraction overflow")}, //nolint:lll
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				res := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				_ = res.SetFromBig(test.res)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt, except := test1.Sub(test2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, res, (*uint256.Int)(&resobt))
				}
			}
		})

		//nolint:dupl
		t.Run("Sub (Fuzzy)", func(t *testing.T) {
			var x, y I256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).Abs((*uint256.Int)(&x)).ToBig()
				if (*uint256.Int)(&x).Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}
				bigY := (*uint256.Int)(&y).Abs((*uint256.Int)(&y)).ToBig()
				if (*uint256.Int)(&y).Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}
				bigSum := bigX.Sub(bigX, bigY)
				bigRes, _ := uint256.FromBig(bigSum)

				result, except := x.Sub(y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "subtraction overflow"), except)
					require.Equal(t, I256Value(*uint256.NewInt(0)), result)
					require.Equal(t, bigX, bigSum)
				} else {
					require.Nil(t, except)
					require.Equal(t, I256Value(*bigRes), result)
				}
			}
		})

		t.Run("Mul", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(10), big.NewInt(-2), big.NewInt(-20), nil},
				{big.NewInt(10), big.NewInt(-12), big.NewInt(-120), nil},
				{big.NewInt(-2), big.NewInt(10), big.NewInt(-20), nil},
				{big.NewInt(-12), big.NewInt(10), big.NewInt(-120), nil},
				{big.NewInt(10), big.NewInt(2), big.NewInt(20), nil},
				{big.NewInt(-10), big.NewInt(-2), big.NewInt(20), nil},
				{big.NewInt(-10), big.NewInt(0).SetBytes([]byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}), big.NewInt(0), exception(OverflowError, "multiplication overflow")}, //nolint:lll
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				res := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				_ = res.SetFromBig(test.res)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt, except := test1.Mul(test2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, res, (*uint256.Int)(&resobt))
				}
			}
		})

		//nolint:dupl
		t.Run("Mul (Fuzzy)", func(t *testing.T) {
			var x, y I256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).Abs((*uint256.Int)(&x)).ToBig()
				if (*uint256.Int)(&x).Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}
				bigY := (*uint256.Int)(&y).Abs((*uint256.Int)(&y)).ToBig()
				if (*uint256.Int)(&y).Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}
				bigSum := bigX.Mul(bigX, bigY)
				bigRes, _ := uint256.FromBig(bigSum)

				result, except := x.Mul(y)

				if except != nil {
					require.Equal(t, exception(OverflowError, "multiplication overflow"), except)
					require.Equal(t, I256Value(*uint256.NewInt(0)), result)
					require.Equal(t, bigX, bigSum)
				} else {
					require.Nil(t, except)
					require.Equal(t, I256Value(*bigRes), result)
				}
			}
		})

		t.Run("Div", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(10), big.NewInt(-2), big.NewInt(-5), nil},
				{big.NewInt(10), big.NewInt(-12), big.NewInt(0), nil},
				{big.NewInt(-2), big.NewInt(10), big.NewInt(0), nil},
				{big.NewInt(-12), big.NewInt(10), big.NewInt(-1), nil},
				{big.NewInt(10), big.NewInt(2), big.NewInt(5), nil},
				{big.NewInt(-10), big.NewInt(-2), big.NewInt(5), nil},
				{big.NewInt(-10), big.NewInt(0), big.NewInt(0), exception(DivideByZeroError, "division by zero")},
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				res := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				_ = res.SetFromBig(test.res)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt, except := test1.Div(test2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, res, (*uint256.Int)(&resobt))
				}
			}
		})

		//nolint:dupl
		t.Run("Div (Fuzzy)", func(t *testing.T) {
			var x, y I256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).Abs((*uint256.Int)(&x)).ToBig()
				if (*uint256.Int)(&x).Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}
				bigY := (*uint256.Int)(&y).Abs((*uint256.Int)(&y)).ToBig()
				if (*uint256.Int)(&y).Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}

				result, except := x.Div(y)

				// Big division panics if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "division by zero"), except)
					require.Equal(t, I256Value(*uint256.NewInt(0)), result)
				} else {
					bigQuotient := bigX.Div(bigX, bigY)
					bigRes, _ := uint256.FromBig(bigQuotient)
					require.Nil(t, except)
					require.Equal(t, I256Value(*bigRes), result)
				}
			}
		})

		t.Run("Mod", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(10), big.NewInt(-2), big.NewInt(0), nil},
				{big.NewInt(10), big.NewInt(-12), big.NewInt(10), nil},
				{big.NewInt(-2), big.NewInt(10), big.NewInt(-2), nil},
				{big.NewInt(-12), big.NewInt(10), big.NewInt(-2), nil},
				{big.NewInt(10), big.NewInt(2), big.NewInt(0), nil},
				{big.NewInt(-10), big.NewInt(-2), big.NewInt(0), nil},
				{big.NewInt(-10), big.NewInt(0), big.NewInt(0), exception(DivideByZeroError, "modulo division by zero")},
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				res := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				_ = res.SetFromBig(test.res)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt, except := test1.Mod(test2)
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, res, (*uint256.Int)(&resobt))
				}
			}
		})

		//nolint:dupl
		t.Run("Mod (Fuzzy)", func(t *testing.T) {
			var x, y I256Value
			for i := 0; i < 1000; i++ {
				f.Fuzz(&x)
				f.Fuzz(&y)

				bigX := (*uint256.Int)(&x).Abs((*uint256.Int)(&x)).ToBig()
				if (*uint256.Int)(&x).Sign() == -1 {
					bigX = bigX.Neg(bigX)
				}
				bigY := (*uint256.Int)(&y).Abs((*uint256.Int)(&y)).ToBig()
				if (*uint256.Int)(&y).Sign() == -1 {
					bigY = bigY.Neg(bigY)
				}

				result, except := x.Mod(y)

				// Big division panics if division by 0 occurs
				if bigY.Cmp(big.NewInt(0)) == 0 {
					require.Equal(t, exception(DivideByZeroError, "modulo division by zero"), except)
					require.Equal(t, I256Value(*uint256.NewInt(0)), result)
				} else {
					bigQuotient := bigX.Mod(bigX, bigY)
					bigRes, _ := uint256.FromBig(bigQuotient)
					require.Nil(t, except)
					require.Equal(t, I256Value(*bigRes), result)
				}
			}
		})

		t.Run("Incr", func(t *testing.T) {
			tests := []struct {
				input *big.Int
				res   *big.Int
				err   *Exception
			}{
				{big.NewInt(-1), big.NewInt(0), nil},
				{big.NewInt(0), big.NewInt(1), nil},
				{big.NewInt(20), big.NewInt(21), nil},
				{big.NewInt(0).SetBytes([]byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}), big.NewInt(1), exception(OverflowError, "increment overflow")}, //nolint:lll
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				resval := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = resval.SetFromBig(test.res)
				testval := I256Value(*ip1)
				resop := I256Value(*resval)
				resobt, except := testval.Incr()
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, resop, resobt)
				}
			}
		})

		t.Run("Decr", func(t *testing.T) {
			tests := []struct {
				input *big.Int
				res   *big.Int
				err   *Exception
			}{
				{big.NewInt(0), big.NewInt(-1), nil},
				{big.NewInt(3), big.NewInt(2), nil},
				{big.NewInt(-20), big.NewInt(-21), nil},
				{big.NewInt(0).SetBytes([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}), big.NewInt(0x00), exception(OverflowError, "decrement overflow")}, //nolint:lll
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				resval := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = resval.SetFromBig(test.res)
				testval := I256Value(*ip1)
				resop := I256Value(*resval)
				resobt, except := testval.Decr()
				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, resop, resobt)
				}
			}
		})

		t.Run("Gt", func(t *testing.T) {
			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    BoolValue
				err    *Exception
			}{
				{big.NewInt(-10), big.NewInt(10), false, nil},
				{big.NewInt(-10), big.NewInt(-20), true, nil},
				{big.NewInt(100), big.NewInt(-20), true, nil},
			}
			for _, test := range tests {
				ip1 := uint256.NewInt(0)
				ip2 := uint256.NewInt(0)
				_ = ip1.SetFromBig(test.input)
				_ = ip2.SetFromBig(test.input2)
				test1 := I256Value(*ip1)
				test2 := I256Value(*ip2)
				resobt := test1.Gt(test2)
				assert.Equal(t, test.res, resobt)
			}
		})
	})

	t.Run("Register Methods", func(t *testing.T) {
		runtime := NewRuntime()
		maxVal := big.NewInt(0).SetBytes([]byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) //nolint:lll
		minVal := big.NewInt(0).SetBytes([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:lll
		t.Run("__join__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodJoin]

			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    *big.Int
				err    *Exception
			}{
				{big.NewInt(-10), big.NewInt(20), big.NewInt(10), nil},
				{big.NewInt(-10), big.NewInt(20), big.NewInt(10), nil},
				{big.NewInt(-10), big.NewInt(20), big.NewInt(10), nil},
				{maxVal, big.NewInt(20), big.NewInt(0), exception(OverflowError, "addition overflow")},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				ip2, _ := uint256.FromBig(test.input2)
				ip2val := I256Value(*ip2)
				res, _ := uint256.FromBig(test.res)
				resval := I256Value(*res)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval, 1: ip2val})

				if test.err != nil {
					assert.Equal(t, test.err.Class, except.Class)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, resval, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__lt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodLt]

			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    BoolValue
				err    *Exception
			}{
				{big.NewInt(-10), big.NewInt(20), true, nil},
				{big.NewInt(-10), big.NewInt(-20), false, nil},
				{big.NewInt(100), big.NewInt(200), true, nil},
				{maxVal, big.NewInt(20), false, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				ip2, _ := uint256.FromBig(test.input2)
				ip2val := I256Value(*ip2)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval, 1: ip2val})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__gt__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodGt]

			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    BoolValue
				err    *Exception
			}{
				{big.NewInt(-10), big.NewInt(20), false, nil},
				{big.NewInt(-10), big.NewInt(-20), true, nil},
				{big.NewInt(100), big.NewInt(200), false, nil},
				{maxVal, big.NewInt(20), true, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				ip2, _ := uint256.FromBig(test.input2)
				ip2val := I256Value(*ip2)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval, 1: ip2val})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
		//nolint:dupl
		t.Run("__eq__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodEq]

			tests := []struct {
				input  *big.Int
				input2 *big.Int
				res    BoolValue
				err    *Exception
			}{
				{big.NewInt(-10), big.NewInt(20), false, nil},
				{big.NewInt(-20), big.NewInt(-20), true, nil},
				{big.NewInt(100), big.NewInt(200), false, nil},
				{maxVal, big.NewInt(20), false, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				ip2, _ := uint256.FromBig(test.input2)
				ip2val := I256Value(*ip2)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval, 1: ip2val})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__bool__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodBool]

			tests := []struct {
				input *big.Int
				res   BoolValue
				err   *Exception
			}{
				{big.NewInt(-10), true, nil},
				{big.NewInt(-20), true, nil},
				{big.NewInt(0), false, nil},
				{maxVal, true, nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__str__", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][MethodStr]

			tests := []struct {
				input *big.Int
				res   StringValue
				err   *Exception
			}{
				{big.NewInt(-10), "-10", nil},
				{big.NewInt(0), "0", nil},
				{big.NewInt(100), "100", nil},
				{maxVal, "57896044618658097711785492504343953926634992332820282019728792003956564819967", nil},
				{minVal, "-57896044618658097711785492504343953926634992332820282019728792003956564819968", nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI256][0x10]
			tests := []struct {
				input *big.Int
				res   *big.Int
				err   *Exception
			}{
				{big.NewInt(-10), big.NewInt(10), nil},
				{big.NewInt(0), big.NewInt(0), nil},
				{big.NewInt(100), big.NewInt(100), nil},
			}
			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				ip, _ := uint256.FromBig(test.input)
				ipval := I256Value(*ip)
				res, _ := uint256.FromBig(test.res)
				resval := I256Value(*res)
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: ipval})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, resval, outputs.Get(0))
				}
			}
		})
	})
}
