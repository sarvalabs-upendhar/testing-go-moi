package krama

import (
	"errors"
	"time"

	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/types"
)

func (k *Engine) minter() {
	for {
		interactionQueue := k.pool.Executables()

		for interactionQueue.Len() > 0 && k.slots.AvailableOperatorSlots() > 0 {
			ix, ok := interactionQueue.Pop().(*types.Interaction)
			if !ok {
				k.logger.Error("Error interaction type assertion failed", "ix-hash", ix.Hash())

				continue
			}

			go func() {
				k.logger.Debug("Forwarding interaction to krama engine", "ix-hash", ix.Hash())

				respChan := make(chan Response)
				ixs := types.Interactions{ix}
				k.requests <- Request{
					slotType:     ktypes.OperatorSlot,
					operator:     k.selfID,
					ixs:          ixs,
					msg:          nil,
					responseChan: respChan,
				}
				// Wait for response from krama engine handler
				resp := <-respChan
				if resp.err != nil {
					switch resp.err.Error() {
					case types.ErrInvalidInteractions.Error():
						k.pool.Drop(ix)
					default:
						if !errors.Is(resp.err, types.ErrSlotsFull) {
							if err := k.pool.IncrementWaitTime(ix.Sender(), k.avgICSTime); err != nil {
								k.logger.Error("Error incrementing wait time", "err", err)
							}

							return
						}

						k.logger.Error("ICS creation failed", "err", resp.err)
					}
				}
			}()

			time.Sleep(2 * time.Millisecond)
		}

		select {
		case <-k.ctx.Done():
			return
		case <-time.After(1000 * time.Millisecond):
		}
	}
}
