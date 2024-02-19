package consensus

import (
	"time"

	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
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
			k.logger.Debug("Cleaning up the slot", clusterID)
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
		respChan        = make(chan error)
		interactions    = new(common.Interactions)
		contextType     = int32(0)
		statusCode      = types.InternalError
	)

	k.logger.Trace("Received ICS request", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	defer func() {
		if err := k.SendICSResponse(msg.Sender, msg.ClusterID, contextType, statusCode); err != nil {
			k.logger.Error("Failed to send ics response", "err", err)
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
		SlotType:     types.ValidatorSlot,
		Operator:     id.KramaID(canonicalICSReq.Operator),
		Msg:          canonicalICSReq,
		ResponseChan: respChan,
		Ixs:          *interactions,
		ReqTime:      time.Unix(0, canonicalICSReq.Timestamp),
	}

	k.Requests() <- kramaRequest

	// Wait for response from krama engine
	if err := <-respChan; err != nil {
		switch err.Error() {
		case common.ErrSlotsFull.Error():
			statusCode = types.SlotsFull
		case common.ErrHashMismatch.Error():
			statusCode = types.InvalidHash
		default:
			statusCode = types.InternalError
		}

		return err
	}

	contextType = canonicalICSReq.ContextType
	statusCode = types.Success

	return nil
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

	if icsRes.StatusCode != types.Success {
		return nil
	}

	// ignore the response if the success message already exists
	if slot.ClusterState().GetSuccessMsg() != nil {
		return nil
	}

	for _, nodeset := range slot.ClusterState().NodeSet.Nodes {
		if nodeset == nil {
			continue
		}

		for index, kramaID := range nodeset.Ids {
			if kramaID == msg.Sender {
				nodeset.Responses.SetIndex(index, true)
				nodeset.UpdateRespCount()
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

	clusterState.GetNodeSet(common.ObserverSet).QuorumSize = icsSuccess.QuorumSizes[common.ObserverSet]
	clusterState.GetNodeSet(common.RandomSet).QuorumSize = icsSuccess.QuorumSizes[common.RandomSet]

	for j := 0; j < len(clusterState.NodeSet.Nodes); j++ {
		if icsSuccess.Responses[j] != nil && icsSuccess.Responses[j].Size > 0 {
			nodeset := clusterState.GetNodeSet(common.IcsSetType(j))
			nodeset.Responses = icsSuccess.Responses[j]
			nodeset.RespCount = nodeset.Responses.TrueIndicesSize()
		}
	}

	clusterState.SetSuccessMsg(msg)
	slot.ICSSuccessChan <- true

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

	slot.ICSSuccessChan <- false

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
