package types

import (
	"sync"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
)

// HeightVoteSet is a struct that represents a set of votes across multiple heights and votes
type HeightVoteSet struct {
	logger hclog.Logger

	tsHash common.Hash

	// Represents the slice of lattice IDs for the voteset
	chainIDs []string

	// Represents the slice of heights tracked by the voteset
	heights map[identifiers.Identifier]uint64

	// Represents the cluster state
	cs *ClusterState

	// Represents the highest tracking view of the voteset
	view uint64

	// Represents a mapping of view number to the voteset for that view.
	viewVoteSet ViewVoteSet

	// Represents a synchronization mutex for the voteset
	mtx sync.Mutex
}

// NewHeightVoteSet is a constructor function that generates and returns a new HeightVoteSet.
// Accepts a slice of chainIDs, heights and the set of validators.
func NewHeightVoteSet(
	chainIDs []string,
	heights map[identifiers.Identifier]uint64,
	valset *ClusterState,
	logger hclog.Logger,
) *HeightVoteSet {
	// Create a new HeightVoteSet with the lattice IDs
	hvs := &HeightVoteSet{
		logger:   logger.Named("Height-VoteSet"),
		chainIDs: chainIDs,
		mtx:      sync.Mutex{},
	}
	// Reset the HVS and return it
	hvs.Reset(heights, valset)

	return hvs
}

func (hvs *HeightVoteSet) SetTSHash(tsHash common.Hash) {
	hvs.tsHash = tsHash
}

// Reset is a method of HeightVoteSet that resets the vote set.
// Accepts a slice of heights and a set of validators to assign to the height vote set.
func (hvs *HeightVoteSet) Reset(heights map[identifiers.Identifier]uint64, valset *ClusterState) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Set the heights and validator set
	hvs.heights = heights
	hvs.cs = valset

	// TODO: Should we reset the peerCatchupRounds?
	// hvs.peerCatchupRounds = make(map[id.KramaID][]int32)

	// Reset the viewVoteSet and add the 0 view
	hvs.viewVoteSet = ViewVoteSet{
		Prevotes:   NewVoteSet(hvs.heights, valset.currentView.ID(), common.PREVOTE, hvs.cs, hvs.logger),
		Precommits: NewVoteSet(hvs.heights, valset.currentView.ID(), common.PRECOMMIT, hvs.cs, hvs.logger),
	}

	hvs.view = valset.currentView.ID()
}

func (hvs *HeightVoteSet) AddQC(v *Vote) (bool, error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	hvs.logger.Debug("Adding Qc", "vote-view", v.View, "vote-type", v.Type)

	vs := hvs.getVoteSet(v.Type)

	return vs.AddQc(v, hvs.tsHash)
}

// AddVote is a method of HeightVoteSet that adds a new vote for a peer id.
// Adds the vote the voteset that corresponds to the vote view and type.
// If the peer has more than two catchup rounds, the vote is not added.
func (hvs *HeightVoteSet) AddVote(v *Vote) (bool, error) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// TODO: Check vote validity

	vs := hvs.getVoteSet(v.Type)

	return vs.AddVote(v, hvs.tsHash)
}

// getVoteSet is a method of HeightVoteSet that retrieves the VoteSet for a given vote type and view.
// Returns nil if view does not exist and panics if the votetype is invalid.
func (hvs *HeightVoteSet) getVoteSet(votetype common.ConsensusMsgType) *VoteSet {
	switch votetype {
	// PREVOTE set
	case common.PREVOTE:
		return hvs.viewVoteSet.Prevotes

	// PRECOMMIT set
	case common.PRECOMMIT:
		return hvs.viewVoteSet.Precommits
	}

	return nil
}

// Prevotes is a method of HeightVoteSet that retrieves the prevotes from the set for a given view value.
func (hvs *HeightVoteSet) Prevotes() *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the prevotes for the given view
	return hvs.getVoteSet(common.PREVOTE)
}

// Precommits is a method of HeightVoteSet that retrieves the precommits from the set for a given view value.
func (hvs *HeightVoteSet) Precommits() *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the precommits for the given view
	return hvs.getVoteSet(common.PRECOMMIT)
}

func (hvs *HeightVoteSet) GetQC(tsHash common.Hash, view uint64, voteType common.ConsensusMsgType) (*common.Qc, error) {
	vs := hvs.getVoteSet(voteType)

	if *vs.maj23TsHash != tsHash {
		return nil, errors.New("ts hash doesn't match with super majority")
	}

	return &common.Qc{
		Type:          voteType,
		View:          view,
		TSHash:        *vs.maj23TsHash,
		SignerIndices: vs.votesBitArray,
		Signature:     vs.maj23aggSignature,
	}, nil
}
