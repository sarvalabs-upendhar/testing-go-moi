package krama

import (
	"errors"
	"time"

	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/types"
)

func (k *Engine) minter() {
	respChan := make(chan Response)

	for {
		if k.slots.AreSlotsAvailable(ktypes.OperatorSlot) {
			interactionQueue := k.pool.Executables()

			for interactionQueue.Len() > 0 {
				ix, ok := interactionQueue.Pop().(*types.Interaction)
				if !ok {
					k.logger.Error("Error interaction type assertion failed", "hash", ix.Hash())

					continue
				}

				ixs := types.Interactions{ix}

				k.logger.Info("Forwarding request to krama engine")

				k.requests <- Request{slotType: ktypes.OperatorSlot, operator: k.selfID, ixs: ixs, msg: nil, responseChan: respChan}
				// Wait for response from krama engine handler
				resp := <-respChan
				if resp.err != nil {
					switch resp.err.Error() {
					case types.ErrInvalidInteractions.Error():
						k.pool.Drop(ix)
					default:
						if !errors.Is(resp.err, types.ErrSlotsFull) {
							if err := k.pool.IncrementWaitTime(ix.Sender(), k.avgICSTime); err != nil {
								k.logger.Error("Error incrementing wait time", "error", err)
							}

							continue
						}

						k.logger.Error("ICS creation failed", resp.err)
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
