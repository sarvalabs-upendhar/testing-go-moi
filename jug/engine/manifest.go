package engine

import "github.com/sarvalabs/moichain/types"

// ManifestConfig represents a configuration object for Manifests.
// It is generated from the Manifest when being compiled into a LogicObject and
// contains information required to regenerate the Manifest back from the LogicObject.
type ManifestConfig struct {
	Sum types.Hash
}

// ManifestHeader represents a simple header for a Manifest and describes its syntax form
// and engine mode. Useful for determining which engine to use to handle the Manifest.
// Every engine's manifest implementation must be able to decode into this header.
type ManifestHeader struct {
	Syntax string `polo:"syntax" yaml:"syntax" json:"syntax"`
	Engine string `polo:"engine" yaml:"engine" json:"engine"`
}
