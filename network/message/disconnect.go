package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type DisconnectReq struct {
	Reason string
}

func (disconnReq *DisconnectReq) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(disconnReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize disconnect request message")
	}

	return rawData, nil
}

func (disconnReq *DisconnectReq) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(disconnReq, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize disconnect request message")
	}

	return nil
}
