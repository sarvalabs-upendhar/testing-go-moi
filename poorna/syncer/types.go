package syncer

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/dhruva"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

type AccDetailsQueue struct {
	queue []*types.AccountMetaInfo
	lock  sync.RWMutex
}

func (a *AccDetailsQueue) Push(data []*types.AccountMetaInfo) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.queue = append(a.queue, data...)
}

func (a *AccDetailsQueue) Pop() (*types.AccountMetaInfo, error) {
	if len(a.queue) > 0 {
		a.lock.Lock()
		defer a.lock.Unlock()

		data := a.queue[0]
		a.queue = a.queue[1:]

		return data, nil
	}

	return nil, errors.New("Queue is empty")
}

func (a *AccDetailsQueue) Len() int {
	a.lock.Lock()
	defer a.lock.Unlock()

	return len(a.queue)
}

type TesseractResponse struct {
	Data  []byte
	Delta map[types.Hash][]byte
}

func approvalsCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Approvals.Byte(), hash)
}

func accountCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Account.Byte(), hash)
}

func contextCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Context.Byte(), hash)
}

func storageCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Storage.Byte(), hash)
}

func logicCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Logic.Byte(), hash)
}

func balanceCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Balance.Byte(), hash)
}

func dbKeyFromCID(address types.Address, cid atypes.CID) []byte {
	return dhruva.DBKey(address, dhruva.Prefix(cid.ContentType()), cid.Key())
}
