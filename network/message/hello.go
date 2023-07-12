package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/kramaid"
)

type HelloMsg struct {
	KramaID   kramaid.KramaID
	Address   []string
	Signature []byte
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
