package state

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
)

type Transition struct {
	objects          ObjectMap
	auxiliaryObjects ObjectMap
	receipts         common.Receipts
}

func NewTransition(objects ObjectMap, auxiliaryObjects ObjectMap) *Transition {
	return &Transition{
		objects:          objects,
		auxiliaryObjects: auxiliaryObjects,
		receipts:         make(common.Receipts),
	}
}

func (t *Transition) Snapshot() *Transition {
	snap := &Transition{
		objects:  make(ObjectMap),
		receipts: make(common.Receipts),
	}

	if len(t.objects) > 0 {
		for id, object := range t.objects {
			snap.objects[id] = object.Copy()
		}
	}

	if len(t.receipts) > 0 {
		snap.receipts = t.receipts.Copy()
	}

	return snap
}

func (t *Transition) UpdateSnapshot(snap *Transition) {
	t.objects = snap.objects
	t.receipts = snap.receipts
}

func (t *Transition) Objects() ObjectMap {
	return t.objects
}

func (t *Transition) SetReceipt(ixHash common.Hash, receipt *common.Receipt) {
	t.receipts[ixHash] = receipt
}

func (t *Transition) Receipts() common.Receipts {
	return t.receipts
}

func (t *Transition) GetObject(id identifiers.Identifier) *Object {
	return t.objects[id]
}

func (t *Transition) GetAuxiliaryObject(id identifiers.Identifier) *Object {
	return t.auxiliaryObjects[id]
}

func (t *Transition) Delete(id identifiers.Identifier) {
	delete(t.objects, id)
}

func (t *Transition) IncrementSequenceID(id identifiers.Identifier, keyID uint64) {
	_ = t.objects[id].IncrementSequenceID(keyID)
}

func (t *Transition) Flush(id identifiers.Identifier) error {
	return t.objects[id].flush()
}

func (t *Transition) CreateAsset(
	id identifiers.Identifier,
	assetID identifiers.Identifier,
	descriptor *common.AssetDescriptor,
) (identifiers.AssetID, error) {
	return t.objects[id].CreateAsset(assetID, descriptor)
}

func (t *Transition) CreateContext(
	id identifiers.Identifier,
	consensusNodes []kramaid.KramaID,
) error {
	return t.objects[id].CreateContext(consensusNodes)
}

func (t *Transition) UpdateContext(
	id identifiers.Identifier,
	consensusNodes []kramaid.KramaID,
) error {
	return t.objects[id].UpdateContext(consensusNodes)
}

func (t *Transition) DeductFuel(id identifiers.Identifier, amount *big.Int) {
	t.objects[id].DeductFuel(amount)
}

func (t *Transition) HasSufficientFuel(id identifiers.Identifier, amount *big.Int) (bool, error) {
	return t.objects[id].HasSufficientFuel(amount)
}

func (t *Transition) GetAccTypeUsingStateObject(id identifiers.Identifier) common.AccountType {
	return t.objects[id].accType
}

func (t *Transition) ConsensusNodesHash(id identifiers.Identifier) common.Hash {
	if t.objects[id].metaContext == nil {
		return common.NilHash
	}

	return t.objects[id].metaContext.ConsensusNodesHash
}

func (t *Transition) ContextHash(id identifiers.Identifier) common.Hash {
	return t.objects[id].data.ContextHash
}

func (t *Transition) InheritedAccount(id identifiers.Identifier) identifiers.Identifier {
	if t.objects[id].metaContext == nil {
		return identifiers.Nil
	}

	return t.objects[id].metaContext.InheritedAccount
}

func (t *Transition) IsGenesis(id identifiers.Identifier) bool {
	return t.objects[id].isGenesis
}
