package tree

import (
	db "github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/types"
)

type mockDB struct {
	data map[string][]byte
}

func NewMockDB() *mockDB {
	return &mockDB{
		data: make(map[string][]byte),
	}
}

func (m *mockDB) SetMerkleTreeEntry(address types.Address, prefix db.Prefix, key, value []byte) error {
	dbKey := append(append(address.Bytes(), prefix.Byte()), key...)

	m.data[string(dbKey)] = value

	return nil
}

func (m *mockDB) GetMerkleTreeEntry(address types.Address, prefix db.Prefix, key []byte) ([]byte, error) {
	dbKey := append(append(address.Bytes(), prefix.Byte()), key...)

	value, ok := m.data[string(dbKey)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return value, nil
}

func (m *mockDB) SetMerkleTreeEntries(address types.Address, prefix db.Prefix, entries map[string][]byte) error {
	for k, v := range entries {
		dbKey := append(append(address.Bytes(), prefix.Byte()), []byte(k)...)

		m.data[string(dbKey)] = v
	}

	return nil
}

func (m *mockDB) WritePreImages(address types.Address, entries map[types.Hash][]byte) error {
	for k, v := range entries {
		dbKey := db.PreImageKey(address, k)

		m.data[string(dbKey)] = v
	}

	return nil
}

func (m *mockDB) GetPreImage(address types.Address, hash types.Hash) ([]byte, error) {
	dbKey := db.PreImageKey(address, hash)

	value, ok := m.data[string(dbKey)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return value, nil
}
