package common

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type KramaIDReq struct {
	KramaID string `json:"krama_id"`
	RPCUrl  string `json:"rpc_url"`
}

func (k KramaIDReq) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(k)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize krama id req")
	}

	return rawData, nil
}

func (k *KramaIDReq) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(k, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize krama id req")
	}

	return nil
}
