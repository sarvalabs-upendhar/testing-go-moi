package common

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

type PeerInfo struct {
	ID      string `json:"krama_id"`
	Address string `json:"address"`
}

type PeerList struct {
	TrustedPeers []PeerInfo `json:"trusted_peers"`
	StaticPeers  []PeerInfo `json:"static_peers"`
}

var ErrReadingPeerList = errors.New("error reading peer list file")

// ReadPeerList reads the list of trusted and static peers from the given file and returns it.
func ReadPeerList(path string) (*PeerList, error) {
	if path == "" {
		return &PeerList{}, nil
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrReadingPeerList
	}

	peerList := new(PeerList)
	if err = json.Unmarshal(file, peerList); err != nil {
		return nil, ErrReadingPeerList
	}

	return peerList, nil
}
