package db

import (
	"bytes"
	"encoding/binary"

	"github.com/sarvalabs/go-moi/common/identifiers"
)

var (
	TagDelim    = []byte("-")
	TagEnviron  = []byte("environ")
	TagManifest = []byte("manifest")
	TagStorage  = []byte("storage")
	TagEvents   = []byte("events")
	TagHead     = []byte("head")
	TagSize     = []byte("size")
)

// EnvironmentKey returns a key to the environment object
func EnvironmentKey(env string) []byte {
	// environ-{env}
	return bytes.Join([][]byte{TagEnviron, []byte(env)}, TagDelim)
}

// EnvironmentPrefix returns a key prefix for all items in an environment
func EnvironmentPrefix(env string) []byte {
	// {env}
	return []byte(env)
}

func AccountKey(env string, id identifiers.Identifier) []byte {
	// {env}-{id}
	return bytes.Join([][]byte{[]byte(env), id.Bytes()}, TagDelim)
}

// AccountPrefix returns a key prefix for all items under an id
func AccountPrefix(env string, id identifiers.Identifier) []byte {
	// {env}-{id}
	return bytes.Join([][]byte{[]byte(env), id.Bytes()}, TagDelim)
}

// LogicAccountKey returns a key to the entity item of a Logic
// It is the same as the prefix key for all items in the logic's id
func LogicAccountKey(env string, logic identifiers.LogicID) []byte {
	// {env}-{id}
	return AccountPrefix(env, logic.AsIdentifier())
}

// LogicManifestKey returns a key for the manifest of a logic
func LogicManifestKey(env string, logic identifiers.LogicID) []byte {
	// {env}-{id}-manifest
	return bytes.Join([][]byte{[]byte(env), logic.Bytes(), TagManifest}, TagDelim)
}

// StoragePrefix returns a key prefix for all storage keys for a given id
func StoragePrefix(env string, id identifiers.Identifier) []byte {
	// {env}-{id}-storage
	return bytes.Join([][]byte{[]byte(env), id.Bytes(), TagStorage}, TagDelim)
}

// LogicStoragePrefix returns a key prefix for all storage keys of a particular logic for a given id
func LogicStoragePrefix(env string, id identifiers.Identifier, logic identifiers.LogicID) []byte {
	// {env}-{id}-storage-{logic}
	return bytes.Join([][]byte{[]byte(env), id.Bytes(), TagStorage, logic.Bytes()}, TagDelim)
}

// StorageKey returns a key for a specific storage key of a logic in a given id
func StorageKey(env string, id identifiers.Identifier, logic identifiers.LogicID, key []byte) []byte {
	// {env}-{id}-storage-{logic}-{key}
	return bytes.Join([][]byte{[]byte(env), id.Bytes(), TagStorage, logic.Bytes(), key}, TagDelim)
}

// EventKey returns a key for a specific index
func EventKey(env string, index uint64) []byte {
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, index)

	// {env}-events-{index}
	return bytes.Join([][]byte{[]byte(env), TagEvents, value}, TagDelim)
}

// EventHeadKey returns the key for the head index
func EventHeadKey(env string) []byte {
	// {env}-events-head
	return bytes.Join([][]byte{[]byte(env), TagEvents, TagHead}, TagDelim)
}

// EventSizeKey returns the key for the size
func EventSizeKey(env string) []byte {
	// {env}-events-size
	return bytes.Join([][]byte{[]byte(env), TagEvents, TagSize}, TagDelim)
}

// AssetEntity: {env}-{id} ?
// SpendableBalance: {env}-{id}-balance-spendable-{asset}
// ApprovalBalance: {env}-{id}-balance-approval-{asset}-{spender}
// LockupBalance: {env}-{id}-balance-lockup-{asset}-{logic}
