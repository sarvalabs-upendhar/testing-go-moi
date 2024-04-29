package engineio

import (
	"encoding/json"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type manifestV1 struct {
	header   ManifestHeader
	elements []ManifestElement
	elemptrs map[uint64]uint64
}

func (manifest manifestV1) Header() ManifestHeader { return manifest.header }
func (manifest manifestV1) Engine() ManifestEngine { return manifest.header.Engine }
func (manifest manifestV1) Syntax() uint64         { return manifest.header.Syntax }
func (manifest manifestV1) Kind() EngineKind       { return manifest.header.Engine.Kind }

func (manifest manifestV1) Size() uint64                { return uint64(len(manifest.elements)) }
func (manifest manifestV1) Elements() []ManifestElement { return manifest.elements }

func (manifest manifestV1) GetElement(ptr ElementPtr) (ManifestElement, bool) {
	idx, ok := manifest.elemptrs[ptr]
	if !ok {
		return ManifestElement{}, false
	}

	return manifest.elements[idx], true
}

func (manifest manifestV1) Hash() [32]byte {
	// Generate the polo encoded manifest bytes
	encoded, err := manifest.Encode(common.POLO)
	if err != nil {
		panic("manifest encode to polo failed!")
	}

	// Generate blake2b hash of the polo encoded manifest
	return blake2b.Sum256(encoded)
}

func (manifest manifestV1) Encode(encoding common.Encoding) ([]byte, error) {
	switch encoding {
	case common.POLO:
		return polo.Polorize(manifest)
	case common.JSON:
		return json.Marshal(manifest)
	case common.YAML:
		return yaml.Marshal(manifest)
	default:
		return nil, errors.New("unsupported encoding format")
	}
}

func (manifest *manifestV1) Decode(encoding common.Encoding, data []byte) error {
	switch encoding {
	case common.POLO:
		return polo.Depolorize(manifest, data)
	case common.JSON:
		return json.Unmarshal(data, manifest)
	case common.YAML:
		return yaml.Unmarshal(data, manifest)
	default:
		return errors.New("unsupported encoding format for manifest")
	}
}

func (manifest manifestV1) Polorize() (*polo.Polorizer, error) {
	// Create a new polorizer buffer and encode the syntax edition
	buffer := polo.NewPolorizer()
	buffer.PolorizeUint(manifest.Syntax()) // [0] syntax

	// Create a new buffer to encode the engine struct
	engine := polo.NewPolorizer()
	engine.PolorizeString(manifest.Engine().Kind.String()) // [1][0] kind

	// Create a new buffer and encode all the engine flags
	flags := polo.NewPolorizer()
	for _, flag := range manifest.Engine().Flags {
		flags.PolorizeString(flag)
	}

	// Encode the flags into the engine buffer
	engine.PolorizePacked(flags) // [1][1] flags
	// Encode the engine into the main buffer
	buffer.PolorizePacked(engine) // [1] engine

	// Create a new buffer for the elements
	elements := polo.NewPolorizer()
	// Encode each element into the element buffer
	for _, element := range manifest.elements {
		encoded := polo.NewPolorizer()

		// Encode the element properties
		encoded.PolorizeUint(element.Ptr)    // [2][0] ptr
		_ = encoded.Polorize(element.Deps)   // [2][1] deps
		encoded.PolorizeString(element.Kind) // [2][2] kind
		// Encode the element data
		if err := encoded.Polorize(element.Data); err != nil { // [2][3] data
			return nil, err
		}

		// Add the encoded element into the elements buffer
		elements.PolorizePacked(encoded)
	}

	// Encode the elements into the main buffer
	buffer.PolorizePacked(elements) // [2] elements

	return buffer, nil
}

func (manifest *manifestV1) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	// Decode syntax edition from the buffer
	manifest.header.Syntax, err = depolorizer.DepolorizeUint() // [0] syntax
	if err != nil {
		return err
	}

	// Obtain the engine data pack from the buffer
	engineHeader, err := depolorizer.DepolorizePacked() // [1] engine
	if err != nil {
		return err
	}

	// Obtain the engine kind
	kind, err := engineHeader.DepolorizeString() // [1][0] kind
	if err != nil {
		return err
	}

	engine, ok := FetchEngine(EngineKindFromString(kind))
	if !ok {
		return errors.New("manifest engine unavailable")
	}

	// Decode the engine flags
	flags := make([]string, 0)
	if err = engineHeader.Depolorize(&flags); err != nil { // [1][1] flags
		return err
	}

	manifest.header.Engine = ManifestEngine{
		Kind:  engine.Kind(),
		Flags: flags,
	}

	// Obtain the elements data pack from the main buffer
	elements, err := depolorizer.DepolorizePacked() // [2] elements
	if err != nil {
		return err
	}

	manifest.elements = make([]ManifestElement, 0)
	manifest.elemptrs = make(map[uint64]uint64)

	for !elements.Done() {
		element, err := elements.DepolorizePacked()
		if err != nil {
			return err
		}

		elementPtr, err := element.DepolorizeUint() // [2][0] ptr
		if err != nil {
			return err
		}

		elementDeps := make([]uint64, 0) // [2][1] deps
		if err = element.Depolorize(&elementDeps); err != nil {
			return err
		}

		elementKind, err := element.DepolorizeString() // [2][2] kind
		if err != nil {
			return err
		}

		elementData, err := element.DepolorizeAny() // [2][3] data
		if err != nil {
			return err
		}

		// Generate an object for the element's data
		// We get the object from the engine's generator function
		object, ok := engine.GenerateManifestElement(elementKind)
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", elementKind)
		}

		// Decode the element data into the object
		if err := polo.Depolorize(object, elementData); err != nil {
			return err
		}

		// Create a ManifestElement and insert into the manifest
		manifest.elemptrs[elementPtr] = uint64(len(manifest.elements))
		manifest.elements = append(manifest.elements, ManifestElement{
			Kind: elementKind,
			Ptr:  elementPtr,
			Deps: elementDeps,
			Data: object,
		})
	}

	return nil
}

