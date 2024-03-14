package db

import (
	"bytes"

	"github.com/sarvalabs/go-moi-identifiers"
)

var (
	TagDelim    = []byte("-")
	TagEnviron  = []byte("environ")
	TagManifest = []byte("manifest")
	TagStorage  = []byte("storage")
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

// AddressPrefix returns a key prefix for all items under an address
func AddressPrefix(env string, addr identifiers.Address) []byte {
	// {env}-{addr}
	return bytes.Join([][]byte{[]byte(env), addr.Bytes()}, TagDelim)
}

// LogicEntityKey returns a key to the entity item of a Logic
// It is the same as the prefix key for all items in the logic's address
func LogicEntityKey(env string, logic identifiers.LogicID) []byte {
	// {env}-{addr}
	return AddressPrefix(env, logic.Address())
}

// LogicManifestKey returns a key for the manifest of a logic
func LogicManifestKey(env string, logic identifiers.LogicID) []byte {
	// {env}-{addr}-manifest
	return bytes.Join([][]byte{[]byte(env), logic.Address().Bytes(), TagManifest}, TagDelim)
}

// StoragePrefix returns a key prefix for all storage keys for a given address
func StoragePrefix(env string, addr identifiers.Address) []byte {
	// {env}-{addr}-storage
	return bytes.Join([][]byte{[]byte(env), addr.Bytes(), TagStorage}, TagDelim)
}

// LogicStoragePrefix returns a key prefix for all storage keys of a particular logic for a given address
func LogicStoragePrefix(env string, addr identifiers.Address, logic identifiers.LogicID) []byte {
	// {env}-{addr}-storage-{logic}
	return bytes.Join([][]byte{[]byte(env), addr.Bytes(), TagStorage, logic.Bytes()}, TagDelim)
}

// StorageKey returns a key for a specific storage key of a logic in a given address
func StorageKey(env string, addr identifiers.Address, logic identifiers.LogicID, key []byte) []byte {
	// {env}-{addr}-storage-{logic}-{key}
	return bytes.Join([][]byte{[]byte(env), addr.Bytes(), TagStorage, logic.Bytes(), key}, TagDelim)
}

// AssetEntity: {env}-{addr} ?
// SpendableBalance: {env}-{addr}-balance-spendable-{asset}
// ApprovalBalance: {env}-{addr}-balance-approval-{asset}-{spender}
// LockupBalance: {env}-{addr}-balance-lockup-{asset}-{logic}
