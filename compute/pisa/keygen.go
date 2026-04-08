package pisa

import (
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-pisa/storage"
	"github.com/sarvalabs/go-polo"
)

type (
	ArrIdx = storage.ArrayIndex
	MapKey = storage.MapKey
	ClsFld = storage.ClassField
)

func KeySlice(key [32]byte) []byte {
	return key[:]
}

func GenerateStorageKey(slot uint8, accessors ...storage.KeyLayer) [32]byte {
	key := storage.NewKey(slot, accessors...)
	key32 := key.Bytes32()

	return key32
}

func MakeMapKey(object any) MapKey {
	serial := must(polo.Polorize(object))
	hashed := blake2b.Sum256(serial)

	return hashed
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}
