package consensus

import (
	"context"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/telemetry/tracing"

	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

// initOutboundMessageHandler initializes and runs a loop to handle outbound messages.
func (k *Engine) initOutboundMessageHandler(ctx context.Context, slot *types.Slot) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-slot.BftOutboundChan:
			k.handleOutboundMessage(ctx, slot.ClusterID(), msg)
		}
	}
}

// handleOutboundMessage processes the outbound vote messages.
func (k *Engine) handleOutboundMessage(ctx context.Context, clusterID common.ClusterID, msg types.ConsensusMessage) {
	if voteMsg, ok := msg.Message.(*types.VoteMessage); ok {
		if err := k.sendVote(ctx, clusterID, voteMsg); err != nil {
			k.logger.Error("Failed to send vote msg", "error", err)
		}
	}
}

// sendICSRequest broadcasts the ICS request to all the nodes that are part of the ICS
func (k *Engine) sendICSRequest(
	ctx context.Context,
	canonicalReq types.CanonicalICSRequest,
	nodeset *common.ICSNodeSet,
) {
	requestTS := time.Now()

	spanCtx, span := tracing.Span(ctx, "Krama.KramaEngine", "sendICSRequest")
	defer span.End()

	defer k.metrics.captureRequestTurnaroundTime(requestTS)

	waitGroup := new(sync.WaitGroup)
	requestedNodes := make(map[id.KramaID]struct{})

	for icsSetType, ns := range nodeset.Nodes {
		if ns == nil {
			continue
		}

		canonicalReq.ContextType = int32(icsSetType)

		payload, err := k.signICSRequest(canonicalReq)
		if err != nil {
			k.logger.Error("Failed to sign canonical ics request", err)

			continue
		}

		for index, kramaID := range ns.Ids {
			if k.selfID == kramaID {
				ns.Responses.SetIndex(index, true)
				ns.UpdateRespCount()

				continue
			}

			if _, ok := requestedNodes[kramaID]; ok {
				continue
			}

			requestedNodes[kramaID] = struct{}{}

			waitGroup.Add(1)

			go func(kramaID id.KramaID, icsSetType int) {
				defer waitGroup.Done()
				k.logger.Trace("Sending ICS Request", "cluster-id", canonicalReq.ClusterID, "to", kramaID)
				err = k.transport.SendMessage(
					spanCtx,
					kramaID,
					k.selfID,
					canonicalReq.ClusterID,
					networkmsg.ICSREQUEST,
					payload,
				)

				if err != nil {
					k.logger.Error("Failed to send ics message", "krama-id", kramaID, "err", err)
				}
			}(kramaID, icsSetType)
		}
	}

	waitGroup.Wait()
}

// SendICSResponse sends an ICS response to a specific KramaID with the given context type and status code
func (k *Engine) SendICSResponse(
	kramaID id.KramaID,
	clusterID common.ClusterID,
	contextType int32,
	statusCode types.ICSResponseCode,
) error {
	k.logger.Trace("Sending ICS Response", "cluster-id", clusterID, "to", kramaID, "status-code", statusCode)

	if err := k.transport.SendMessage(
		k.ctx,
		kramaID,
		k.selfID,
		clusterID,
		networkmsg.ICSRESPONSE,
		types.NewICSResponse(contextType, statusCode)); err != nil {
		k.logger.Error("Failed to send ics response", "krama-id", kramaID, "err", err)

		return err
	}

	return nil
}

// sendICSSuccess broadcasts an ICS success message to all the connected peers.
func (k *Engine) sendICSSuccess(ctx context.Context, clusterID common.ClusterID) error {
	spanCtx, span := tracing.Span(ctx, "Krama.Engine", "sendICSSuccessMsg")
	defer span.End()

	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	var (
		clusterState = slot.ClusterState()
		payload      = clusterState.CreateICSSuccessMsg()
	)

	rawData, err := payload.Bytes()
	if err != nil {
		return err
	}

	icsMsg := types.NewICSMsg(k.selfID, clusterID, networkmsg.ICSSUCCESS, rawData)

	clusterState.SetSuccessMsg(icsMsg)

	k.transport.BroadcastMessage(spanCtx, icsMsg)

	return nil
}

// sendICSFailure broadcasts an ICS failure message to all the connected peers.
func (k *Engine) sendICSFailure(ctx context.Context, clusterID common.ClusterID) error {
	payload, err := types.NewICSFailure(clusterID).Bytes()
	if err != nil {
		return err
	}

	k.transport.BroadcastMessage(ctx, types.NewICSMsg(k.selfID, clusterID, networkmsg.ICSFAILURE, payload))

	return nil
}

// sendVote broadcasts the vote o all the connected peers using ICSHave
func (k *Engine) sendVote(
	ctx context.Context,
	clusterID common.ClusterID,
	vote *types.VoteMessage,
) error {
	payload, err := types.NewICSHave(nil, vote.Vote).Bytes()
	if err != nil {
		return err
	}

	k.transport.BroadcastMessage(ctx, types.NewICSMsg(k.selfID, clusterID, networkmsg.ICSHAVE, payload))

	return nil
}
