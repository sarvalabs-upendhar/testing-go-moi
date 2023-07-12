package message

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type PeerInfo struct {
	ID   peer.ID
	Data []byte
}

func (pi *PeerInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(pi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize peer info")
	}

	return rawData, nil
}

type SyncReputationInfo struct {
	Msg []PeerInfo
}
