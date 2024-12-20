package state

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// Deeds represents a collection of assetID's mapped to empty values to indicate their existence.
type Deeds struct {
	Entries map[string]struct{}
}

// Bytes serializes Deeds to a byte slice.
func (d *Deeds) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(d)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize deeds object")
	}

	return rawData, nil
}

// FromBytes deserializes a byte slice into Deeds.
func (d *Deeds) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(d, bytes); err != nil {
		return errors.Wrap(err, "failed to polorize deeds object")
	}

	return nil
}

// Copy returns a deep copy of the Deeds object.
func (d *Deeds) Copy() *Deeds {
	newObject := &Deeds{
		Entries: make(map[string]struct{}, len(d.Entries)),
	}

	for k := range d.Entries {
		newObject.Entries[k] = struct{}{}
	}

	return newObject
}
