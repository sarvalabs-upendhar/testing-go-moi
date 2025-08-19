package test

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type debugStateDriver struct {
	id identifiers.Identifier

	logicstate map[[32]byte]map[string][]byte
}

func newDebugStateDriver(t *testing.T, id identifiers.Identifier) *debugStateDriver {
	t.Helper()

	return &debugStateDriver{
		id:         id,
		logicstate: make(map[[32]byte]map[string][]byte),
	}
}

func (state *debugStateDriver) Identifier() [32]byte                       { return state.id }
func (state *debugStateDriver) Root() [32]byte                             { return state.id }
func (state *debugStateDriver) LogicState() map[[32]byte]map[string][]byte { return state.logicstate }

func (state *debugStateDriver) ReadPersistentStorage(logicID [32]byte, key [32]byte) ([]byte, error) {
	if state.logicstate[logicID] == nil {
		return nil, common.ErrKeyNotFound
	}

	val, ok := state.logicstate[logicID][hex.EncodeToString(key[:])]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

func (state *debugStateDriver) WritePersistentStorage(logicID, key [32]byte, val []byte) error {
	if state.logicstate[logicID] == nil {
		state.logicstate[logicID] = make(map[string][]byte)
	}

	state.logicstate[logicID][hex.EncodeToString(key[:])] = val

	return nil
}

func (state *debugStateDriver) WriteTransientStorage(logicID, key [32]byte, val []byte) error {
	return errors.New("transient storage not supported in debug state driver")
}

func (state *debugStateDriver) ReadTransientStorage(logicID [32]byte, key [32]byte) ([]byte, error) {
	return nil, errors.New("transient storage not supported in debug state driver")
}

func (state *debugStateDriver) DeleteTransientStorage(logicID, key [32]byte) (uint64, error) {
	return 0, errors.New("transient storage not supported in debug state driver")
}

func (state *debugStateDriver) DeletePersistentStorage(logicID, key [32]byte) (uint64, error) {
	if state.logicstate[logicID] == nil {
		return 0, common.ErrKeyNotFound
	}

	val, ok := state.logicstate[logicID][hex.EncodeToString(key[:])]
	if !ok {
		return 0, common.ErrKeyNotFound
	}

	delete(state.logicstate[logicID], hex.EncodeToString(key[:]))

	return uint64(len(val)), nil
}

func (state *debugStateDriver) Copy() *debugStateDriver {
	clone := &debugStateDriver{
		id:         state.id,
		logicstate: make(map[[32]byte]map[string][]byte),
	}

	for key, val := range state.logicstate {
		clone.logicstate[key] = make(map[string][]byte)
		for k, v := range val {
			clone.logicstate[key][k] = v
		}
	}

	return clone
}

func (state *debugStateDriver) Reset(snap *debugStateDriver) {
	state.logicstate = make(map[[32]byte]map[string][]byte, len(snap.logicstate))

	for key, val := range snap.logicstate {
		state.logicstate[key] = make(map[string][]byte, len(val))
		for k, v := range val {
			state.logicstate[key][k] = v
		}
	}
}

type debugTxnDriver struct {
	kind     common.IxOpType
	hash     common.Hash
	price    *big.Int
	limit    uint64
	callsite string
	calldata []byte
	origin   [32]byte
	access   map[[32]byte]bool
}

func newDebugTxnDriver(
	t *testing.T, kind common.IxOpType,
	origin [32]byte,
	hash common.Hash,
	callsite string, calldata []byte,
	limit uint64, price *big.Int,
	access map[[32]byte]bool,
) debugTxnDriver {
	t.Helper()

	return debugTxnDriver{
		origin:   origin,
		kind:     kind,
		hash:     hash,
		price:    price,
		limit:    limit,
		callsite: callsite,
		calldata: calldata,
		access:   access,
	}
}

func (txn debugTxnDriver) Calldata() polo.Document {
	doc := make(polo.Document)

	if len(txn.calldata) == 0 {
		return doc
	}

	if err := polo.Depolorize(&doc, txn.calldata); err != nil {
		panic(fmt.Sprintf("failed to get logic payload %s", err))
	}

	return doc
}

func (txn debugTxnDriver) Timestamp() uint64 {
	return 0
}

func (txn debugTxnDriver) Origin() [32]byte {
	return txn.origin
}

func (txn debugTxnDriver) Parameters() map[string][]byte {
	// TODO implement me
	return nil
}

func (txn debugTxnDriver) Type() common.IxOpType         { return txn.kind }
func (txn debugTxnDriver) Hash() common.Hash             { return txn.hash }
func (txn debugTxnDriver) FuelPrice() *big.Int           { return txn.price }
func (txn debugTxnDriver) FuelLimit() uint64             { return txn.limit }
func (txn debugTxnDriver) Callsite() string              { return txn.callsite }
func (txn debugTxnDriver) Identifier() [32]byte          { return txn.hash }
func (txn debugTxnDriver) AccessList() map[[32]byte]bool { return txn.access }
func (txn debugTxnDriver) Access(id [32]byte) (bool, error) {
	if txn.access == nil {
		return true, nil
	}

	if _, ok := txn.access[id]; !ok {
		return false, errors.New("actor not found")
	}

	return txn.access[id], nil
}
