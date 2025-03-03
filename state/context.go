package state

import (
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type MetaContextObject struct {
	ConsensusNodes     []kramaid.KramaID
	SubAccounts        map[identifiers.Identifier]identifiers.Identifier
	InheritedAccount   identifiers.Identifier
	ConsensusNodesHash common.Hash
	StorageContext     common.Hash
	ComputeContext     common.Hash
	DefaultMTQ         int32
	PreviousHash       common.Hash
}

func (m *MetaContextObject) Copy() *MetaContextObject {
	newObject := new(MetaContextObject)
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
