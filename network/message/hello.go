package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"
)

type HelloMsg struct {
	KramaID   kramaid.KramaID
	Address   []string
	Signature []byte
}

func (hm *HelloMsg) Canonical() ([]byte, error) {
	msg := HelloMsg{
		KramaID:   hm.KramaID,
		Address:   hm.Address,
		Signature: nil,
	}

	return msg.Bytes()
}

func (hm *HelloMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(hm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize hello message")
	}

	return rawData, nil
}

func (hm *HelloMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(hm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize hello message")
	}

	return nil
}