type manifestV1JSON struct {
	Syntax uint64 `json:"syntax"`
	Engine struct {
		Kind  string   `json:"kind"`
		Flags []string `json:"flags"`
	} `json:"engine"`
	Elements []manifestElementV1JSON `json:"elements"`
}

type manifestElementV1JSON struct {
	Ptr  uint64          `json:"ptr"`
	Deps []uint64        `json:"deps"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

func (manifest manifestV1) MarshalJSON() ([]byte, error) {
	raw := new(manifestV1JSON)

	raw.Syntax = manifest.header.Syntax
	raw.Engine.Kind = manifest.Kind().String()
	raw.Engine.Flags = manifest.header.Engine.Flags
	raw.Elements = make([]manifestElementV1JSON, 0, len(manifest.elements))

	for _, element := range manifest.elements {
		data, err := json.Marshal(element.Data)
		if err != nil {
			return nil, err
		}

		encoded := manifestElementV1JSON{
			Ptr:  element.Ptr,
			Deps: element.Deps,
			Kind: element.Kind,
			Data: data,
		}

		raw.Elements = append(raw.Elements, encoded)
	}

	return json.Marshal(raw)
}

func (manifest *manifestV1) UnmarshalJSON(data []byte) error {
	raw := new(manifestV1JSON)
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}

	engine, ok := FetchEngine(EngineKindFromString(raw.Engine.Kind))
	if !ok {
		return errors.New("manifest engine unavailable")
	}

	manifest.header = ManifestHeader{
		Syntax: raw.Syntax,
		Engine: ManifestEngine{
			Kind:  engine.Kind(),
			Flags: raw.Engine.Flags,
		},
	}

	manifest.elements = make([]ManifestElement, 0, len(raw.Elements))
	manifest.elemptrs = make(map[uint64]uint64, len(raw.Elements))

	for idx, element := range raw.Elements {
		object, ok := engine.GenerateManifestElement(element.Kind)
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", element.Kind)
		}

		if err := json.Unmarshal(element.Data, object); err != nil {
			return err
		}

		manifest.elemptrs[element.Ptr] = uint64(idx)
		manifest.elements = append(manifest.elements, ManifestElement{
			Kind: element.Kind,
			Ptr:  element.Ptr,
			Deps: element.Deps,
			Data: object,
		})
	}

	return nil
}

type manifestV1YAML struct {
	Syntax uint64 `yaml:"syntax"`
	Engine struct {
		Kind  string   `yaml:"kind"`
		Flags []string `yaml:"flags"`
	} `yaml:"engine"`
	Elements []manifestElementV1YAML `yaml:"elements"`
}

type manifestElementV1YAML struct {
	Ptr  uint64    `yaml:"ptr"`
	Deps []uint64  `yaml:"deps"`
	Kind string    `yaml:"kind"`
	Data yaml.Node `yaml:"data"`
}

func (manifest manifestV1) MarshalYAML() (interface{}, error) {
	raw := new(manifestV1YAML)

	raw.Syntax = manifest.header.Syntax
	raw.Engine.Kind = manifest.Kind().String()
	raw.Engine.Flags = manifest.header.Engine.Flags
	raw.Elements = make([]manifestElementV1YAML, 0, len(manifest.elements))

	for _, element := range manifest.elements {
		data := new(yaml.Node)
		if err := data.Encode(element.Data); err != nil {
			return nil, err
		}

		encoded := manifestElementV1YAML{
			Ptr:  element.Ptr,
			Deps: element.Deps,
			Kind: element.Kind,
			Data: *data,
		}

		raw.Elements = append(raw.Elements, encoded)
	}

	return raw, nil
}

func (manifest *manifestV1) UnmarshalYAML(value *yaml.Node) error {
	raw := new(manifestV1YAML)
	if err := value.Decode(raw); err != nil {
		return err
	}

	engine, ok := FetchEngine(EngineKindFromString(raw.Engine.Kind))
	if !ok {
		return errors.New("manifest engine unavailable")
	}

	manifest.header = ManifestHeader{
		Syntax: raw.Syntax,
		Engine: ManifestEngine{
			Kind:  engine.Kind(),
			Flags: raw.Engine.Flags,
		},
	}

	manifest.elements = make([]ManifestElement, 0, len(raw.Elements))
	manifest.elemptrs = make(map[uint64]uint64, len(raw.Elements))

	for idx, element := range raw.Elements {
		object, ok := engine.GenerateManifestElement(element.Kind)
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", element.Kind)
		}

		if err := element.Data.Decode(object); err != nil {
			return err
		}

		manifest.elemptrs[element.Ptr] = uint64(idx)
		manifest.elements = append(manifest.elements, ManifestElement{
			Kind: element.Kind,
			Ptr:  element.Ptr,
			Deps: element.Deps,
			Data: object,
		})
	}

	return nil
}
