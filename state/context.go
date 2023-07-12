package state

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/sarvalabs/go-moi/common"
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

func (c *ContextObject) Hash() (common.Hash, error) {
	hash, err := common.PoloHash(c)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to polorize context object")
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
	BehaviouralContext common.Hash
	RandomContext      common.Hash
	StorageContext     common.Hash
	ComputeContext     common.Hash
	DefaultMTQ         int32
	PreviousHash       common.Hash
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

func (m *MetaContextObject) Hash() (common.Hash, error) {
	hash, err := common.PoloHash(m)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to polorize meta context object")
	}

	return hash, nil
}
