package common

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"
)

type Signature struct {
	ID        identifiers.Identifier
	KeyID     uint64
	Signature []byte
}

type Signatures []Signature

// Bytes serializes signatures to bytes.
func (s Signatures) Bytes() ([]byte, error) {
	data, err := polo.Polorize(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize signatures payload")
	}

	return data, nil
}

// FromBytes deserializes signatures from bytes.
func (s *Signatures) FromBytes(data []byte) error {
	if err := polo.Depolorize(s, data); err != nil {
		return errors.Wrap(err, "failed to depolorize signatures payload")
	}

	return nil
}
