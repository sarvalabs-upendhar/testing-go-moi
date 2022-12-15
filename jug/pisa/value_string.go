package pisa

import (
	"strings"

	"github.com/sarvalabs/go-polo"
)

// StringValue represents a Value that operates like a string.
type StringValue string

// NewStringValue generates a new StringValue for a given string value.
func NewStringValue(str string) StringValue { return StringValue(str) }

// DefaultStringValue generates a new StringValue with an empty string.
func DefaultStringValue() StringValue { return "" }

// Type returns the Datatype of StringValue, which is TypeString.
// Implements the Value interface for StringValue.
func (str StringValue) Type() *Datatype { return TypeString }

// Copy returns a copy of StringValue as a Value.
// Implements the Value interface for StringValue.
func (str StringValue) Copy() Value { return StringValue(strings.Clone(string(str))) }

// Data returns the POLO encoded bytes of StringValue.
// Implements the Value interface for StringValue.
func (str StringValue) Data() []byte {
	data, _ := polo.Polorize(str)

	return data
}
