package lockledger

import (
	"math/big"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const (
	EphemeralSlotSpendable = 0
	EphemeralSlotLockedup  = 1

	PersistentSlotName   = 0
	PersistentSlotSymbol = 1
	PersistentSlotSupply = 2
	PersistentSlotOwner  = 3
)

type InputSeed struct {
	Name   string `polo:"name"`
	Symbol string `polo:"symbol"`
	Supply uint64 `polo:"supply"`
}

type InputLockup struct {
	Amount uint64 `polo:"amount"`
}

type InputMint struct {
	Amount uint64 `polo:"amount"`
}

func GetEphemeralSpendable(storage engineio.StorageReader) (uint64, error) {
	// Generate a storage access key
	key := pisa.GenerateStorageKey(EphemeralSlotSpendable)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return 0, err
	}

	var value uint64
	if err = polo.Depolorize(&value, val); err != nil {
		return 0, err
	}

	return value, nil
}

func GetEphemeralLockedup(storage engineio.StorageReader) (uint64, error) {
	// Generate a storage access key
	key := pisa.GenerateStorageKey(EphemeralSlotLockedup)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return 0, err
	}

	var value uint64
	if err = polo.Depolorize(&value, val); err != nil {
		return 0, err
	}

	return value, nil
}

func GetPersistentSymbol(storage engineio.StorageReader) (string, error) {
	// Generate a storage access key for Symbol
	key := pisa.GenerateStorageKey(PersistentSlotSymbol)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return "", err
	}

	var symbol string
	if err = polo.Depolorize(&symbol, val); err != nil {
		return "", err
	}

	return symbol, nil
}

func GetPersistentSupply(storage engineio.StorageReader) (*big.Int, error) {
	// Generate a storage access key for Symbol
	key := pisa.GenerateStorageKey(PersistentSlotSupply)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return nil, err
	}

	supply := new(big.Int)
	if err = polo.Depolorize(&supply, val); err != nil {
		return nil, err
	}

	return supply, nil
}
