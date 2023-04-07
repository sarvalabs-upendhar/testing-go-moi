package kbft

import (
	"errors"
	"log"
	"sync"

	"github.com/hashicorp/go-hclog"

	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

// RoundVoteSet is a struct that represents a set for a votes for a single round.
type RoundVoteSet struct {
	// Represents the pre-votes for the round
	Prevotes *VoteSet

	// Represents the pre-commits for the round
	Precommits *VoteSet
}

// HeightVoteSet is a struct that represents a set of votes across multiple heights and votes
type HeightVoteSet struct {
	logger hclog.Logger

	// Represents the slice of lattice IDs for the voteset
	chainIDs []string

	// Represents the slice of heights tracked by the voteset
	heights map[types.Address]uint64

	// Represents the cluster state
	cs *ktypes.ClusterState

	// Represents the highest tracking round of the voteset
	round int32

	// Represents a mapping of round number to the voteset for that round.
	roundVoteSets map[int32]RoundVoteSet

	// Represents a mapping of peer IDs to a slice of rounds that the peer is catching up on.
	// A peer will have at most 2 rounds in its catchup.
	peerCatchupRounds map[id.KramaID][]int32

	// Represents a synchronization mutex for the voteset
	mtx sync.Mutex
}

// NewHeightVoteSet is a constructor function that generates and returns a new HeightVoteSet.
// Accepts a slice of chainIDs, heights and the set of validators.
func NewHeightVoteSet(
	chainIDs []string,
	heights map[types.Address]uint64,
	valset *ktypes.ClusterState,
	logger hclog.Logger,
) *HeightVoteSet {
	// Create a new HeightVoteSet with the lattice IDs
	hvs := &HeightVoteSet{
		logger:            logger,
		chainIDs:          chainIDs,
		mtx:               sync.Mutex{},
		peerCatchupRounds: make(map[id.KramaID][]int32),
	}
	// Reset the HVS and return it
	hvs.Reset(heights, valset)

	return hvs
}

// Reset is a method of HeightVoteSet that resets the vote set.
// Accepts a slice of heights and a set of validators to assign to the height vote set.
func (hvs *HeightVoteSet) Reset(heights map[types.Address]uint64, valset *ktypes.ClusterState) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Set the heights and validator set
	hvs.heights = heights
	hvs.cs = valset

	// TODO: Should we reset the peerCatchupRounds?
	// hvs.peerCatchupRounds = make(map[id.KramaID][]int32)

	// Reset the roundVoteSets and add the 0 round
	hvs.roundVoteSets = make(map[int32]RoundVoteSet)
	hvs.addRound(0)
	hvs.round = 0
}

// AddVote is a method of HeightVoteSet that adds a new vote for a peer id.
// Adds the vote the voteset that corresponds to the vote round and type.
// If the peer has more than two catchup rounds, the vote is not added.
func (hvs *HeightVoteSet) AddVote(v *ktypes.Vote, peerID id.KramaID) (bool, error) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// TODO: Check vote validity

	// Get voteset for the vote type and round
	if vs := hvs.getVoteSet(v.Round, v.Type); vs == nil {
		// If the voteset does not exist, check if peer has less than 2 catchup rounds.
		if rounds := hvs.peerCatchupRounds[peerID]; len(rounds) < 2 {
			// Add the round to the HeightVoteSet
			hvs.addRound(v.Round)
			vs = hvs.getVoteSet(v.Round, v.Type)

			// Add the round to the catch up rounds and the voteset
			hvs.peerCatchupRounds[peerID] = append(rounds, v.Round)

			return vs.AddVote(v, peerID)
		} else {
			return false, errors.New("could not add vote. peer has more than 2 catchup rounds")
		}
	} else {
		// Add the vote to the voteset
		return vs.AddVote(v, peerID)
	}
}

// SetRound is a method of HeightVoteSet that sets the round for a given round value.
// Ensures that the given round value is greater than the current round or that the current round is not 0.
// Adds all rounds between current round and given round if it does not exist already exist.
func (hvs *HeightVoteSet) SetRound(round int32) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	nextround := hvs.round - 1
	// Panic if given round value is not greater than the current round of if the current round is not 0
	if hvs.round != 0 && (round < nextround) {
		log.Panic("can't add new round")
	}

	// For every round from current hvs round to given
	// round, add the round if does not exist
	for r := nextround; r <= round; r++ {
		if _, ok := hvs.roundVoteSets[r]; ok {
			continue
		}

		hvs.addRound(r)
	}

	// Update the round value on the height vote set
	hvs.round = round
}

// addRound is a method of HeightVoteSet that adds a new round to the vote set.
// Creates a new RoundVoteSet and adds it to the set's roundVoteSets slice.
// Panics if the rounds already exists.
func (hvs *HeightVoteSet) addRound(r int32) {
	// Panic if round already exists
	if _, ok := hvs.roundVoteSets[r]; ok {
		log.Panicln("Round already exists")
	}

	// Create a new RoundVoteSet and set it for the round
	hvs.roundVoteSets[r] = RoundVoteSet{
		Prevotes:   NewVoteSet(hvs.heights, r, ktypes.PREVOTE, hvs.cs, hvs.logger),
		Precommits: NewVoteSet(hvs.heights, r, ktypes.PRECOMMIT, hvs.cs, hvs.logger),
	}
}

// getVoteSet is a method of HeightVoteSet that retrieves the VoteSet for a given vote type and round.
// Returns nil if round does not exist and panics if the votetype is invalid.
func (hvs *HeightVoteSet) getVoteSet(round int32, votetype ktypes.ConsensusMsgType) *VoteSet {
	// Retrieve the round vote set for the round
	rvs, ok := hvs.roundVoteSets[round]
	if !ok {
		// Empty return if round does not exist
		return nil
	}

	switch votetype {
	// PREVOTE set
	case ktypes.PREVOTE:
		return rvs.Prevotes

	// PRECOMMIT set
	case ktypes.PRECOMMIT:
		return rvs.Precommits

	// Unknown type
	default:
		log.Panicln("invalid vote type")
	}

	return nil
}

// getPrevotes is a method of HeightVoteSet that retrieves the prevotes from the set for a given round value.
func (hvs *HeightVoteSet) getPrevotes(round int32) *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the prevotes for the given round
	return hvs.getVoteSet(round, ktypes.PREVOTE)
}

// getPrecommits is a method of HeightVoteSet that retrieves the precommits from the set for a given round value.
func (hvs *HeightVoteSet) getPrecommits(round int32) *VoteSet {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Return the precommits for the given round
	return hvs.getVoteSet(round, ktypes.PRECOMMIT)
}

// POLInfo is a method of HeightVoteSet that retrieves the last round with a Proof of Lock.
// Returns the round number and the tesseract grid id with the lock.
func (hvs *HeightVoteSet) POLInfo() (int32, *types.TesseractGridID) {
	// Acquire lock
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	// Check all rounds going back from current round
	for r := hvs.round; r >= 0; r-- {
		// Get the PREVOTE's for the round
		rvs := hvs.getVoteSet(r, ktypes.PREVOTE)

		// If 2/3 majority exists for round, return the grid id and the round value
		gridid, ok := rvs.TwoThirdMajority()
		if ok {
			return r, gridid
		}
	}

	// If no round has a proof of lock, return empty
	return -1, nil
}
