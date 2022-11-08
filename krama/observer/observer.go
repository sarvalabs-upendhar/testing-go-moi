package observer

import (
	"context"
	"time"

	ktypes "gitlab.com/sarvalabs/moichain/krama/types"
	"gitlab.com/sarvalabs/moichain/types"
	"gitlab.com/sarvalabs/polo/go-polo"
)

type WatchDog struct {
	ctx  context.Context
	slot *ktypes.Slot
	msgs []*types.ICSMSG
}

func NewWatchDog(ctx context.Context, slot *ktypes.Slot) *WatchDog {
	return &WatchDog{
		ctx:  ctx,
		slot: slot,
		msgs: make([]*types.ICSMSG, 0, slot.CLusterInfo().ICS.Size*2),
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

	watchDogProofs := ktypes.WatchDogProofs{
		MetaData: metaData,
		Extra:    nil, // TODO: Capture signature
	}

	return polo.Polorize(watchDogProofs)
}
