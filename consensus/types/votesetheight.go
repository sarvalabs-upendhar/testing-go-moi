package types

import (
	"log"
	"sync"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
)

// HeightVoteSet is a struct that represents a set of votes across multiple heights and votes
type HeightVoteSet struct {
	logger hclog.Logger

	// Represents the slice of lattice IDs for the voteset
	chainIDs []string

	// Represents the slice of heights tracked by the voteset
	heights map[identifiers.Identifier]uint64

	// Represents the cluster state
	cs *ClusterState

	// Represents the highest tracking view of the voteset
	view uint64

	// Represents a mapping of view number to the voteset for that view.
	viewVoteSet map[uint64]ViewVoteSet

	// Represents a mapping of peer IDs to a slice of rounds that the peer is catching up on.
	// A peer will have at most 2 rounds in its catchup.
	peerCatchupRounds map[identifiers.KramaID][]uint64

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
		logger:            logger.Named("Height-VoteSet"),
		chainIDs:          chainIDs,
		mtx:               sync.Mutex{},
		peerCatchupRounds: make(map[identifiers.KramaID][]uint64),
	}
	// Reset the HVS and return it
	hvs.Reset(heights, valset)

	return hvs
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
	hvs.viewVoteSet = make(map[uint64]ViewVoteSet)
	hvs.addView(0)
	hvs.view = 0
}

func (hvs *HeightVoteSet) PreCommitAggregatedSignature(
	viewID uint64,
	ts common.Hash,
) ([]byte, *common.ArrayOfBits, error) {
	preCommits := hvs.Precommits(viewID)
	tesseractPreCommits := preCommits.votesByTesseract[ts]

	aggregatedSignature, err := tesseractPreCommits.AggregateSignatures()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to aggregate signatures")
	}

	return aggregatedSignature, tesseractPreCommits.bitarray, err
}

func (hvs *HeightVoteSet) AddQC(v *Vote, peerID identifiers.KramaID) (bool, error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	hvs.logger.Debug("Adding Qc", "vote-view", v.View, "vote-type", v.Type)

	vs := hvs.getVoteSet(v.View, v.Type)
	if vs != nil {
		return vs.AddQc(v)
	}

	hvs.addView(v.View)
	vs = hvs.getVoteSet(v.View, v.Type)

	return vs.AddQc(v)
}

// AddVote is a method of HeightVoteSet that adds a new vote for a peer id.
// Adds the vote the voteset that corresponds to the vote view and type.
// If the peer has more than two catchup rounds, the vote is not added.
func (hvs *HeightVoteSet) AddVote(v *Vote, peerID identifiers.KramaID) (bool, error) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// TODO: Check vote validity

	// Get voteset for the vote type and view
	if vs := hvs.getVoteSet(v.View, v.Type); vs == nil {
		// If the voteset does not exist, check if peer has less than 2 catchup rounds.
		if rounds := hvs.peerCatchupRounds[peerID]; len(rounds) < 2 {
			// Add the view to the HeightVoteSet
			hvs.addView(v.View)
			vs = hvs.getVoteSet(v.View, v.Type)

			// Add the view to the catch up rounds and the voteset
			hvs.peerCatchupRounds[peerID] = append(rounds, v.View)

			return vs.AddVote(v)
		} else {
			return false, errors.New("could not add vote. peer has more than 2 catchup rounds")
		}
	} else {
		// Add the vote to the voteset
		return vs.AddVote(v)
	}
}

// SetView is a method of HeightVoteSet that sets the view for a given view value.
// Ensures that the given view value is greater than the current view or that the current view is not 0.
// Adds all rounds between current view and given view if it does not exist already exist.
func (hvs *HeightVoteSet) SetView(view uint64) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	nextround := hvs.view - 1
	// Panic if given view value is not greater than the current view if the current view is not 0
	if hvs.view != 0 && (view < nextround) {
		log.Panic("can't add new view")
	}

	// For every view from current hvs view to given
	// view, add the view if it does not exist
	for r := nextround; r <= view; r++ {
		if _, ok := hvs.viewVoteSet[r]; ok {
			continue
		}

		hvs.addView(r)
	}

	// Update the view value on the height vote set
	hvs.view = view
}

// addView is a method of HeightVoteSet that adds a new view to the vote set.
// Creates a new RoundVoteSet and adds it to the set's viewVoteSet slice.
// Panics if the rounds already exists.
func (hvs *HeightVoteSet) addView(v uint64) {
	// Panic if view already exists
	if _, ok := hvs.viewVoteSet[v]; ok {
		log.Panicln("View already exists")
	}

	// Create a new RoundVoteSet and set it for the view
	hvs.viewVoteSet[v] = ViewVoteSet{
		Prevotes:   NewVoteSet(hvs.heights, v, common.PREVOTE, hvs.cs, hvs.logger),
		Precommits: NewVoteSet(hvs.heights, v, common.PRECOMMIT, hvs.cs, hvs.logger),
	}
}

