package runtime

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/types"
)

// TypeTable represents a type management table for PISA.
// It contains symbolic type definitions, builtin type methods
// as well external class and event definitions.
type TypeTable struct {
	// Represents symbolic type definitions
	symbolic map[uint64]*register.Typedef
	// Represents builtin type methods
	builtins map[register.PrimitiveType]register.MethodTable

	// Represents class type definitions and methods
	classes map[string]*register.Typedef
	// Represents event type definitions
	events map[string]*register.Typedef
}

// BabylonTypeTable generates a new TypeTable with only builtin methods initialized.
// Symbolic typedefs and external class and event definitions need to be inserted prior to execution.
func BabylonTypeTable() TypeTable {
	return TypeTable{
		symbolic: make(map[uint64]*register.Typedef),
		classes:  make(map[string]*register.Typedef),
		events:   make(map[string]*register.Typedef),

		builtins: map[register.PrimitiveType]register.MethodTable{
			register.PrimitiveBool:    register.BoolMethods(),
			register.PrimitiveBytes:   register.BytesMethods(),
			register.PrimitiveString:  register.StringMethods(),
			register.PrimitiveU64:     register.U64Methods(),
			register.PrimitiveI64:     register.I64Methods(),
			register.PrimitiveAddress: register.AddressMethods(),
		},
	}
}

// GetMethod retrieves a Method from the TypeTable for a given Typedef and MethodCode
func (table *TypeTable) GetMethod(datatype *register.Typedef, methodCode register.MethodCode) (register.Method, bool) {
	if datatype.Kind() == register.Primitive {
		if methods, ok := table.builtins[datatype.P]; ok {
			if method := methods[methodCode]; method != nil {
				return method, true
			}
		}
	}

	return nil, false
}

func (table *TypeTable) Size() int {
	return len(table.symbolic) // + len(table.classes) + ...
}

func (table *TypeTable) SetSymbolic(ptr uint64, datatype *register.Typedef) {
	table.symbolic[ptr] = datatype
}

func (table *TypeTable) GetSymbolic(ptr uint64) (*register.Typedef, bool) {
	datatype, ok := table.symbolic[ptr]

	return datatype, ok
}

func (table *TypeTable) EjectElements() []*types.LogicElement {
	elements := make([]*types.LogicElement, 0, table.Size())

	for index, typedef := range table.symbolic {
		// Polorize the symbolic typedef
		data, _ := polo.Polorize(typedef)
		// Create a LogicElement for the symbolic typedef and append it
		elements = append(elements, &types.LogicElement{Kind: ElementCodeTypedef, Index: index, Data: data})
	}

	// todo: generate elements for classes and events

	return elements
}
