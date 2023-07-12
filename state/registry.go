package state

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type RegistryObject struct {
	Entries map[string][]byte
}

func (r *RegistryObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize registry object")
	}

	return rawData, nil
}

func (r *RegistryObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(r, bytes); err != nil {
		return errors.Wrap(err, "failed to polorize registry object")
	}

	return nil
}

func (r *RegistryObject) Copy() *RegistryObject {
	newObject := &RegistryObject{
		Entries: make(map[string][]byte, len(r.Entries)),
	}

	for k, v := range r.Entries {
		newObject.Entries[k] = make([]byte, len(v))
		copy(newObject.Entries[k], v)
	}

	return r
}
