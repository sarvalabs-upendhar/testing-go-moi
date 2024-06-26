package engineio

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

// Manifest is the canonical deployment artifact for logics in MOI.
// It is a composite artifact that describes the bytecode, the binary interface (ABI) and
// other parameters for the runtime of choice. The spec for LogicManifests is available at
// https://sarvalabs.notion.site/Logic-Manifest-Standard-93f5fee1af8d4c3cad155b9827b97930?pvs=4
type Manifest interface {
	// Kind returns the kind of engine of the Manifest
	Kind() EngineKind
	// Hash returns the 256-bit digest of the Manifest.
	Hash() [32]byte
	// Size returns the number of elements in the Manifest
	Size() uint64

	// Syntax returns the syntax version of the Manifest
	Syntax() uint64
	// Engine returns the engine information of the Manifest as a ManifestEngine
	Engine() ManifestEngine
	// Header returns the header information of the Manifest as a ManifestHeader
	Header() ManifestHeader

	// Elements returns all the elements in the Manifest as an array of ManifestElement
	Elements() []ManifestElement
	// GetElement returns the ManifestElement from the Manifest with the given ElementPtr.
	// The boolean indicated is such an element exists in the Manifest.
	GetElement(ElementPtr) (ManifestElement, bool)

	// Encode returns the encoded bytes form of the Manifest for the specified encoding
	Encode(common.Encoding) ([]byte, error)
	// Decode decodes the given encoded bytes in the specified encoding into the Manifest
	Decode(common.Encoding, []byte) error
}

// NewManifest decodes the given raw data of the specified encoding type into a Manifest.
// Fails if the encoding is unsupported or if the data is malformed.
func NewManifest(data []byte, encoding common.Encoding) (Manifest, error) {
	header := new(manifestSyntaxHeader)
	// Decode the header based on the encoding format
	switch encoding {
	case common.POLO:
		if err := polo.Depolorize(header, data); err != nil {
			return nil, err
		}
	case common.JSON:
		if err := json.Unmarshal(data, header); err != nil {
			return nil, err
		}
	case common.YAML:
		if err := yaml.Unmarshal(data, header); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported encoding format for manifest")
	}

	var manifest Manifest
	// Decode the manifest based on the known manifest
	switch header.Syntax {
	case 1:
		manifest = new(manifestV1)
		if err := manifest.Decode(encoding, data); err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("unsupported syntax version for manifest")
	}

	return manifest, nil
}

// NewManifestFromFile reads a file at the specified filepath and decodes it into an engineio.Manifest.
// The encoding format of the file is determined from the file extension.
func NewManifestFromFile(path string) (Manifest, error) {
	path, _ = filepath.Abs(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.Errorf("manifest file not found @ '%v'", path)
	}

	var (
		extension string
		encoding  common.Encoding
	)

	switch extension = filepath.Ext(path); extension {
	case ".json":
		encoding = common.JSON
	case ".polo":
		encoding = common.POLO
	case ".yaml":
		encoding = common.YAML
	default:
		return nil, errors.Errorf("manifest file has unsupported extension: '%v'", extension)
	}

	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest file")
	}

	manifest, err := NewManifest(encoded, encoding)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode %v manifest data", extension)
	}

	return manifest, nil
}

type manifestSyntaxHeader struct {
	Syntax uint64 `yaml:"syntax" json:"syntax"`
}

// ManifestHeader represents the header for a Manifest and describes its syntax form
// and engine specification. Useful for determining which engine to use to handle the
// Manifest. Every engine's manifest implementation must be able to decode into this header.
type ManifestHeader struct {
	Syntax uint64         `yaml:"syntax" json:"syntax"`
	Engine ManifestEngine `yaml:"engine" json:"engine"`
}

// ManifestEngine describes the engine specification in the Manifest
type ManifestEngine struct {
	Kind  EngineKind `yaml:"kind" json:"kind"`
	Flags []string   `yaml:"flags" json:"flags"`
}

// ManifestElement describes a single element in the Manifest.
// It is converted into a LogicElement after compilation.
//
// Each element is of a particular type (described by the engine runtime) and is identified by
// a unique 64-bit pointer  and describes its dependencies with other elements in the manifest.
// The data of the manifest element must be resolved into the format specific for the runtime based on its
// kind. The raw object to decode into can be accessed with the GetElementGenerator method of EngineRuntime.
type ManifestElement struct {
	Ptr  ElementPtr
	Deps []ElementPtr
	Kind ElementKind
	Data any
}
