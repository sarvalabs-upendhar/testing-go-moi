package pisa

import (
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-pisa/state"
	"github.com/sarvalabs/go-polo"
)

type (
	Accessor = state.Accessor
	ArrIdx   = state.ArrayIndex
	MapKey   = state.MapKey
	ClsFld   = state.ClassField
)

func GenerateStorageKey(slot uint8, accessors ...state.Accessor) []byte {
	key := state.GenerateStorageKey(slot, accessors...)
	key32 := key.Bytes32()

	return key32[:]
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
