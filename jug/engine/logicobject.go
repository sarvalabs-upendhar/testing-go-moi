package engine

import (
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/moichain/types"
)

// LogicObject is a generic container for representing an executable logic.
// It contains fields for representing the kind and metadata of the logic
// along with references to callable endpoints and all its logic elements.
type LogicObject struct {
	// Represents the logic ID
	ID types.Hash
	// Represents the kind of execution engine for the logic
	Engine Kind
	// Represents configuration from the manifest that can be used to reconstruct it
	Metadata ManifestConfig

	// Represents whether the logic is stateful
	Stateful bool
	// Represents whether the logic has been sealed.
	// A sealed logic disables mutation of its state if it is stateful.
	// A logic will become sealed when its state is migrated into another LogicObject
	Sealed bool

	// Represents the collection of all logic elements
	Elements ElementSet
	// Represents a mapping between named callable logic procedures to their element index.
	// The kind string for logic procedures is engine specific and is required for retrieving
	// the element from the logic objects' element set.
	Callables map[string]uint64
}

// NewLogicObject generates a new LogicObject for some given parameters.
// Accepts the engine Kind, whether the logic is stateful and the manifest metadata.
// It also expects a map of callable endpoints indexed by their string key, the value
// representing the element index for the logic function that can be invoked externally.
// The elements to be inserted into the LogicObject are also expected in a slice of Element objects.
func NewLogicObject(
	engine Kind,
	stateful bool,
	config ManifestConfig,
	callables map[string]uint64,
	elements []*Element,
) *LogicObject {
	// Create a new LogicObject with given data
	logic := &LogicObject{
		Engine:    engine,
		Stateful:  stateful,
		Sealed:    false,
		Metadata:  config,
		Elements:  make(ElementSet, len(elements)),
		Callables: callables,
	}

	// Insert given elements into the LogicObject element set
	logic.Elements.Insert(elements...)
	// Generate the Logic ID for the LogicObject
	logic.generateLogicID()

	return logic
}

// generateLogicID is a method of LogicObject that generates and set the logic ID into the object.
// TODO: Implement (this is temporary logic)
func (logic *LogicObject) generateLogicID() {
	data, _ := polo.Polorize(logic)
	hash := blake2b.Sum256(data)
	logic.ID = hash
}
