package engineio

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// ManifestEncoding is an enum with variants that describe
// encoding schemes supported on Manifest objects
type ManifestEncoding int

const (
	POLO ManifestEncoding = iota
	JSON
	YAML
)

type Manifest struct {
	encoding ManifestEncoding

	Syntax   string
	Engine   ManifestEngineSpec
	Elements []ManifestElement
}

type ManifestEngineSpec struct {
	Kind  string   `yaml:"kind" json:"kind"`
	Flags []string `yaml:"flags" json:"flags"`
}

type ManifestElement struct {
	Ptr  uint64
	Kind string
	Deps []uint64
	Data ManifestElementObject
}

type ManifestElementObject interface {
	polo.Polorizable
	polo.Depolorizable
}

type (
	ManifestElementGenerator func() ManifestElementObject
	ManifestElementRegistry  map[string]ManifestElementGenerator
)

var elementRegistries = map[EngineKind]ManifestElementRegistry{}

func RegisterElementRegistry(kind EngineKind, registry ManifestElementRegistry) {
	elementRegistries[kind] = registry
}

func NewManifest(data []byte, encoding ManifestEncoding) (*Manifest, error) {
	manifest := new(Manifest)

	switch encoding {
	case JSON:
		if err := json.Unmarshal(data, manifest); err != nil {
			return nil, err
		}

	case POLO:
		if err := polo.Depolorize(manifest, data); err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("unsupported manifest encoding")
	}

	manifest.encoding = encoding

	return manifest, nil
}

func (manifest Manifest) Header() ManifestHeader {
	return ManifestHeader{manifest.Syntax, manifest.Engine}
}

func (manifest Manifest) Hash() (types.Hash, error) {
	bytes, err := manifest.Encode(POLO)
	if err != nil {
		return types.NilHash, err
	}

	return types.GetHash(bytes), nil
}

func (manifest Manifest) Encode(encoding ManifestEncoding) ([]byte, error) {
	switch encoding {
	case JSON:
		return json.Marshal(manifest)
	case POLO:
		return polo.Polorize(manifest)

	default:
		return nil, errors.New("unsupported manifest encoding")
	}
}

// ManifestHeader represents a simple header for a Manifest and describes its syntax form
// and engine config. Useful for determining which engine to use to handle the Manifest.
// Every engine's manifest implementation must be able to decode into this header.
type ManifestHeader struct {
	Syntax string             `yaml:"syntax" json:"syntax"`
	Engine ManifestEngineSpec `yaml:"engine" json:"engine"`
}

// LogicEngine returns the normalized form of the logic engine value in the ManifestHeader.
// It is capitalized to uppercase letter and converted into a types.LogicEngine
func (header ManifestHeader) LogicEngine() EngineKind {
	return EngineKind(strings.ToUpper(header.Engine.Kind))
}

func (header ManifestHeader) validate() (ManifestElementRegistry, error) {
	if header.Syntax != "0.1.0" {
		return nil, errors.New("unsupported manifest syntax")
	}

	registry, ok := elementRegistries[header.LogicEngine()]
	if !ok {
		return nil, errors.New("unsupported manifest engine: element registry not found")
	}

	return registry, nil
}

func (manifest *Manifest) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	type ManifestPOLO struct {
		Syntax   string
		Engine   ManifestEngineSpec
		Elements []struct {
			Ptr  uint64
			Kind string
			Deps []uint64
			Data polo.Raw
		}
	}

	raw := new(ManifestPOLO)
	if err = depolorizer.Depolorize(raw); err != nil {
		return err
	}

	manifest.Syntax = raw.Syntax
	manifest.Engine = raw.Engine

	registry, err := manifest.Header().validate()
	if err != nil {
		return err
	}

	manifest.Elements = make([]ManifestElement, 0, len(raw.Elements))

	for _, element := range raw.Elements {
		generator, ok := registry[element.Kind]
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", element.Kind)
		}

		elementDepolorizer, err := polo.NewDepolorizer(element.Data)
		if err != nil {
			return err
		}

		object := generator()
		if err = object.Depolorize(elementDepolorizer); err != nil {
			return err
		}

		manifest.Elements = append(manifest.Elements, ManifestElement{
			Ptr:  element.Ptr,
			Kind: element.Kind,
			Deps: element.Deps,
			Data: object,
		})
	}

	return nil
}

func (manifest *Manifest) UnmarshalJSON(data []byte) (err error) {
	type ManifestJSON struct {
		Syntax   string             `json:"syntax"`
		Engine   ManifestEngineSpec `json:"engine"`
		Elements []struct {
			Ptr  uint64          `json:"ptr"`
			Kind string          `json:"kind"`
			Deps []uint64        `json:"deps"`
			Data json.RawMessage `json:"data"`
		} `json:"elements"`
	}

	raw := new(ManifestJSON)
	if err = json.Unmarshal(data, raw); err != nil {
		return err
	}

	manifest.Syntax = raw.Syntax
	manifest.Engine = raw.Engine

	registry, err := manifest.Header().validate()
	if err != nil {
		return err
	}

	manifest.Elements = make([]ManifestElement, 0, len(raw.Elements))

	for _, element := range raw.Elements {
		generator, ok := registry[element.Kind]
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", element.Kind)
		}

		object := generator()
		if err = json.Unmarshal(element.Data, object); err != nil {
			return err
		}

		manifest.Elements = append(manifest.Elements, ManifestElement{
			Ptr:  element.Ptr,
			Kind: element.Kind,
			Deps: element.Deps,
			Data: object,
		})
	}

	return nil
}
