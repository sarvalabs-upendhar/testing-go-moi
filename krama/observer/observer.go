package observer

import (
	"context"
	"time"

	types "github.com/sarvalabs/moichain/krama/types"
)

type WatchDog struct {
	ctx  context.Context
	slot *types.Slot
	msgs []*types.ICSMSG
}

func NewWatchDog(ctx context.Context, slot *types.Slot) *WatchDog {
	return &WatchDog{
		ctx:  ctx,
		slot: slot,
		msgs: make([]*types.ICSMSG, 0, slot.ClusterInfo().ICS.Size*2),
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

			icsMsg, err := msg.ICSMsg(wg.slot.ClusterID())
			if err != nil {
				return
			}

			wg.msgs = append(wg.msgs, icsMsg)
		}
	}
}

func (wg *WatchDog) GenerateProofs() ([]byte, error) {
	metaData, err := wg.slot.ClusterInfo().GetMetaData(wg.msgs)
	if err != nil {
		return nil, err
	}

	watchDogProofs := types.WatchDogProofs{
		MetaData: metaData,
		Extra:    nil, // TODO: Capture signature
	}

	rawData, err := watchDogProofs.Bytes()
	if err != nil {
		return nil, err
	}

	return rawData, nil
}
