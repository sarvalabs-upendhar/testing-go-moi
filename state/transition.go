package state

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type ObjectRef interface {
	Identifier() identifiers.Identifier
	ContextHash() common.Hash
	Copy() ObjectRef
	IncrementSequenceID(keyID uint64) error
	InheritedAccount() identifiers.Identifier
	IsGenesis() bool
	AccountType() common.AccountType
	HasSufficientFuel(amount *big.Int) (bool, error)
	DeductFuel(amount *big.Int)
	ConsensusNodesHash() common.Hash
	AccType() common.AccountType
	flush() error
}

type Transition struct {
	objects          ObjectMap
	systemObject     *SystemObject
	auxiliaryObjects ObjectMap
	receipts         common.Receipts
}

func NewTransition(sysObject *SystemObject, objects ObjectMap, auxiliaryObjects ObjectMap) *Transition {
	return &Transition{
		systemObject:     sysObject,
		objects:          objects,
		auxiliaryObjects: auxiliaryObjects,
		receipts:         make(common.Receipts),
	}
}

func (t *Transition) ObjectsCount() int {
	count := len(t.objects)
	if t.systemObject != nil {
		count++
	}

	return count
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

	if t.systemObject != nil {
		snap.systemObject = t.systemObject.Copy()
	}

	return snap
}

func (t *Transition) UpdateSnapshot(snap *Transition) {
	t.objects = snap.objects
	t.systemObject = snap.systemObject
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

func (t *Transition) GetSystemObject() *SystemObject {
	return t.systemObject
}

func (t *Transition) GetAuxiliaryObject(id identifiers.Identifier) *Object {
	return t.auxiliaryObjects[id]
}

func (t *Transition) IncrementSequenceID(id identifiers.Identifier, keyID uint64) {
	_ = t.objects[id].IncrementSequenceID(keyID)
}

func (t *Transition) Flush(id identifiers.Identifier) error {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.flush()
	}

	return t.objects[id].flush()
}

func (t *Transition) Commit() (common.AccountStateHashes, error) {
	commitHashes := make(common.AccountStateHashes, len(t.objects))

	for _, stateObject := range t.objects {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return nil, err
		}

		commitHashes[stateObject.Identifier()] = &common.StateAndContextHash{
			StateHash:   stateHash,
			ContextHash: stateObject.ContextHash(),
		}
	}

	if t.systemObject != nil {
		stateHash, err := t.systemObject.Commit()
		if err != nil {
			return nil, err
		}

		commitHashes[t.systemObject.Identifier()] = &common.StateAndContextHash{
			StateHash:   stateHash,
			ContextHash: t.systemObject.ContextHash(),
		}
	}

	return commitHashes, nil
}

/*
func (t *Transition) CreateAsset(
	id identifiers.Identifier,
	assetID identifiers.Identifier,
	descriptor *common.AssetDescriptor,
) (identifiers.AssetID, error) {
	return t.objects[id].CreateAsset(assetID, descriptor)
}

func (t *Transition) CreateContext(
	id identifiers.Identifier,
	consensusNodes []identifiers.KramaID,
) error {
	return t.objects[id].CreateContext(consensusNodes)
}

func (t *Transition) UpdateContext(
	id identifiers.Identifier,
	consensusNodes []identifiers.KramaID,
) error {
	return t.objects[id].UpdateContext(consensusNodes)
}
*/

func (t *Transition) DeductFuel(id identifiers.Identifier, amount *big.Int) {
	t.objects[id].DeductFuel(amount)
}

func (t *Transition) HasSufficientFuel(id identifiers.Identifier, amount *big.Int) (bool, error) {
	return t.objects[id].HasSufficientFuel(amount)
}

func (t *Transition) GetAccTypeUsingStateObject(id identifiers.Identifier) common.AccountType {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.AccType()
	}

	return t.objects[id].AccType()
}

func (t *Transition) GetConsensusNodes(id identifiers.Identifier) []identifiers.KramaID {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.ConsensusNodes()
	}

	return t.GetObject(id).ConsensusNodes()
}

func (t *Transition) ConsensusNodesHash(id identifiers.Identifier) common.Hash {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.ConsensusNodesHash()
	}

	return t.objects[id].ConsensusNodesHash()
}

func (t *Transition) ContextHash(id identifiers.Identifier) common.Hash {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.ContextHash()
	}

	return t.objects[id].ContextHash()
}

func (t *Transition) InheritedAccount(id identifiers.Identifier) identifiers.Identifier {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.InheritedAccount()
	}

	return t.objects[id].InheritedAccount()
}

func (t *Transition) IsGenesis(id identifiers.Identifier) bool {
	if t.systemObject != nil && t.systemObject.id == id {
		return t.systemObject.IsGenesis()
	}

	return t.objects[id].IsGenesis()
}
