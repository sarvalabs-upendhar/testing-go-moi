package pisa

// TypeTable represents a type management table for PISA.
// It contains symbolic type definitions, builtin type methods
// as well external class and event definitions.
type TypeTable struct {
	// Represents symbolic type definitions
	symbolic map[uint64]*Datatype
	// Represents builtin type methods
	builtins map[PrimitiveType]BuiltinMethods

	// Represents class type definitions and methods
	classes map[string]*Datatype
	// Represents event type definitions
	events map[string]*Datatype
}

// BaseTypeTable generates a new TypeTable with only builtin methods initialized.
// Symbolic typedefs and external class and event definitions need to be inserted prior to execution.
func BaseTypeTable() TypeTable {
	return TypeTable{
		symbolic: make(map[uint64]*Datatype),
		classes:  make(map[string]*Datatype),
		events:   make(map[string]*Datatype),
		builtins: map[PrimitiveType]BuiltinMethods{
			PrimitiveString: {},
			PrimitiveBytes:  {},
			PrimitiveBool:   {},
		},
	}
}

// insertSymbolic inserts a symbolic typedef into the TypeTable at the given pointer address.
func (table *TypeTable) insertSymbolic(ptr uint64, datatype *Datatype) {
	table.symbolic[ptr] = datatype
}

// BuiltinMethods represents a collection of BuiltinMethods
type BuiltinMethods [256]*BuiltinMethod

// BuiltinMethod represents a method for a Builtin type (Primitive).
// Implements the Method interface
type BuiltinMethod struct {
	fields    CallFields
	primitive PrimitiveType
	execute   func(*ExecutionContext, ValueTable) (ValueTable, error)
}

func (method BuiltinMethod) Builtin() bool         { return true }
func (method BuiltinMethod) Datatype() *Datatype   { return method.primitive.Datatype() }
func (method BuiltinMethod) Interface() CallFields { return method.fields }

func (method BuiltinMethod) Execute(ctx *ExecutionContext, inputs ValueTable) (ValueTable, error) {
	return method.execute(ctx, inputs)
}
