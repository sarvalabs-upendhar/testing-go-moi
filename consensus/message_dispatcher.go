package consensus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

// sendPrepareMsg broadcasts the ICS request to all the context nodes that are part of the ICS
func (k *Engine) sendPrepareMsg(
	ctx context.Context,
	clusterID common.ClusterID,
	msg *types.Prepare,
	nodeset *types.ICSCommittee,
	viewInfos []*common.ViewInfo,
) (int, error) {
	var (
		requestTS      = time.Now()
		failedReqCount = new(atomic.Int32)
		waitGroup      = new(sync.WaitGroup)
		requestedNodes = make(map[identifiers.KramaID]struct{})
	)

	defer k.metrics.captureRequestTurnaroundTime(requestTS)

	payload, err := msg.Bytes()
	if err != nil {
		return 0, err
	}

	// Construct the prepared message here to avoid sending the prepare message to the local node (self)
	preparedMsg, err := k.createPreparedMsg(msg, viewInfos)
	if err != nil {
		return 0, err
	}

	var (
		mtx              sync.Mutex
		unRespondedNodes = make([]identifiers.KramaID, 0)
	)

	for icsSetType, ns := range nodeset.ContextSet() {
		if ns == nil {
			continue
		}

		for index, info := range ns.Infos {
			if k.selfID == info.KramaID {
				ns.UpdateViewInfo(index, preparedMsg)
				ns.UpdateResponse(types.VoteCounter, index, true)

				continue
			}

			if _, ok := requestedNodes[info.KramaID]; ok {
				continue
			}

			requestedNodes[info.KramaID] = struct{}{}

			waitGroup.Add(1)

			go func(kramaID identifiers.KramaID, icsSetType int) {
				defer waitGroup.Done()

				k.logger.Trace("sending prepare msg", "cluster-id", clusterID, "to", kramaID)

				var err error

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
						false,
					))
				if err != nil {
					failedReqCount.Add(1)

					k.logger.Error("failed to send ics message", "krama-id", kramaID, "err", err)
				}
			}(info.KramaID, icsSetType)
		}
	}

	waitGroup.Wait()

	if len(unRespondedNodes) > 0 {
		k.randomizer.DeletePeers(unRespondedNodes)
	}

	return int(failedReqCount.Load()), nil
}
