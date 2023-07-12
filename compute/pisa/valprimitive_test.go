package pisa

import (
	"encoding/hex"
	"math/big"
	"math/rand"
	"testing"

	"github.com/holiman/uint256"

	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/go-moi/common"
)

func TestBoolValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new BoolValue
		value := BoolValue(true)

		// Test Type()
		assert.Equal(t, PrimitiveBool, value.Type(), "BoolValue Type should be TypeBool")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of BoolValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, true, norm, "Normalized value of BoolValue should be equal to bool value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x2}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of BoolValue should match expected value")
	})

	t.Run("Helpers", func(t *testing.T) {
		// Create a new BoolValue
		value := BoolValue(true)

		// Test And()
		assert.False(t, bool(value.And(false)))
		assert.True(t, bool(value.And(true)))

		// Test Or()
		assert.True(t, bool(value.Or(false)))
		assert.True(t, bool(value.Or(true)))

		// Test Not()
		assert.False(t, bool(value.Not()))
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__eq__ [0x3]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBool][MethodEq]

			tests := []struct {
				x, y, z BoolValue
			}{
				{true, true, true},
				{true, false, false},
				{false, true, false},
				{false, false, true},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.x, 1: test.y})

				assert.Nil(t, except)
				assert.Equal(t, test.z, outputs.Get(0), "%v, %v", test.x, test.y)
			}
		})
	})
}

func TestStringValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new StringValue
		value := StringValue("foobar")

		// Test Type()
		assert.Equal(t, PrimitiveString, value.Type(), "StringValue Type should be TypeString")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of StringValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, "foobar", norm, "Normalized value of StringValue should be equal to string value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x6, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of StringValue should match expected value")
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__join__ [0x3]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x3]

			tests := []struct {
				str  StringValue
				str2 StringValue
				res  StringValue
				err  *Exception
			}{
				{"Hello", "kk", "Hellokk", nil},
				{"", "", "", nil},
				{"a", " ", "a ", nil},
				{"--", "//", "--//", nil},
				{" ", "123", " 123", nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.str2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__eq__ [0x3]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][MethodEq]

			tests := []struct {
				x, y StringValue
				z    BoolValue
			}{
				{"foo", "bar", false},
				{"foo", "foo", true},
				{"bar", "boo", false},
				{"boo", "boo", true},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.x, 1: test.y})

				assert.Nil(t, except)
				assert.Equal(t, test.z, outputs.Get(0), "%v, %v", test.x, test.y)
			}
		})

		t.Run("__bool__ [0x7]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x7]

			tests := []struct {
				str StringValue
				res BoolValue
				err *Exception
			}{
				{"Hello", true, nil},
				{"", false, nil},
				{"a", true, nil},
				{"--", true, nil},
				{" ", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__str__ [0x8]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x8]

			tests := []struct {
				str StringValue
				res StringValue
				err *Exception
			}{
				{"Hello", "Hello", nil},
				{"", "", nil},
				{"a", "a", nil},
				{"--", "--", nil},
				{" ", " ", nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("__addr__ [0x9]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x9]

			tests := []struct {
				input  StringValue
				output AddressValue
				except *Exception
			}{
				{
					"62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa",
					AddressValue(common.BytesToAddress(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")))), //nolint:lll
					nil,
				},
				{
					"0x62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa",
					AddressValue(common.BytesToAddress(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")))), //nolint:lll
					nil,
				},
				{
					"62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaaf6",
					ZeroAddress,
					exception(ValueError, "data too long for address"),
				},
				{
					"0x62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaaf6",
					ZeroAddress,
					exception(ValueError, "data too long for address"),
				},
				{
					"62db",
					AddressValue(common.BytesToAddress([]byte{0x62, 0xdb})),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.except == nil {
					assert.Nil(t, except)
					assert.Equal(t, test.output, outputs.Get(0))
				} else {
					assert.Equal(t, test.except, except)
					assert.Equal(t, RegisterSet{}, outputs)
				}
			}
		})

		t.Run("__len__ [0xA]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0xA]

			tests := []struct {
				str StringValue
				res U64Value
				err *Exception
			}{
				{"Hello", 5, nil},
				{"", 0, nil},
				{"a", 1, nil},
				{"--", 2, nil},
				{" ", 1, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Get [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x10]

			tests := []struct {
				str StringValue
				pos U64Value
				res StringValue
				err *Exception
			}{
				{"Hello", 3, "l", nil},
				{"cd", 0, "c", nil},
				{"a", 0, "a", nil},
				{"//", 1, "/", nil},
				{" ", 0, " ", nil},
				{" ", 3, " ", exception(AccessError, "index out of bounds")},
				{" ", 1, " ", exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.pos})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Set [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x11]

			tests := []struct {
				str    StringValue
				pos    U64Value
				setter StringValue
				res    StringValue
				err    *Exception
			}{
				{"Hello", 3, "k", "Helko", nil},
				{"cd", 0, "l", "ld", nil},
				{"a", 0, "m", "m", nil},
				{"//", 1, "r", "/r", nil},
				{" ", 0, "1", "1", nil},
				{" ", 3, "1", "", exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.pos, 2: test.setter})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("IsAlpha [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x12]

			tests := []struct {
				str StringValue
				res BoolValue
				err *Exception
			}{
				{"Hello", true, nil},
				{"", false, nil},
				{"a", true, nil},
				{"--", false, nil},
				{"123", false, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("IsNumeric [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x13]

			tests := []struct {
				str StringValue
				res BoolValue
				err *Exception
			}{
				{"Hello", false, nil},
				{"", false, nil},
				{"1", true, nil},
				{"--", false, nil},
				{"123", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("IsLower [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x14]

			tests := []struct {
				str StringValue
				res BoolValue
				err *Exception
			}{
				{"Hello1", false, nil},
				{"abcd1", true, nil},
				{"1", true, nil},
				{"--", true, nil},
				{"moi", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("IsUpper [0x15]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x15]

			tests := []struct {
				str StringValue
				res BoolValue
				err *Exception
			}{
				{"HELLO", true, nil},
				{"ABCD1", true, nil},
				{"abc", false, nil},
				{"--", true, nil},
				{"moi", false, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("HasPrefix [0x16]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x16]

			tests := []struct {
				str    StringValue
				prefix StringValue
				res    BoolValue
				err    *Exception
			}{
				{"Hello", "He", true, nil},
				{"", "", true, nil},
				{"a", "a", true, nil},
				{"//", "l", false, nil},
				{"123 ", "12", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.prefix})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("HasSuffix [0x17]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x17]

			tests := []struct {
				str    StringValue
				suffix StringValue
				res    BoolValue
				err    *Exception
			}{
				{"Hello", "lo", true, nil},
				{"", "", true, nil},
				{"a", "a", true, nil},
				{"car", "l", false, nil},
				{"1234 ", "234 ", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.suffix})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("Contains [0x18]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x18]

			tests := []struct {
				str    StringValue
				substr StringValue
				res    BoolValue
				err    *Exception
			}{
				{"Hello", "ll", true, nil},
				{"", "", true, nil},
				{"a", "a", true, nil},
				{"car", "l", false, nil},
				{"1234 ", "2", true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.substr})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Split [0x19]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x19]

			tests := []struct {
				str   StringValue
				delim StringValue
				res   []RegisterValue
				err   *Exception
			}{
				{
					"Hello,ok", ",",
					[]RegisterValue{StringValue("Hello"), StringValue("ok")},
					nil,
				},
				{
					"to/in/for", "/",
					[]RegisterValue{StringValue("to"), StringValue("in"), StringValue("for")},
					nil,
				},
				{
					"abc", "",
					[]RegisterValue{StringValue("a"), StringValue("b"), StringValue("c")},
					nil,
				},
				{
					"a1b1c1d", "1",
					[]RegisterValue{StringValue("a"), StringValue("b"), StringValue("c"), StringValue("d")},
					nil,
				},
				{
					"1.2.3", ".",
					[]RegisterValue{StringValue("1"), StringValue("2"), StringValue("3")},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.delim})
				expected := must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, test.res...))

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})

		t.Run("Slice [0x1A]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1A]

			tests := []struct {
				str  StringValue
				idx1 U64Value
				idx2 U64Value
				res  StringValue
				err  *Exception
			}{
				{"Hello", 0, 2, "He", nil},
				{"to", 1, 2, "o", nil},
				{"abc", 1, 2, "b", nil},
				{"a1b1c1d", 2, 5, "b1c", nil},
				{"1.2.3", 3, 4, ".", nil},
				{"1.2.3", 3, 9, "", exception(AccessError, "index out of bounds")},
				{"1.2.3", 9, 4, "", exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.idx1, 2: test.idx2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToLower [0x1B]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1B]

			tests := []struct {
				str StringValue
				res StringValue
				err *Exception
			}{
				{"HELLO", "hello", nil},
				{"ABCD1", "abcd1", nil},
				{"abc", "abc", nil},
				{"--", "--", nil},
				{"A", "a", nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToUpper [0x1C]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1C]

			tests := []struct {
				str StringValue
				res StringValue
				err *Exception
			}{
				{"hello", "HELLO", nil},
				{"abcd1", "ABCD1", nil},
				{"abc", "ABC", nil},
				{"--", "--", nil},
				{"A", "A", nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToBytes [0x1D]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1D]

			tests := []struct {
				str StringValue
				res BytesValue
				err *Exception
			}{
				{"hello", BytesValue("hello"), nil},
				{"abcd1", BytesValue("abcd1"), nil},
				{"abc", BytesValue("abc"), nil},
				{"--", BytesValue("--"), nil},
				{"A", BytesValue("A"), nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToU64 [0x1E]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1E]

			tests := []struct {
				str StringValue
				res U64Value
				err *Exception
			}{
				{"0x0", U64Value(0), nil},
				{"0x64", U64Value(100), nil},
				{"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", U64Value(0), exception(ValueError, "strconv.ParseUint: parsing \"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF\": value out of range")}, //nolint:lll
				{"0xFFFFFFFFFFFFFFFF", U64Value(18446744073709551615), nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToI64 [0x1F]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x1F]

			tests := []struct {
				str StringValue
				res I64Value
				err *Exception
			}{
				{"0x0", I64Value(0), nil},
				{"0x64", I64Value(100), nil},
				{"0xFFFFFFFFFFFFFFFF", I64Value(-1), nil},
				{"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", I64Value(0), exception(ValueError, "strconv.ParseUint: parsing \"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF\": value out of range")}, //nolint:lll
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToU256 [0x20]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x20]

			tests := []struct {
				str StringValue
				res *U256Value
				err *Exception
			}{
				{"0x0", &U256Value{value: uint256.NewInt(0)}, nil},
				{"0x64", &U256Value{value: uint256.NewInt(100)}, nil},
				{"0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe", &U256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe")}, nil},               //nolint:lll
				{"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe", &U256Value{uint256.NewInt(0)}, exception(ValueError, "hex number > 256 bits")}, //nolint:lll
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToI256 [0x21]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveString][0x21]

			tests := []struct {
				str StringValue
				res *I256Value
				err *Exception
			}{
				{"0x0", &I256Value{value: uint256.NewInt(0)}, nil},
				{"0x64", &I256Value{value: uint256.NewInt(100)}, nil},
				{"0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe", &I256Value{uint256.MustFromHex("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe")}, nil},               //nolint:lll
				{"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe", &I256Value{uint256.NewInt(0)}, exception(ValueError, "hex number > 256 bits")}, //nolint:lll
				{"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", &I256Value{value: uint256.MustFromBig(big.NewInt(-1))}, nil},                                                              //nolint:lll
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str})

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

func TestBytesValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new BytesValue
		value := BytesValue{0x10, 0x20, 0x30}

		// Test Type()
		assert.Equal(t, PrimitiveBytes, value.Type(), "BytesValue Type should be TypeBytes")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of BytesValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, []byte{0x10, 0x20, 0x30}, norm,
			"Normalized value of BytesValue should be equal to string value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x6, 0x10, 0x20, 0x30}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of BytesValue should match expected value")
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__eq__ [0x3]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][MethodEq]

			tests := []struct {
				x, y BytesValue
				z    BoolValue
			}{
				{BytesValue("foo"), BytesValue("bar"), false},
				{BytesValue("foo"), BytesValue("foo"), true},
				{BytesValue("bar"), BytesValue("boo"), false},
				{BytesValue("boo"), BytesValue("boo"), true},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.x, 1: test.y})

				assert.Nil(t, except)
				assert.Equal(t, test.z, outputs.Get(0), "%v, %v", test.x, test.y)
			}
		})

		t.Run("__addr__ [0x9]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][MethodAddr]

			tests := []struct {
				input  BytesValue
				output AddressValue
				except *Exception
			}{
				{
					BytesValue(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa"))),
					AddressValue(common.BytesToAddress(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")))), //nolint:lll
					nil,
				},
				{
					BytesValue(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaaf6"))),
					ZeroAddress,
					exception(ValueError, "data too long for address"),
				},
				{
					BytesValue(must(hex.DecodeString("62db"))),
					AddressValue(common.BytesToAddress([]byte{0x62, 0xdb})),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				if test.except == nil {
					assert.Nil(t, except)
					assert.Equal(t, test.output, outputs.Get(0))
				} else {
					assert.Equal(t, test.except, except)
					assert.Equal(t, RegisterSet{}, outputs)
				}
			}
		})

		t.Run("Get [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x10]

			tests := []struct {
				testbyte BytesValue
				pos      U64Value
				res      BytesValue
				err      *Exception
			}{
				{[]byte{10, 20, 30, 50, 70}, 1, []byte{20}, nil},
				{[]byte{1}, 0, []byte{1}, nil},
				{[]byte{0xb, 0x3, 0x5}, 2, []byte{0x5}, nil},
				{[]byte{0xb, 0x3, 0x5}, 4, []byte{}, exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.pos})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Set [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x11]

			tests := []struct {
				testbyte  BytesValue
				testbyte2 BytesValue
				pos       U64Value
				res       BytesValue
				err       *Exception
			}{
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{99}, 2, []byte{10, 20, 99, 70, 120, 255}, nil},
				{[]byte{10}, []byte{100}, 0, []byte{100}, nil},
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{55}, 5, []byte{10, 20, 30, 70, 120, 55}, nil},
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{55}, 7, []byte{}, exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{
					0: test.testbyte,
					1: test.pos,
					2: test.testbyte2,
				})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("HasPrefix [0x12]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x12]

			tests := []struct {
				testbyte BytesValue
				prefix   BytesValue
				res      BoolValue
				err      *Exception
			}{
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{20}, false, nil},
				{[]byte{20}, []byte{20}, true, nil},
				{[]byte{2, 1, 1, 1, 1}, []byte{2}, true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.prefix})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("HasSuffix [0x13]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x13]

			tests := []struct {
				testbyte BytesValue
				suffix   BytesValue
				res      BoolValue
				err      *Exception
			}{
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{20}, false, nil},
				{[]byte{20}, []byte{20}, true, nil},
				{[]byte{2, 1, 1, 1, 1}, []byte{1}, true, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.suffix})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("Contains [0x14]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x14]

			tests := []struct {
				testbyte BytesValue
				subbyte  BytesValue
				res      BoolValue
				err      *Exception
			}{
				{[]byte{10, 20, 30, 70, 120, 255}, []byte{20}, true, nil},
				{[]byte{20}, []byte{20}, true, nil},
				{[]byte{2, 1, 1, 1, 1}, []byte{0}, false, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.subbyte})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("Split [0x15]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x15]

			tests := []struct {
				byt   BytesValue
				delim BytesValue
				res   []RegisterValue
				err   *Exception
			}{
				{
					[]byte("Hello,ok"), []byte(","),
					[]RegisterValue{BytesValue("Hello"), BytesValue("ok")},
					nil,
				},
				{
					[]byte("to/in/for"), []byte("/"),
					[]RegisterValue{BytesValue("to"), BytesValue("in"), BytesValue("for")},
					nil,
				},
				{
					[]byte("abc"), []byte(""),
					[]RegisterValue{BytesValue("a"), BytesValue("b"), BytesValue("c")},
					nil,
				},
				{
					[]byte("a1b1c1d"), []byte("1"),
					[]RegisterValue{BytesValue("a"), BytesValue("b"), BytesValue("c"), BytesValue("d")},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.byt, 1: test.delim})
				expected := must(newVarrayFromValues(VarrayDatatype{PrimitiveBytes}, test.res...))

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})

		t.Run("Slice [0x16]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x16]

			tests := []struct {
				testbyte BytesValue
				idx1     U64Value
				idx2     U64Value
				res      BytesValue
				err      *Exception
			}{
				{[]byte{1, 2, 3, 4, 5}, 3, 4, []byte{4}, nil},
				{[]byte{1}, 0, 1, []byte{1}, nil},
				{[]byte{100, 20, 50}, 0, 2, []byte{100, 20}, nil},
				{[]byte{100, 20, 50}, 0, 7, []byte{100, 20}, exception(AccessError, "index out of bounds")},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.idx1, 2: test.idx2})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ToU64 [0x17]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x17]

			tests := []struct {
				byt BytesValue
				res U64Value
				err *Exception
			}{
				{
					[]byte{100},
					U64Value(100),
					nil,
				},
				{
					[]byte{0xFF},
					U64Value(255),
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
					U64Value(0),
					exception(OverflowError, "U64 size overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.byt})
				expected := test.res

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})

		t.Run("ToI64 [0x18]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x18]

			tests := []struct {
				byt BytesValue
				res I64Value
				err *Exception
			}{
				{
					[]byte{100},
					I64Value(100),
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
					I64Value(-1),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.byt})
				expected := test.res

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("ToU256 [0x19]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x19]

			tests := []struct {
				byt BytesValue
				res *U256Value
				err *Exception
			}{
				{
					[]byte{100},
					&U256Value{value: uint256.NewInt(100)},
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
					&U256Value{value: uint256.NewInt(18446744073709551615)},
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, //nolint:lll
					&U256Value{value: uint256.NewInt(0)},
					exception(OverflowError, "U256 size overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.byt})
				expected := test.res

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})

		//nolint:dupl
		t.Run("ToI256 [0x20]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveBytes][0x20]

			tests := []struct {
				byt BytesValue
				res *I256Value
				err *Exception
			}{
				{
					[]byte{100},
					&I256Value{value: uint256.NewInt(100)},
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
					&I256Value{value: uint256.MustFromHex("0xFFFFFFFFFFFFFFFF")},
					nil,
				},
				{
					[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, //nolint:lll
					&I256Value{value: uint256.NewInt(0)},
					exception(OverflowError, "I256 size overflow"),
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.byt})
				expected := test.res

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, expected, outputs.Get(0))
				}
			}
		})
	})
}

func TestAddressValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new AddressValue
		value := AddressValue{0x10}

		// Test Type()
		assert.Equal(t, PrimitiveAddress, value.Type(), "AddressValue Type should be TypeAddress")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of AddressValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, [32]byte{0x10}, norm,
			"Normalized value of AddressValue should be equal to [32]byte value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{
			0x6, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of AddressValue should match expected value")
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("__eq__ [0x3]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveAddress][MethodEq]

			tests := []struct {
				x, y AddressValue
				z    BoolValue
			}{
				{
					AddressValue(common.CreateAddressFromString("foo")),
					AddressValue(common.CreateAddressFromString("bar")),
					false,
				},
				{
					AddressValue(common.CreateAddressFromString("foo")),
					AddressValue(common.CreateAddressFromString("foo")),
					true,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.x, 1: test.y})

				assert.Nil(t, except)
				assert.Equal(t, test.z, outputs.Get(0), "%v, %v", test.x, test.y)
			}
		})

		t.Run("ToBytes [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveAddress][0x10]

			tests := []struct {
				input  AddressValue
				result BytesValue
				err    *Exception
			}{
				{
					AddressValue(common.BytesToAddress(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")))), //nolint:lll
					BytesValue(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa"))),
					nil,
				},
				{
					AddressValue(common.BytesToAddress([]byte{0x62, 0xdb})),
					BytesValue(must(hex.DecodeString("00000000000000000000000000000000000000000000000000000000000062db"))),
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				assert.Nil(t, except)
				assert.Equal(t, test.result, outputs.Get(0))
			}
		})

		t.Run("ToU256 [0x11]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveAddress][0x11]

			tests := []struct {
				input  AddressValue
				result *U256Value
				err    *Exception
			}{
				{
					AddressValue(common.BytesToAddress(must(hex.DecodeString("62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")))), //nolint:lll
					&U256Value{uint256.MustFromHex("0x62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa")},
					nil,
				},
				{
					AddressValue(common.BytesToAddress([]byte{0x62, 0xdb})),
					&U256Value{uint256.MustFromHex("0x62db")},
					nil,
				},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

				assert.Nil(t, except)
				assert.Equal(t, test.result, outputs.Get(0))
			}
		})
	})
}

func randomAddressValue(t *testing.T) AddressValue {
	t.Helper()

	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return AddressValue(common.BytesToAddress(address))
}
