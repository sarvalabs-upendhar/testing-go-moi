package pisa

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/types"
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

	t.Run("Helpers", func(t *testing.T) {
		// Create a new BoolValue
		value := StringValue("boofar")
		value2 := StringValue("-")

		// Test Concat()
		assert.Equal(t, StringValue("boofar-boofar"), value.Concat(value2).Concat(value))

		// Test HasPrefix()
		prefix1 := StringValue("boo")
		assert.True(t, bool(value.HasPrefix(prefix1)))
		prefix2 := StringValue("hello")
		assert.False(t, bool(value.HasPrefix(prefix2)))
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
}

func TestStringMethods(t *testing.T) {
	runtime := NewRuntime()

	//nolint:dupl
	t.Run("__eq__[0x3]", func(t *testing.T) {
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
		method := methodsString()[0x3]
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

	t.Run("__bool__[0x7]", func(t *testing.T) {
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
		method := methodsString()[0x7]
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

	t.Run("__str__[0x8]", func(t *testing.T) {
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
		method := methodsString()[0x8]
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

	t.Run("__len__[0xA]", func(t *testing.T) {
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
		method := methodsString()[0xA]
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
	t.Run("Get [0x10]", func(t *testing.T) {
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
		}
		method := methodsString()[0x10]
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

	//nolint:dupl
	t.Run("Set [0x11]", func(t *testing.T) {
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
		}
		method := methodsString()[0x11]
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
		method := methodsString()[0x12]
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
		method := methodsString()[0x13]
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
		method := methodsString()[0x14]
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
		method := methodsString()[0x15]
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
		method := methodsString()[0x16]
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
		method := methodsString()[0x17]
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
		method := methodsString()[0x18]
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
		tests := []struct {
			str   StringValue
			delim StringValue
			res   []StringValue
			err   *Exception
		}{
			{"Hello,ok", ",", []StringValue{"Hello", "ok"}, nil},
			{"to/in/for", "/", []StringValue{"to", "in", "for"}, nil},
			{"abc", "", []StringValue{"a", "b", "c"}, nil},
			{"a1b1c1d", "1", []StringValue{"a", "b", "c", "d"}, nil},
			{"1.2.3", ".", []StringValue{"1", "2", "3"}, nil},
		}
		method := methodsString()[0x19]
		for _, test := range tests {
			scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
			outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.delim})
			polorizedRes, _ := polo.Polorize(test.res)
			if test.err != nil {
				assert.Equal(t, test.err, except)
			} else {
				assert.Nil(t, except)
				assert.Equal(t, polorizedRes, outputs.Get(0).Data())
			}
		}
	})

	//nolint:dupl
	t.Run("Slice [0x1A]", func(t *testing.T) {
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
		}
		method := methodsString()[0x1A]
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
		method := methodsString()[0x1B]
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
		method := methodsString()[0x1C]
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
}

func TestByteMethods(t *testing.T) {
	runtime := NewRuntime()

	t.Run("Get [0x10]", func(t *testing.T) {
		tests := []struct {
			testbyte BytesValue
			pos      U64Value
			res      BytesValue
			err      *Exception
		}{
			{[]byte{10, 20, 30, 50, 70}, 1, []byte{20}, nil},
			{[]byte{1}, 0, []byte{1}, nil},
			{[]byte{0xb, 0x3, 0x5}, 2, []byte{0x5}, nil},
		}
		method := methodsBytes()[0x10]
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
		}
		method := methodsBytes()[0x11]
		for _, test := range tests {
			scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
			outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.testbyte, 1: test.pos, 2: test.testbyte2})
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
		method := methodsBytes()[0x12]
		for _, test := range tests {
			scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
			outputs, except := method.Builtin.runner(
				scope.engine,
				RegisterSet{0: test.testbyte, 1: test.prefix},
			)
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
		method := methodsBytes()[0x13]
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
		method := methodsBytes()[0x14]
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
		tests := []struct {
			str   BytesValue
			delim BytesValue
			res   []BytesValue
			err   *Exception
		}{
			{[]byte("Hello,ok"), []byte(","), []BytesValue{[]byte("Hello"), []byte("ok")}, nil},
			{[]byte("to/in/for"), []byte("/"), []BytesValue{[]byte("to"), []byte("in"), []byte("for")}, nil},
			{[]byte("abc"), []byte(""), []BytesValue{[]byte("a"), []byte("b"), []byte("c")}, nil},
			{[]byte("a1b1c1d"), []byte("1"), []BytesValue{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}, nil},
		}
		method := methodsBytes()[0x15]
		for _, test := range tests {
			scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
			outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.str, 1: test.delim})
			polorizedRes, _ := polo.Polorize(test.res)
			if test.err != nil {
				assert.Equal(t, test.err, except)
			} else {
				assert.Nil(t, except)
				assert.Equal(t, polorizedRes, outputs.Get(0).Data())
			}
		}
	})

	t.Run("Slice [0x16]", func(t *testing.T) {
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
		}
		method := methodsBytes()[0x16]
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
}

func randomAddressValue(t *testing.T) AddressValue {
	t.Helper()

	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return AddressValue(types.BytesToAddress(address))
}
