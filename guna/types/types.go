package types

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/go-polo"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

const (
	MaxBehaviourContextSize = 8
	MaxRandomContextSize    = 7
)

type Context interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type ContextObject struct {
	Ids []id.KramaID
}

func (c *ContextObject) AddNodes(nodes []id.KramaID, maxSize int) {
	c.Ids = append(c.Ids, nodes...)
	if diff := len(c.Ids) - maxSize; diff > 0 {
		c.Ids = c.Ids[diff:]
	}
}

func (c *ContextObject) Copy() *ContextObject {
	newSlice := make([]id.KramaID, len(c.Ids))

	copy(newSlice, c.Ids)

	newObject := new(ContextObject)
	newObject.Ids = newSlice

	return newObject
}

func (c *ContextObject) Hash() (types.Hash, error) {
	hash, err := types.PoloHash(c)
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to polorize context object")
	}

	return hash, nil
}

func (c *ContextObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize context object")
	}

	return rawData, nil
}

func (c *ContextObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(c, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize context object")
	}

	return nil
}

type MetaContextObject struct {
	BehaviouralContext types.Hash
	RandomContext      types.Hash
	StorageContext     types.Hash
	ComputeContext     types.Hash
	DefaultMTQ         int32
	PreviousHash       types.Hash
}

func (m *MetaContextObject) Copy() *MetaContextObject {
	newObject := new(MetaContextObject)
	newObject.BehaviouralContext = m.BehaviouralContext
	newObject.RandomContext = m.RandomContext
	newObject.ComputeContext = m.ComputeContext
	newObject.DefaultMTQ = m.DefaultMTQ

	return newObject
}

func (m *MetaContextObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize meta context object")
	}

	return rawData, nil
}

func (m *MetaContextObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(m, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize meta context object")
	}

	return nil
}

func (m *MetaContextObject) Hash() (types.Hash, error) {
	hash, err := types.PoloHash(m)
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to polorize meta context object")
	}

	return hash, nil
}

type AccountSetupArgs struct {
	Address            types.Address
	AccType            types.AccountType
	MoiID              string
	BehaviouralContext []id.KramaID
	RandomContext      []id.KramaID
	Assets             []*types.AssetDescriptor
	Balances           map[types.AssetID]*big.Int
}

func (as *AccountSetupArgs) ContextDelta() types.ContextDelta {
	return map[types.Address]*types.DeltaGroup{
		as.Address: {
			BehaviouralNodes: as.BehaviouralContext,
			RandomNodes:      as.RandomContext,
		},
	}
}
