package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type Response struct {
	PeerID    kramaid.KramaID
	SessionID common.Address
	StateHash cid.CID
	Status    bool
	HaveList  block.HaveList
	PeerSet   []kramaid.KramaID
}

func (r *Response) GetAgoraMsg() *AgoraResponseMsg {
	return &AgoraResponseMsg{
		SessionID: r.SessionID,
		Status:    r.Status,
		HaveList:  r.HaveList.GetRawBlocks(),
		PeerSet:   r.PeerSet,
	}
}

type AgoraResponseMsg struct {
	SessionID common.Address
	Status    bool
	HaveList  [][]byte
	PeerSet   []kramaid.KramaID
}

func (resp *AgoraResponseMsg) GetBlocks() []block.Block {
	blocks := make([]block.Block, 0, len(resp.HaveList))

	for _, data := range resp.HaveList {
		blocks = append(blocks, block.NewBlockFromMessage(data))
	}

	return blocks
}

func (resp *AgoraResponseMsg) GetSessionID() common.Address {
	return resp.SessionID
}

func (resp *AgoraResponseMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(resp, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize agora response message")
	}

	return nil
}
