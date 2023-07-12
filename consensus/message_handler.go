package consensus

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	networkmsg "github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
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
				k.logger.Debug("Inbound message channel close")

				return
			}

			if err := k.handleInboundMsg(slot, inboundMsg); err != nil {
				k.logger.Error("Error handling inbound message", "cluster-ID", slot.InboundChan)
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
				k.logger.Debug("Outbound message channel close")

				return
			}

			if err := k.handleOutboundMsg(slot, outboundMsg); err != nil {
				k.logger.Error("Error handling outbound message.", "cluster-ID", slot.InboundChan)
			}
		}
	}
}

func (k *Engine) handleOutboundMsg(slot *ktypes.Slot, msg ktypes.ConsensusMessage) error {
	peerID, data := msg.PeerID, msg.Message

	switch consensusMsg := data.(type) {
	// Vote Message
	case *ktypes.VoteMessage:
		rawData, err := consensusMsg.Vote.Bytes()
		if err != nil {
			return err
		}

		slot.OutboundChan <- &ktypes.ICSMSG{
			MsgType:   networkmsg.VOTEMSG,
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

func (k *Engine) handleInboundMsg(slot *ktypes.Slot, msg *ktypes.ICSMSG) error {
	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.ClusterState()

	sender, data, msgType := msg.Sender, msg.Msg, msg.MsgType
	switch msgType {
	case networkmsg.VOTEMSG:
		vote := new(ktypes.Vote)

		// Unmarshal message
		if err := vote.FromBytes(data); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to depolarise vote message from %s", sender))
		}
		// Create a consensus message for the Vote
		consensusMsg := ktypes.ConsensusMessage{
			PeerID:  sender,
			Message: &ktypes.VoteMessage{Vote: vote},
		}

		slot.ForwardMsg(consensusMsg)

	case networkmsg.ICSSUCCESS:
		// Unmarshal into an ICS success message
		successMsg := new(networkmsg.ICSSuccessMsg)

		if err := successMsg.FromBytes(data); err != nil {
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
		clusterState.NodeSet.Nodes[common.ObserverSet] = common.NewNodeSet(successMsg.ObserverSet, observerPublicKeys)
		clusterState.NodeSet.Nodes[common.ObserverSet].QuorumSize = successMsg.QuorumSizes[common.ObserverSet]
		clusterState.NodeSet.Nodes[common.RandomSet] = common.NewNodeSet(successMsg.RandomSet, randomPublicKeys)
		clusterState.NodeSet.Nodes[common.RandomSet].QuorumSize = successMsg.QuorumSizes[common.RandomSet]

		clusterState.UpdateClusterSize()

		for j := 0; j < len(clusterState.NodeSet.Nodes); j++ {
			if successMsg.Responses[j] != nil && successMsg.Responses[j].Size > 0 {
				clusterState.NodeSet.Nodes[j].Responses = successMsg.Responses[j]
				clusterState.NodeSet.Nodes[j].RespCount = clusterState.NodeSet.Nodes[j].Responses.TrueIndicesSize()
			}
		}

		k.logger.Info(
			"Received ICS success message",
			"cluster-ID", successMsg.ClusterID,
		)

		clusterState.SuccessMsg = msg
		slot.ICSSuccessChan <- true
	default:
		return errors.New("invalid message type")
	}

	return nil
}
