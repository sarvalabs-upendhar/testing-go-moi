package pisa

import (
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
)

// FieldTable represents a collection of typefield objects.
// The fields are indexed by both their position and name.
type FieldTable struct {
	Table   map[uint8]*typefield
	Symbols map[string]uint8
}

// NewFieldTable generates a FieldTable from a map of positions to field expression strings.
// Each field expression must be '{name} [{datatype}]', where datatype must be a valid type expression.
// Returns an error if the given map of field expressions contains positional gaps or invalid expressions.
func NewFieldTable(table map[uint8]string) (*FieldTable, error) {
	// Create a blank field table
	fields := &FieldTable{
		make(map[uint8]*typefield, len(table)),
		make(map[string]uint8, len(table)),
	}

	// Ensure that there are less than 256 field expressions
	if len(table) > 256 {
		return nil, errors.New("cannot have more than 256 fields for FieldTable")
	}

	// Iterate through each position, querying the expression map for each position
	// If an expression is missing for a position, the FieldTable cannot be generated with gaps.
	for position := uint8(0); position < uint8(len(table)); position++ {
		// Query for the field expression and error if not found
		expr, ok := table[position]
		if !ok {
			return nil, errors.Errorf("missing field in position '%v' for FieldTable", position)
		}

		// Parse the field expression into a typefield
		parsed, err := parseTypefield(expr)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid field expression in position '%v' for FieldTable", position)
		}

		// Insert the typefield into the FieldTable
		fields.insert(parsed.Name, position, parsed)
	}

	return fields, nil
}

// String returns the fields of the FieldTable as a string.
// Format of the returned string is '(type1, type2, ...)'.
// Implements the Stringer interface for FieldTable.
func (fields FieldTable) String() string {
	fieldTypes := make([]string, 0, len(fields.Symbols))
	for position := uint8(0); position < uint8(len(fields.Table)); position++ {
		fieldTypes = append(fieldTypes, fields.Table[position].Type.String())
	}

	combined := strings.Join(fieldTypes, ", ")
	combined = "(" + combined + ")"

	return combined
}

// fetch retrieves a typefield from the FieldTable for a given position.
// Returns nil if there is typefield for that position
func (fields FieldTable) fetch(position uint8) *typefield { //nolint:unused
	return fields.Table[position]
}

// insert adds a typefield into the FieldTable at the given position with a given name.
func (fields *FieldTable) insert(name string, index uint8, field *typefield) {
	fields.Table[index] = field
	fields.Symbols[name] = index
}

// lookup retrieves a typefield from the FieldTable for a given name.
// Returns nil if there is no typefield for that name.
func (fields FieldTable) lookup(name string) *typefield { //nolint:unused
	index, exists := fields.Symbols[name]
	if !exists {
		return nil
	}

	return fields.Table[index]
}

// typefield represent a named field for composite object such as
// storage and calldata fields as well as class and event attributes
type typefield struct {
	Name string
	Type *Datatype
}

// parseTypefield attempts to parse an input into a typefield.
// An expression of a type field has a name and a typedata expression. The pattern for a type
// field expression is -> '{name} [{datatype}]', where datatype must be a valid type expression.
func parseTypefield(input string) (*typefield, error) {
	// Create a new parser
	parser := newTypeParser(input)
	// Check that parser's cursor token is an identifier
	if !parser.IsCursor(symbolizer.TokenIdentifier) {
		return nil, errors.New("type field does not begin with identifier")
	}

	// Capture identifier literal as the declaration name
	name := parser.Cursor().Literal
	parser.Advance()

	// Unwrap [] enclosed data from the parser
	enclosed, err := parser.Unwrap(symbolizer.EnclosureSquare())
	if err != nil {
		return nil, errors.Wrap(err, "type field type data malformed")
	}

	// Parse the enclosed data into a datatype
	dt, err := ParseDatatype(enclosed)
	if err != nil {
		return nil, errors.Wrap(err, "invalid type field type data")
	}

	// Create a Symbol with the name and type data
	return &typefield{Name: name, Type: dt}, nil
}
