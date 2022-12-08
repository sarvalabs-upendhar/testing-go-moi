package pisa

// Constant represents a constant value declaration.
// It consists of the type information of the constant (primitive)
// and some POLO encoded bytes that describe the constant value.
type Constant struct {
	Type PrimitiveType
	Data []byte
}

// ConstantTable represents a collection of Constant
// objects indexed by their 64-bit pointer (uint64)
type ConstantTable map[uint64]*Constant

// lookup retrieves a Constant object from the ConstantTable for
// a given pointer with a boolean indicating if it exists.
func (table ConstantTable) fetch(ptr uint64) (*Constant, bool) { //nolint:unused
	constant, exists := table[ptr]

	return constant, exists
}

// insert adds a Constant object into the ConstantTable at the specified pointer.
func (table ConstantTable) insert(ptr uint64, constant *Constant) { //nolint:unused
	table[ptr] = constant
}
