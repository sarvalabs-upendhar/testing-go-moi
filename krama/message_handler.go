package krama

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	ktypes "gitlab.com/sarvalabs/moichain/krama/types"
	"gitlab.com/sarvalabs/moichain/types"
	"gitlab.com/sarvalabs/polo/go-polo"
)

func (k *Engine) startMessageHandlers(ctx context.Context, slot *ktypes.Slot) {
	go k.icsInboundMessageHandler(ctx, slot)
	go k.icsOutboundMessageHandler(ctx, slot)
}

func (k *Engine) icsInboundMessageHandler(ctx context.Context, slot *ktypes.Slot) {
	for {
		select {
		case <-ctx.Done():
			return
		case inboundMsg, ok := <-slot.InboundChan:
			if !ok {
				k.logger.Debug("inbound msg channel close")

				return
			}

			if err := k.handleInboundMsg(slot, inboundMsg); err != nil {
				k.logger.Error("Error handling inbound message", "cluster-id", slot.InboundChan)
			}
		}
	}
}

func (k *Engine) icsOutboundMessageHandler(ctx context.Context, slot *ktypes.Slot) {
	defer close(slot.OutboundChan)

	for {
		select {
		case <-ctx.Done():
			return
		case outboundMsg, ok := <-slot.BftOutboundChan:
			if !ok {
				k.logger.Debug("outbound msg channel close")

				return
			}

			if err := k.handleOutboundMsg(slot, outboundMsg); err != nil {
				k.logger.Error("Error handling outbound message", "cluster-id", slot.InboundChan)
			}
		}
	}
}

func (k *Engine) handleOutboundMsg(slot *ktypes.Slot, msg types.ConsensusMessage) error {
	peerID, data := msg.PeerID, msg.Message

	switch consensusMsg := data.(type) {
	// Vote Message
	case *types.VoteMessage:
		// Marshal proto message into an ClusterInfo message and push into the send queue
		rawData := consensusMsg.Vote.Bytes()
		slot.OutboundChan <- &types.ICSMSG{
			MsgType:   types.VOTEMSG,
			Msg:       rawData,
			Sender:    peerID,
			ClusterID: string(slot.ClusterID()),
		}

	// Unsupported Message
	default:
		return errors.New("invalid message type")
	}

	return nil
}

func (k *Engine) handleInboundMsg(slot *ktypes.Slot, msg *types.ICSMSG) error {
	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.CLusterInfo()

	sender, data, msgType := msg.Sender, msg.Msg, msg.MsgType
	switch msgType {
	case types.VOTEMSG:
		vote := new(types.Vote)

		// Unmarshal message
		if err := polo.Depolorize(vote, data); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to depolarise vote message from %s", sender))
		}
		// Create a consensus message for the Vote
		consensusMsg := types.ConsensusMessage{
			PeerID:  sender,
			Message: &types.VoteMessage{Vote: vote},
		}

		slot.ForwardMsg(consensusMsg)

	case types.ICSSUCCESS:
		// Unmarshal into an ICS success message
		successMsg := new(types.ICSSuccessMsg)

		if err := polo.Depolorize(successMsg, data); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to depolarise ics_success message from %s", sender))
		}

		observerPublicKeys, err := k.state.GetPublicKeys(successMsg.ObserverSet...)
		if err != nil {
			return errors.New("failed to retrieve public keys")
		}

		randomPublicKeys, err := k.state.GetPublicKeys(successMsg.RandomSet...)
		if err != nil {
			return errors.New("failed to retrieve public keys")
		}
		// update the cluster state with the latest node set's
		clusterState.ICS.Nodes[types.ObserverSet] = types.NewNodeSet(successMsg.ObserverSet, observerPublicKeys)
		clusterState.ICS.Nodes[types.ObserverSet].QuorumSize = successMsg.QuorumSizes[types.ObserverSet]
		clusterState.ICS.Nodes[types.RandomSet] = types.NewNodeSet(successMsg.RandomSet, randomPublicKeys)
		clusterState.ICS.Nodes[types.RandomSet].QuorumSize = successMsg.QuorumSizes[types.RandomSet]

		clusterState.UpdateClusterSize()

		for j := 0; j < len(clusterState.ICS.Nodes); j++ {
			if successMsg.Responses[j] != nil && successMsg.Responses[j].Size > 0 {
				clusterState.ICS.Nodes[j].Responses = successMsg.Responses[j]
				clusterState.ICS.Nodes[j].Count = clusterState.ICS.Nodes[j].Responses.TrueIndicesSize()
			}
		}

		k.logger.Info(
			"Received ics_success msg",
			"Cluster id", successMsg.ClusterID,
		)

		clusterState.SuccessMsg = msg
		slot.ICSSuccessChan <- true
	default:
		return errors.New("invalid message type")
	}

	return nil
}
