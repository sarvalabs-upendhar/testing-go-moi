package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"
)

type MsgType int64

const (
	PREPARE MsgType = iota + 1
	PREPARED
	PROPOSAL
	VOTEMSG
	NEWIXSMSG
	ICSGRAFT
	RANDOMWALKREQ
	ACCSTATUSMSG
	ACCSYNCREQ
	ACCSYNCRRESP
	NTQTABLESYNCREQ
	NTQTABLESYNCRESP
	HANDSHAKEMSG
	AGORAREQ
	AGORARESP
	DISCONNECTREQ
)

var NilMessage Message

type Payload interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type Message struct {
	MsgType MsgType
	Sender  identifiers.KramaID
	Payload []byte
}

func (m *Message) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize message")
	}

	return rawData, nil
}

func (m *Message) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(m, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize message")
	}

	return nil
}

func (m *Message) IsHandShakeMessage() bool {
	var hsMsg HandshakeMSG

	if err := hsMsg.FromBytes(m.Payload); err != nil {
		return false
	}

	return true
}
