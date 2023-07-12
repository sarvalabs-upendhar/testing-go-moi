package engineio

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/moichain/common"
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
	Syntax   string             `yaml:"syntax" json:"syntax"`
	Engine   ManifestEngineSpec `yaml:"engine" json:"engine"`
	Elements []ManifestElement  `yaml:"elements" json:"elements"`
}

type ManifestEngineSpec struct {
	Kind  string   `yaml:"kind" json:"kind"`
	Flags []string `yaml:"flags" json:"flags"`
}

type ManifestElement struct {
	Ptr  ElementPtr            `yaml:"ptr" json:"ptr"`
	Deps []ElementPtr          `yaml:"deps" json:"deps"`
	Kind ElementKind           `yaml:"kind" json:"kind"`
	Data ManifestElementObject `yaml:"data" json:"data"`
}

type ManifestElementObject interface {
	polo.Polorizable
	polo.Depolorizable
}

type ManifestElementGenerator func() ManifestElementObject

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
	case YAML:
		if err := yaml.Unmarshal(data, manifest); err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("unsupported manifest encoding")
	}

	return manifest, nil
}

func ReadManifestFile(path string) (*Manifest, error) {
	path, _ = filepath.Abs(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.Errorf("manifest file not found @ '%v'", path)
	}

	var (
		extension string
		encoding  ManifestEncoding
	)

	switch extension = filepath.Ext(path); extension {
	case ".json":
		encoding = JSON
	case ".polo":
		encoding = POLO
	case ".yaml":
		encoding = YAML
	default:
		return nil, errors.Errorf("manifest file has unsupported extension: '%v'", extension)
	}

	encoded, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest file")
	}

	manifest, err := NewManifest(encoded, encoding)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode %v manifest data", extension)
	}

	return manifest, nil
}

func (manifest Manifest) Header() ManifestHeader {
	return ManifestHeader{manifest.Syntax, manifest.Engine}
}

func (manifest Manifest) Hash() (common.Hash, error) {
	bytes, err := manifest.Encode(POLO)
	if err != nil {
		return common.NilHash, err
	}

	return common.GetHash(bytes), nil
}

func (manifest Manifest) Encode(encoding ManifestEncoding) ([]byte, error) {
	switch encoding {
	case JSON:
		return json.Marshal(manifest)
	case POLO:
		return polo.Polorize(manifest)
	case YAML:
		return yaml.Marshal(manifest)

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

func (header ManifestHeader) validate() error {
	if header.Syntax != "0.1.0" {
		return errors.New("unsupported manifest syntax")
	}

	if _, ok := FetchEngineRuntime(header.LogicEngine()); !ok {
		return errors.New("unsupported manifest engine: element registry not found")
	}

	return nil
}

func (manifest *Manifest) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	type ManifestPOLO struct {
		Syntax   string
		Engine   ManifestEngineSpec
		Elements []struct {
			Ptr  ElementPtr
			Deps []ElementPtr
			Kind ElementKind
			Data polo.Any
		}
	}

	raw := new(ManifestPOLO)
	if err = depolorizer.Depolorize(raw); err != nil {
		return err
	}

	manifest.Syntax = raw.Syntax
	manifest.Engine = raw.Engine

	if err = manifest.Header().validate(); err != nil {
		return err
	}

	runtime, _ := FetchEngineRuntime(manifest.Header().LogicEngine())

	manifest.Elements = make([]ManifestElement, 0, len(raw.Elements))

	for _, element := range raw.Elements {
		generator, ok := runtime.GetElementGenerator(element.Kind)
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
			Ptr  ElementPtr      `json:"ptr"`
			Deps []ElementPtr    `json:"deps"`
			Kind ElementKind     `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"elements"`
	}

	raw := new(ManifestJSON)
	if err = json.Unmarshal(data, raw); err != nil {
		return err
	}

	manifest.Syntax = raw.Syntax
	manifest.Engine = raw.Engine

	if err = manifest.Header().validate(); err != nil {
		return err
	}

	runtime, _ := FetchEngineRuntime(manifest.Header().LogicEngine())

	manifest.Elements = make([]ManifestElement, 0, len(raw.Elements))

	for _, element := range raw.Elements {
		generator, ok := runtime.GetElementGenerator(element.Kind)
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

func (manifest *Manifest) UnmarshalYAML(node *yaml.Node) error {
	type ManifestYAML struct {
		Syntax   string             `yaml:"syntax"`
		Engine   ManifestEngineSpec `yaml:"engine"`
		Elements []struct {
			Ptr  ElementPtr   `yaml:"ptr"`
			Deps []ElementPtr `yaml:"deps"`
			Kind ElementKind  `yaml:"kind"`
			Data yaml.Node    `yaml:"data"`
		} `yaml:"elements"`
	}

	raw := new(ManifestYAML)
	if err := node.Decode(raw); err != nil {
		return err
	}

	manifest.Syntax = raw.Syntax
	manifest.Engine = raw.Engine

	if err := manifest.Header().validate(); err != nil {
		return err
	}

	runtime, _ := FetchEngineRuntime(manifest.Header().LogicEngine())

	manifest.Elements = make([]ManifestElement, 0, len(raw.Elements))

	for _, element := range raw.Elements {
		generator, ok := runtime.GetElementGenerator(element.Kind)
		if !ok {
			return errors.Errorf("unrecognized element kind: '%v'", element.Kind)
		}

		object := generator()
		if err := element.Data.Decode(object); err != nil {
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
