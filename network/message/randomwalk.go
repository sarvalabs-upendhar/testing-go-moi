package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/kramaid"
)

type RandomWalkReq struct {
	ReqID  int64
	Count  int32
	Topic  string
	PeerID kramaid.KramaID
}

func (rwr *RandomWalkReq) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rwr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize random walk request")
	}

	return nil
}

type RandomWalkResp struct {
	ReqID    int64
	ID       kramaid.KramaID
	PeerAddr []string
}

func (rwr *RandomWalkResp) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(rwr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize random walk response")
	}

	return rawData, nil
}

func (rwr *RandomWalkResp) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rwr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize random walk response")
	}

	return nil
}
