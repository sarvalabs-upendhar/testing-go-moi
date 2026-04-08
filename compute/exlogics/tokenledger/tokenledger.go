package tokenledger

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const (
	SlotSymbol   = 0
	SlotSupply   = 1
	SlotBalances = 2
)

type InputSeed struct {
	Symbol string `polo:"symbol"`
	Supply uint64 `polo:"supply"`
}

type InputTransfer struct {
	Amount   uint64   `polo:"amount"`
	Receiver [32]byte `polo:"receiver"`
}

func GetSymbol(storage engineio.StorageReader) (string, error) {
	// Generate a storage access key for Symbol
	key := pisa.GenerateStorageKey(SlotSymbol)

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

func GetSupply(storage engineio.StorageReader) (*big.Int, error) {
	// Generate a storage access key for Symbol
	key := pisa.GenerateStorageKey(SlotSupply)

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

func GetBalance(storage engineio.StorageReader, id identifiers.Identifier) (*big.Int, error) {
	// Generate a storage access key for Balance
	key := pisa.GenerateStorageKey(SlotBalances, pisa.MakeMapKey(id))

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return nil, err
	}

	balance := new(big.Int)
	if err = polo.Depolorize(&balance, val); err != nil {
		return nil, err
	}

	return balance, nil
}
