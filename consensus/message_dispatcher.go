package consensus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

// sendPrepareMsg broadcasts the ICS request to all the nodes that are part of the ICS
func (k *Engine) sendPrepareMsg(
	ctx context.Context,
	clusterID common.ClusterID,
	msg *types.Prepare,
	nodeset *types.ICSCommittee,
) (int, error) {
	var (
		requestTS      = time.Now()
		failedReqCount = new(atomic.Int32)
		waitGroup      = new(sync.WaitGroup)
		requestedNodes = make(map[id.KramaID]struct{})
	)

	defer k.metrics.captureRequestTurnaroundTime(requestTS)

	payload, err := msg.Bytes()
	if err != nil {
		return 0, err
	}

	// Construct the prepared message here to avoid sending the prepare message to the local node (self)
	preparedMsg, err := k.createPreparedMsg(msg)
	if err != nil {
		return 0, err
	}

	var (
		mtx              sync.Mutex
		unRespondedNodes = make([]id.KramaID, 0)
	)

	for icsSetType, ns := range nodeset.Sets {
		if ns == nil {
			continue
		}

		for index, info := range ns.Infos {
			if k.selfID == info.ID {
				ns.UpdateViewInfo(index, preparedMsg)
				ns.UpdateResponse(index, true)

				continue
			}

			if _, ok := requestedNodes[info.ID]; ok {
				continue
			}

			requestedNodes[info.ID] = struct{}{}

			waitGroup.Add(1)

			go func(kramaID id.KramaID, icsSetType int) {
				defer waitGroup.Done()

				k.logger.Trace("sending prepare msg", "cluster-id", clusterID, "to", kramaID)

				if err = k.transport.ConnectToDirectPeer(ctx, kramaID, clusterID); err != nil {
					failedReqCount.Add(1)

					k.logger.Error("failed to connect", "krama-id", kramaID, "err", err)

					mtx.Lock()
					unRespondedNodes = append(unRespondedNodes, kramaID)
					mtx.Unlock()

					return
				}

				err = k.transport.SendMessage(
					ctx,
					kramaID,
					types.NewICSMsg(
						k.selfID,
						clusterID,
						networkmsg.PREPARE,
						payload,
					))
				if err != nil {
					failedReqCount.Add(1)

					k.logger.Error("failed to send ics message", "krama-id", kramaID, "err", err)
				}
			}(info.ID, icsSetType)
		}
	}

	waitGroup.Wait()

	if len(unRespondedNodes) > 0 {
		k.randomizer.DeletePeers(unRespondedNodes)
	}

	return int(failedReqCount.Load()), nil
}
