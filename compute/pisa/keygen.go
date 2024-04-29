package pisa

import "github.com/sarvalabs/go-pisa/state"

type (
	ArrIdx = state.ArrayIndex
	MapKey = state.MapKey
	ClsFld = state.ClassField
)

func GenerateStorageKey(slot uint8, accessors ...state.Accessor) []byte {
	key := state.GenerateStorageKey(slot, accessors...)
	key32 := key.Bytes32()

	return key32[:]
}
