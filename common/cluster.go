package common

import (
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	id "github.com/sarvalabs/go-moi/common/kramaid"
)

// ClusterID ...
type ClusterID string

func (c ClusterID) String() string {
	return string(c)
}

func (c ClusterID) Hash() Hash {
	rawHash, err := base58.Decode(c.String())
	if err != nil {
		return NilHash
	}

	return BytesToHash(rawHash)
}

type ICSClusterInfo struct {
	RandomSet   []id.KramaID
	ObserverSet []id.KramaID
	Responses   []*ArrayOfBits
}

func (ci *ICSClusterInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ci)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics cluster info")
	}

	return rawData, nil
}

func (ci *ICSClusterInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ci, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics cluster info")
	}

	return nil
}
