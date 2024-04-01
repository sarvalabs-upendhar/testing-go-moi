package consensus

import (
	"errors"
	"time"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

func (k *Engine) minter() {
	for {
		interactionQueue := k.pool.Executables()

		for interactionQueue.Len() > 0 && k.slots.AvailableOperatorSlots() > 0 {
			ixn, ok := interactionQueue.Pop().(*common.Interaction)
			if !ok {
				k.logger.Error("Error interaction type assertion failed", "ix-hash", ixn.Hash())

				continue
			}

			go func(ix *common.Interaction) {
				k.logger.Debug("Forwarding interaction to krama engine", "ix-hash", ix.Hash())

				respChan := make(chan error)
				ixs := common.Interactions{ix}
				k.requests <- ktypes.Request{
					SlotType:     ktypes.OperatorSlot,
					Operator:     k.selfID,
					Ixs:          ixs,
					Msg:          nil,
					ResponseChan: respChan,
				}
				// Wait for response from krama engine handler
				if err := <-respChan; err != nil {
					switch err.Error() {
					case common.ErrInvalidInteractions.Error():
						k.pool.Drop(ix)
					default:
						if !errors.Is(err, common.ErrSlotsFull) {
							if err := k.pool.IncrementWaitTime(ix.Sender(), k.avgICSTime); err != nil {
								k.logger.Error("Error incrementing wait time", "err", err)
							}

							return
						}

						k.logger.Error("ICS creation failed", "err", err)
					}
				}
			}(ixn)

			time.Sleep(2 * time.Millisecond)
		}

		select {
		case <-k.ctx.Done():
			return
		case <-time.After(1000 * time.Millisecond):
		}
	}
}
