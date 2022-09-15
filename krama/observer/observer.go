package observer

import (
	"context"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/krama/types"
	"gitlab.com/sarvalabs/polo/go-polo"
	"time"
)

type WatchDog struct {
	ctx  context.Context
	slot *types.Slot
	msgs []*ktypes.ICSMSG
}

func NewWatchDog(ctx context.Context, slot *types.Slot) *WatchDog {
	return &WatchDog{
		ctx:  ctx,
		slot: slot,
		msgs: make([]*ktypes.ICSMSG, 0, slot.CLusterInfo().ICS.Size*2),
	}
}

func (wg *WatchDog) StartWatchDog() {
	timeCtx, cancel := context.WithTimeout(wg.ctx, 6*time.Second)
	defer cancel()

	for {
		select {
		case <-timeCtx.Done():
			return
		case msg, ok := <-wg.slot.BftInboundChan:
			if !ok {
				return
			}

			wg.msgs = append(wg.msgs, msg.ICSMsg(wg.slot.ClusterID()))
		}
	}
}

func (wg *WatchDog) GenerateProofs() []byte {
	metaData := wg.slot.CLusterInfo().GetMetaData(wg.msgs)

	watchDogProofs := types.WatchDogProofs{
		MetaData: metaData,
		Extra:    nil, //TODO: Capture signature
	}

	return polo.Polorize(watchDogProofs)
}
