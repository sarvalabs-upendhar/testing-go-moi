package p2p

import (
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PeerScores struct {
	scores map[peer.ID]*pubsub.PeerScoreSnapshot
	mu     sync.RWMutex
}

func newPeerScores() *PeerScores {
	return &PeerScores{
		scores: make(map[peer.ID]*pubsub.PeerScoreSnapshot),
	}
}

func (s *PeerScores) Update(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {
	s.mu.Lock()
	s.scores = scores
	s.mu.Unlock()
}

func (s *PeerScores) Get() map[peer.ID]*pubsub.PeerScoreSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.scores
}
