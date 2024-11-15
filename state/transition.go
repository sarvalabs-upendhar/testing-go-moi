package state

import (
	"math/big"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
)

type Transition struct {
	objects  ObjectMap
	receipts common.Receipts
}

func NewTransition(objects ObjectMap) *Transition {
	return &Transition{
		objects:  objects,
		receipts: make(common.Receipts),
	}
}

func (t *Transition) Snapshot() *Transition {
	snap := &Transition{
		objects:  make(ObjectMap),
		receipts: make(common.Receipts),
	}

	if len(t.objects) > 0 {
		for addr, object := range t.objects {
			snap.objects[addr] = object.Copy()
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

func (t *Transition) GetObject(addr identifiers.Address) *Object {
	return t.objects[addr]
}

func (t *Transition) Delete(addr identifiers.Address) {
	delete(t.objects, addr)
}

func (t *Transition) IncrementNonce(addr identifiers.Address, count uint64) {
	t.objects[addr].IncrementNonce(count)
}

func (t *Transition) Flush(addr identifiers.Address) error {
	return t.objects[addr].flush()
}

func (t *Transition) CreateAsset(
	addr identifiers.Address,
	assetAddr identifiers.Address,
	descriptor *common.AssetDescriptor,
) (identifiers.AssetID, error) {
	return t.objects[addr].CreateAsset(assetAddr, descriptor)
}

func (t *Transition) CreateContext(
	addr identifiers.Address,
	behaviouralNodes,
	randomNodes []kramaid.KramaID,
) (common.Hash, error) {
	return t.objects[addr].CreateContext(behaviouralNodes, randomNodes)
}

func (t *Transition) UpdateContext(
	addr identifiers.Address,
	behaviouralNodes,
	randomNodes []kramaid.KramaID,
) (common.Hash, error) {
	return t.objects[addr].UpdateContext(behaviouralNodes, randomNodes)
}

func (t *Transition) DeductFuel(addr identifiers.Address, amount *big.Int) {
	t.objects[addr].DeductFuel(amount)
}

func (t *Transition) HasSufficientFuel(addr identifiers.Address, amount *big.Int) (bool, error) {
	return t.objects[addr].HasSufficientFuel(amount)
}

func (t *Transition) GetAccTypeUsingStateObject(address identifiers.Address) common.AccountType {
	return t.objects[address].accType
}

func (t *Transition) IsGenesis(addr identifiers.Address) bool {
	return t.objects[addr].isGenesis
}
