package consensus

func (k *Engine) minter() {
	/*	for {
		interactionQueue := k.pool.Executables()

		for interactionQueue.Len() > 0 && k.slots.AvailableOperatorSlots() > 0 {
			ixn, ok := interactionQueue.Pop().(*common.Interaction)
			if !ok {
				k.logger.Error("Error interaction type assertion failed", "ix-hash", ixn.Hash())

				continue
			}

			go func(ix *common.Interaction) {
				k.logger.Debug("Forwarding interaction to krama engine", "ix-hash", ix.Hash())

				respChan := make(chan ktypes.Response)
				ixs := common.Interactions{ix}

				select {
				case <-k.ctx.Done():
				case k.requests <- ktypes.Request{
					Ctx:          k.ctx,
					SlotType:     ktypes.OperatorSlot,
					Operator:     k.selfID,
					Ixs:          ixs,
					Msg:          nil,
					ResponseChan: respChan,
				}:
				}

				select {
				case <-k.ctx.Done():
				case resp := <-respChan:
					if resp.Err == nil {
						return
					}

					switch resp.Err.Error() {
					case common.ErrInvalidInteractions.Error():
						k.pool.Drop(ix)
					default:
						if !errors.Is(resp.Err, common.ErrSlotsFull) {
							if err := k.pool.IncrementWaitTime(ix.Sender(), k.avgICSTime); err != nil {
								k.logger.Error("Error incrementing wait time", "err", err)
							}

							k.logger.Error("ICS creation failed", "error", resp.Err)

							return
						}
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
	}*/
}
