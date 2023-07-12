package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/syncer/cid"
)

type AgoraRequestMsg struct {
	SessionID common.Address
	StateHash cid.CID
	WantList  []cid.CID
}

func (req *AgoraRequestMsg) GetSessionID() common.Address {
	return req.SessionID
}

func (req *AgoraRequestMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(req, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize agora request message")
	}

	return nil
}
