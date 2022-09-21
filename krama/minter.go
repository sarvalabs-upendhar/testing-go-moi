package krama

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/krama/types"
	"time"
)

func (k *Engine) minter() {
	respChan := make(chan Response)

	for {
		if k.slots.AreSlotsAvailable(types.OperatorSlot) {
			interactionQueue := k.pool.Executables()

			for interactionQueue.Len() > 0 {
				ix, ok := interactionQueue.Pop().(*ktypes.Interaction)
				if !ok {
					k.logger.Error("Error interaction type assertion failed", "hash", ix.GetIxHash())

					continue
				}

				ixs := ktypes.Interactions{ix}

				k.logger.Info("Forwarding request to krama engine")

				k.requests <- Request{reqType: 0, ixs: ixs, msg: nil, responseChan: respChan}
				//Wait for response from krama engine handler
				resp := <-respChan
				if resp.err != nil {
					switch resp.err {
					case ktypes.ErrInvalidInteractions:
						k.pool.ResetWithInteractions(ixs)
					default:
						if resp.err != ktypes.ErrSlotsFull {
							if err := k.pool.IncrementWaitTime(ix.FromAddress()); err != nil {
								k.logger.Error("Error incrementing wait time")
							}
						} else {
							k.logger.Error("ICS creation failed", resp.err)
						}
					}
				}
			}
		}
		select {
		case <-k.ctx.Done():
			return
		case <-time.After(1000 * time.Millisecond):
		}
	}
}
