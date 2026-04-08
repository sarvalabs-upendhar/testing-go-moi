package engineio

type (
	// ElementKind is a type alias for an element kind string
	ElementKind = string
	// ElementPtr is a type alias for an element pointer
	ElementPtr = uint64
)

// LogicDescriptor is a container type returned by the CompileManifest method of EngineRuntime.
// It allows different engine runtime to have a unified output standard when compiling manifests.
//
// It serves as a source of information from which an object that implements the LogicDriver
// interface can be generated. It contains within it the manifest's runtime engine, raw contents
// and hash apart from entries for the callsites and classdefs.
type LogicDescriptor struct {
	Engine EngineKind

	ManifestData []byte
	ManifestHash [32]byte

	Artifact []byte

	DeployerCallsite map[string]struct{}
}

// LogicElement represents a generic container for a logic Element.
// It is uniquely identified with a group name and an index pointer.
// Engine implementations are responsible for handling
// namespacing and index conflicts within a group.
type LogicElement struct {
	// Kind represents some type identifier for the element
	Kind ElementKind
	// Deps represents the relational neighbours of the element
	Deps []ElementPtr
	// Data represents the data container for the element
	Data []byte
}
