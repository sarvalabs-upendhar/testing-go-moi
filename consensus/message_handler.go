package consensus

import (
	"time"

	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// handler initializes and runs a loop to handle inbound messages.
func (k *Engine) handler() {
	for {
		select {
		case <-k.ctx.Done():
			return
		case msg := <-k.transport.Messages():
			k.handleInboundMessage(msg)
		case clusterID := <-k.slotCloseCh:
			k.logger.Debug("Cleaning consensus slot", clusterID)
			k.slots.CleanupSlot(clusterID)
		}
	}
}

// handleInboundMessage dispatches the incoming message to the appropriate handler based on the message type.
func (k *Engine) handleInboundMessage(msg *types.ICSMSG) {
	switch msg.MsgType {
	case message.ICSREQUEST:
		go func() {
			if err := k.handleICSRequest(msg); err != nil {
				k.logger.Error("Failed to handle ics request", "err", err, "cluster-id", msg.ClusterID)
			}
		}()
	case message.ICSRESPONSE:
		if err := k.handleICSResponse(msg); err != nil {
			k.logger.Error("Failed to handle ics response", "err", err, "cluster-id", msg.ClusterID)
		}
	case message.ICSSUCCESS:
		if err := k.handleICSSuccess(msg); err != nil {
			k.logger.Error("Failed to handle ics success", "err", err, "cluster-id", msg.ClusterID)
		}
	case message.ICSFAILURE:
		if err := k.handleICSFailure(msg); err != nil {
			k.logger.Error("Failed to handle ics failure", "err", err, "cluster-id", msg.ClusterID)
		}
	case message.ICSHAVE:
		if err := k.handleICSHave(msg); err != nil {
			k.logger.Error("Failed to handle ics have", "err", err, "cluster-id", msg.ClusterID)
		}
	default:
		k.logger.Error("Unsupported message type")
	}
}

// handleICSRequest handles the incoming ICS request, processes it, and sends a response.
func (k *Engine) handleICSRequest(msg *types.ICSMSG) error {
	var (
		icsReq          = new(types.ICSRequest)
		canonicalICSReq = new(types.CanonicalICSRequest)
		respChan        = make(chan types.Response)
		interactions    = new(common.Interactions)
		statusCode      = types.InternalError
		operators       = make([]types.ICSOperatorInfo, 0)
	)

	k.logger.Trace("Received ICS request", "cluster-id", msg.ClusterID, "sender", msg.Sender)
	ctx, span := tracing.Span(
		k.ctx,
		"Krama.KramaEngine",
		"handleICSReq",
		trace.WithAttributes(
			attribute.String("clusterID", msg.ClusterID.String()),
			attribute.Int("slotType", int(types.ValidatorSlot)),
		),
	)

	defer span.End()

	if err := k.transport.ConnectToDirectPeer(ctx, msg.Sender, msg.ClusterID); err != nil {
		k.logger.Error("Failed to connect to direct peer", "peer-id", msg.Sender, "error", err)
	}

	defer func() {
		if err := k.SendICSResponse(msg.Sender, msg.ClusterID, statusCode, operators); err != nil {
			k.logger.Error("Failed to send ics response", "err", err)
		}

		if statusCode != types.Success {
			k.transport.CleanDirectPeer(msg.ClusterID, msg.Sender)
		}
	}()

	if err := icsReq.FromBytes(msg.Payload); err != nil {
		return err
	}

	if err := canonicalICSReq.FromBytes(icsReq.ReqData); err != nil {
		return err
	}

	if err := interactions.FromBytes(canonicalICSReq.IxData); err != nil {
		return err
	}

	if err := k.verifyICSRequest(id.KramaID(canonicalICSReq.Operator), icsReq, interactions); err != nil {
		return err
	}

	kramaRequest := types.Request{
		Ctx:          ctx,
		SlotType:     types.ValidatorSlot,
		Operator:     id.KramaID(canonicalICSReq.Operator),
		Msg:          canonicalICSReq,
		ResponseChan: respChan,
		Ixs:          *interactions,
		ReqTime:      time.Unix(0, canonicalICSReq.Timestamp),
	}

	k.Requests() <- kramaRequest

	select {
	case <-k.ctx.Done():
		return k.ctx.Err()
	case resp := <-respChan:
		if resp.Data != nil {
			operators = resp.Data.([]types.ICSOperatorInfo) //nolint
		}

		if resp.Err == nil {
			statusCode = types.Success

			return nil
		}

		switch resp.Err.Error() {
		case common.ErrSlotsFull.Error():
			statusCode = types.SlotsFull
		case common.ErrHashMismatch.Error():
			statusCode = types.InvalidHash
		case common.ErrOperatorNotEligible.Error():
			statusCode = types.NotEligible
		default:
			statusCode = types.InternalError
		}

		return resp.Err
	}
}

// handleICSResponse handles the incoming ICS response and updates the cluster state accordingly.
func (k *Engine) handleICSResponse(msg *types.ICSMSG) error {
	var icsRes types.ICSResponse

	err := icsRes.FromBytes(msg.Payload)
	if err != nil {
		return err
	}

	k.logger.Trace(
		"Received ICS response",
		"cluster-id", msg.ClusterID,
		"sender", msg.Sender,
		"status-code", icsRes.StatusCode,
	)

	slot := k.slots.GetSlot(msg.ClusterID)
	if slot == nil {
		return nil
	}

	slot.ClusterState().IncrementICSRespCount(1)

	for _, info := range icsRes.OperatorsInfo {
		k.logger.Trace(
			"Adding Operator Info",
			"ixnHash", slot.ClusterState().IxnHash(),
			"krama-id", info.KramaID,
			"priority", info.Priority,
		)

		k.lottery.AddICSOperatorInfo(slot.ClusterState().LotteryKey, info.KramaID, info.Priority)
	}

	if icsRes.StatusCode != types.Success {
		return nil
	}

	// ignore the response if the success message already exists
	if slot.ClusterState().GetSuccessMsg() != nil {
		return nil
	}

	for _, nodeset := range slot.ClusterState().NodeSet.Sets {
		if nodeset == nil {
			continue
		}

		for index, kramaID := range nodeset.Ids {
			if kramaID == msg.Sender {
				nodeset.UpdateResponse(index, true)
			}
		}
	}

	return nil
}

// handleICSSuccess handles the ICS success message. Updates the cluster state and sends success signal to krama engine.
func (k *Engine) handleICSSuccess(msg *types.ICSMSG) error {
	var icsSuccess types.ICSSuccess

	k.logger.Trace("Received ICS success", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	err := icsSuccess.FromBytes(msg.Payload)
	if err != nil {
		return err
	}

	slot := k.slots.GetSlot(msg.ClusterID)
	if slot == nil {
		return nil
	}

	clusterState := slot.ClusterState()

	icsSetType, nodeIdx := clusterState.GetICSNodeIndex(k.selfID)

	// ignore the ics success message if the current node's response is not accounted in the ics success response
	if nodeIdx == -1 {
		return errors.New("node not found in nodeset")
	}

	if icsSuccess.Responses[icsSetType] == nil || !icsSuccess.Responses[icsSetType].GetIndex(nodeIdx) {
		return errors.New("node response not found")
	}

	for j := 0; j < len(clusterState.NodeSet.Sets); j++ {
		if icsSuccess.Responses[j] != nil && icsSuccess.Responses[j].Size > 0 {
			clusterState.UpdateNodeSetResponses(j, icsSuccess.Responses[j])
		}
	}

	clusterState.SetSuccessMsg(msg)

	select {
	case slot.ICSSuccessChan <- true:
	default:
		k.logger.Trace("Failed to forward msg to ICS success channel", "cluster-id", msg.ClusterID, "sender", msg.Sender)
	}

	return nil
}

// handleICSFailure handles the ICS failure message and sends failure signal to krama engine.
func (k *Engine) handleICSFailure(msg *types.ICSMSG) error {
	var icsFailure types.ICSFailure

	k.logger.Trace("Received ICS failure", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	err := icsFailure.FromBytes(msg.Payload)
	if err != nil {
		return err
	}

	slot := k.slots.GetSlot(msg.ClusterID)
	if slot == nil {
		return nil
	}

	select {
	case slot.ICSSuccessChan <- false:
	default:
	}

	return nil
}

// handleICSHave handles the ICS have message and forwards it to the appropriate slot.
func (k *Engine) handleICSHave(msg *types.ICSMSG) error {
	slot := k.slots.GetSlot(msg.ClusterID)
	if slot == nil {
		return nil
	}

	for _, vote := range msg.DecodedMsg.(*types.ICSHave).Votes { //nolint
		slot.ForwardMsg(types.ConsensusMessage{
			PeerID:  msg.Sender,
			Message: &types.VoteMessage{Vote: vote},
		})
	}

	return nil
}
