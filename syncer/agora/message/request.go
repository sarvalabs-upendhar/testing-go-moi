package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/syncer/cid"
)

type AgoraRequestMsg struct {
	SessionID identifiers.Identifier
	StateHash cid.CID
	WantList  []cid.CID
}

func (req *AgoraRequestMsg) GetSessionID() identifiers.Identifier {
	return req.SessionID
}

func (req *AgoraRequestMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(req, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize agora request message")
	}

	return nil
}
