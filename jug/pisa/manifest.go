package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// ManifestSchemaV1 represents the Manifest Schema for PISA.
// This schema definition is only valid for syntax == "1"
type ManifestSchemaV1 struct {
	Syntax string `yaml:"syntax" json:"syntax"`
	Engine string `yaml:"engine" json:"engine"`

	EngineFlags []string `yaml:"flags" json:"flags"`

	Storage  map[uint8]string `yaml:"storage" json:"storage"`
	Builders []RoutineSchema  `yaml:"builders" json:"builders"`

	Constants map[uint64]string        `yaml:"constants" json:"constants"`
	Routines  map[uint64]RoutineSchema `yaml:"routines" json:"routines"`

	Typedefs map[uint64]string           `yaml:"typedefs" json:"typedefs"`
	Events   map[uint64]map[uint8]string `yaml:"events" json:"events"`
	Classes  map[uint64]ClassSchema      `yaml:"classes" json:"classes"`
}

// Hash returns the hash of the Manifest
func (manifest ManifestSchemaV1) Hash() types.Hash {
	return types.GetHash(manifest.Bytes())
}

// Bytes returns the serialized POLO bytes of the Manifest
func (manifest ManifestSchemaV1) Bytes() []byte {
	// Polorize the manifest
	data, _ := polo.Polorize(manifest)
	// Return the polorized data
	return data
}

func (manifest *ManifestSchemaV1) FromBytes(data []byte) error {
	// Depolorize the given data into the Manifest
	if err := polo.Depolorize(manifest, data); err != nil {
		return errors.Wrap(err, "failed to depolorize manifest")
	}

	return nil
}

// ClassSchema represents a structural layout and an intermediary
// format for defining a Logic Class for the PISA Execution Engine
type ClassSchema struct {
	Fields  map[uint8]string         `yaml:"fields" json:"fields"`
	Methods map[string]RoutineSchema `yaml:"methods" json:"methods"`
}

// RoutineSchema represents a structural layout and an intermediary
// format for defining a Logic Routine for the PISA Execution Engine
type RoutineSchema struct {
	Name string `yaml:"name" json:"name"`

	Accepts map[uint8]string `yaml:"accepts" json:"accepts"`
	Returns map[uint8]string `yaml:"returns" json:"returns"`
	Catches []string         `yaml:"catches" json:"catches"`

	Executes Bytecode `yaml:"executes" json:"executes"`
}

// Bytecode represents the intermediary format for PISA instructions.
// Only one of Bytecode or Assembly will be used (Bytecode if both are set)
type Bytecode struct {
	Bytecode []byte   `yaml:"bytecode" json:"bytecode"`
	Assembly []string `yaml:"assembly" json:"assembly"`
}
