package syncer

import (
	"sync"

	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/types"
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
