package toggler

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const SlotValue = 0

type InputSeed struct {
	Initial bool `polo:"initial"`
}

type InputSet struct {
	Value bool `polo:"value"`
}

func GetValue(storage engineio.StorageReader) (bool, error) {
	// Generate a storage access key for Value
	key := pisa.GenerateStorageKey(SlotValue)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return false, err
	}

	var value bool
	if err = polo.Depolorize(&value, val); err != nil {
		return false, err
	}

	return value, nil
}