func (hvs *HeightVoteSet) GetViewBitVoteSet() map[uint64]*VoteBitSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	if len(hvs.viewVoteSet) == 0 {
		return nil
	}

	// Create a copy of the map to avoid concurrency issues
	rvs := make(map[uint64]*VoteBitSet, len(hvs.viewVoteSet))

	for key, value := range hvs.viewVoteSet {
		if key == 0 {
			continue
		}

		set := value.VoteBitSet()
		if set != nil {
			rvs[key] = set
		}
	}

	return rvs
}

// getVoteSet is a method of HeightVoteSet that retrieves the VoteSet for a given vote type and view.
// Returns nil if view does not exist and panics if the votetype is invalid.
func (hvs *HeightVoteSet) getVoteSet(view uint64, votetype common.ConsensusMsgType) *VoteSet {
	// Retrieve the view vote set for the view
	rvs, ok := hvs.viewVoteSet[view]
	if !ok {
		// Empty return if view does not exist
		return nil
	}

	switch votetype {
	// PREVOTE set
	case common.PREVOTE:
		return rvs.Prevotes

	// PRECOMMIT set
	case common.PRECOMMIT:
		return rvs.Precommits
	}

	return nil
}

func (hvs *HeightVoteSet) GetVoteSet(view uint64, votetype common.ConsensusMsgType) *VoteSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	return hvs.getVoteSet(view, votetype)
}

func (hvs *HeightVoteSet) GetVotes(viewVoteSet map[uint64]*VoteBitSet) []*Vote {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	votes := make([]*Vote, 0)

	for round, voteBitSet := range viewVoteSet {
		if _, ok := hvs.viewVoteSet[round]; !ok {
			continue
		}

		if voteBitSet.Prevotes != nil && !voteBitSet.Prevotes.IsEmpty() {
			prevotes := hvs.getVoteSet(round, common.PREVOTE)
			for _, valIndex := range voteBitSet.Prevotes.GetTrueIndices() {
				prevotes.mtx.RLock()

				v, err := prevotes.getVoteByIndex(valIndex)
				if err != nil {
					continue
				}

				votes = append(votes, v)
				prevotes.mtx.RUnlock()
			}
		}

		if voteBitSet.Precommits != nil && !voteBitSet.Precommits.IsEmpty() {
			preCommits := hvs.getVoteSet(round, common.PRECOMMIT)
			for _, valIndex := range voteBitSet.Precommits.GetTrueIndices() {
				preCommits.mtx.RLock()

				v, err := preCommits.getVoteByIndex(valIndex)
				if err != nil {
					continue
				}

				votes = append(votes, v)

				preCommits.mtx.RUnlock()
			}
		}
	}

	return votes
}

// Prevotes is a method of HeightVoteSet that retrieves the prevotes from the set for a given view value.
func (hvs *HeightVoteSet) Prevotes(view uint64) *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the prevotes for the given view
	return hvs.getVoteSet(view, common.PREVOTE)
}

// Precommits is a method of HeightVoteSet that retrieves the precommits from the set for a given view value.
func (hvs *HeightVoteSet) Precommits(view uint64) *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the precommits for the given view
	return hvs.getVoteSet(view, common.PRECOMMIT)
}

// POLInfo is a method of HeightVoteSet that retrieves the last view with a Proof of Lock.
// Returns the view number and the tesseract hash with the lock.
func (hvs *HeightVoteSet) POLInfo() (uint64, common.Hash) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Check all rounds going back from current view
	for r := hvs.view; r <= 0; r-- {
		// Get the PREVOTE's for the view
		rvs := hvs.getVoteSet(r, common.PREVOTE)

		// If 2/3 majority exists for view, return the tesseract hash and the view value
		tsHash, ok := rvs.SuperMajority()
		if ok {
			return r, tsHash
		}
	}

	// If no view has a proof of lock, return empty
	return 0, common.NilHash
}

func (hvs *HeightVoteSet) GetQC(tsHash common.Hash, view uint64, voteType common.ConsensusMsgType) (*common.Qc, error) {
	vs := hvs.getVoteSet(view, voteType)
	if vs == nil {
		return nil, errors.New("invalid vote type")
	}

	if *vs.maj23TsHash != tsHash {
		return nil, errors.New("ts hash doesn't match with super majority")
	}

	return &common.Qc{
		Type:          voteType,
		View:          view,
		TSHash:        *vs.maj23TsHash,
		SignerIndices: vs.maj23VoteSet,
		Signature:     vs.maj23aggSignature,
	}, nil
}
