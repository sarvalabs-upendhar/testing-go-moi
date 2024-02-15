package kbft

import (
	"bytes"
	"errors"
	"log"
	"sync"

	"github.com/hashicorp/go-hclog"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
)

// VoteSet is a struct that represents a set of consensus Votes
type VoteSet struct {
	logger hclog.Logger

	// Represent the height for which the vote-set applies
	heights map[identifiers.Address]uint64

	// Represents the round for which the vote-set applies
	round int32

	// Represents the type consensus vote for the vote-set
	votetype ktypes.ConsensusMsgType

	// Represents a set of validators
	valset *ktypes.ClusterState

	// Represents an access lock on the vote-set
	mtx sync.RWMutex

	// Represents the sum of seen votes, discounting conflicts
	sum []int32

	// Represents the sum of voting power for seen votes
	votingPowerSum []int32

	// Represents the votes in the vote-set
	votes []*ktypes.Vote

	// Represents an array of bits. Each index of the array corresponds to a validator and
	// the value at that index represents whether a vote for the validator exists in the set
	votesBitArray *common.ArrayOfBits

	// Represents the tesseractVoteSets in the vote-set. The
	votesByTesseract map[string]*tesseractVoteSet // string(blockHash|blockParts) -> tesseractVotes

	// Represents the Tesseract voted for by atleast 2/3rds
	maj23 *common.Hash

	// Represents a mapping of peers to their maj23s
	peermaj23s map[string]common.Hash
}

// NewVoteSet is a constructor function that generates and returns a new VoteSet.
// Accepts a slice of heights, the vote round, the type of votes in the set and a set of validators for the VoteSet.
func NewVoteSet(
	heights map[identifiers.Address]uint64,
	round int32,
	voteType ktypes.ConsensusMsgType,
	validatorSet *ktypes.ClusterState,
	logger hclog.Logger,
) *VoteSet {
	// Log the creation and the set of validators
	logger.Info("Creating new vote set with validators.", "size", validatorSet.Size())

	return &VoteSet{
		logger:           logger.Named("Votes-Set"),
		heights:          heights,
		round:            round,
		votetype:         voteType,
		valset:           validatorSet,
		mtx:              sync.RWMutex{},
		votes:            make([]*ktypes.Vote, validatorSet.Size()),
		votingPowerSum:   make([]int32, 3),
		votesBitArray:    common.NewArrayOfBits(validatorSet.Size()),
		votesByTesseract: make(map[string]*tesseractVoteSet, validatorSet.Size()),
		sum:              make([]int32, 3),
		peermaj23s:       make(map[string]common.Hash),
	}
}

// getVote is a method of voteset that retrieves a particular vote from the set.
// Accepts a validator index as an int32 and a tesseract hash
// Returns the Vote and a bool indicating the success status of the fetch.
func (vs *VoteSet) getVote(valIndex int32, tsHash common.Hash) (vote *ktypes.Vote, ok bool) {
	// Attempt to retrieve the vote from the slice of votes
	// Return the vote if its ts hash matches the given hash.
	if existingVote := vs.votes[valIndex]; existingVote != nil && existingVote.TSHash == tsHash {
		return existingVote, true
	}

	// Attempt to retrieve the vote from the tesseractVote map using the ts hash as
	// the key and then the index from that with the index and return it if found.
	if existingVote := vs.votesByTesseract[string(tsHash.Bytes())].getByIndex(valIndex); existingVote != nil {
		return existingVote, true
	}

	// No vote found
	return nil, false
}

// HasMajority is a method of VoteSet that returns whether the set of votes has resulted in a majority
func (vs *VoteSet) HasMajority() bool {
	// No majority if vote-set is null
	if vs == nil {
		return false
	}

	// Acquire lock
	vs.mtx.RLock()
	defer vs.mtx.RUnlock()

	return vs.maj23 != nil
}

// HasMajorityAny is a method of VoteSet that return whether any of the sum indexes represents a majority
func (vs *VoteSet) HasMajorityAny() bool {
	// No majority if vote-set is null
	if vs == nil {
		return false
	}

	// Acquire lock
	vs.mtx.RLock()
	defer vs.mtx.RUnlock()

	for index, sumVal := range vs.sum {
		// If sumVal for a given index has 2/3 majority, return true
		if sumVal < vs.valset.GetQuorum()[index] {
			return false
		}
	}

	// No 2/3 majority found
	return true
}

// TwoThirdMajority is a method of VoteSet that returns a TesseractHash that 2/3 majority has agreed
// on and a boolean reflecting if that majority has been reached in the first place.
func (vs *VoteSet) TwoThirdMajority() (hash common.Hash, ok bool) {
	// No majority if vote-set is null
	if vs == nil {
		return common.NilHash, false
	}

	// Acquire lock
	vs.mtx.RLock()
	defer vs.mtx.RUnlock()

	if vs.maj23 != nil {
		// Return the ts hash agreed on by the majority of votes
		return *vs.maj23, true
	}

	// No majority
	return common.NilHash, false
}

// AddVote is a method of VoteSet that adds a vote to the set.
// The vote and the validator who placed the vote are verified by checking the vote specs,
// signatures and addresses and then added to the set using the addVerifiedVote method.
func (vs *VoteSet) AddVote(v *ktypes.Vote) (added bool, err error) {
	// Acquire lock
	vs.mtx.Lock()
	defer vs.mtx.Unlock()

	if v == nil {
		// Empty votes are invalid
		return false, errors.New("invalid vote")
	}

	// Retrieve the validator index and address
	tesseractGroupID := v.TSHash

	valIndex := v.ValidatorIndex
	if valIndex < 0 {
		return false, errors.New("invalid validator details ")
	}

	// Check that heights and round match for the vote and voteset
	if !areVoteHeightsEqual(v.Heights, vs.heights) || (v.Round != vs.round) || (v.Type != vs.votetype) {
		return false, errors.New("invalid round and height details")
	}

	// Retrieve the validator from the validator set
	valKramaID, publicKey := vs.valset.GetByIndex(valIndex)
	if valKramaID == "" {
		return false, errors.New("invalid validator")
	}

	// Validate that the validator address matches
	if v.ValidatorKramaID != valKramaID {
		return false, errors.New("validator address doesn't match with the validator set of the vote-set")
	}

	// Check if the vote already exists in the set
	if exists, ok := vs.getVote(valIndex, tesseractGroupID); ok {
		if bytes.Equal(exists.Signature, v.Signature) {
			return false, nil
		}

		return false, errors.New("vote for validator with different signature already exists")
	}

	rawData, err := v.SignBytes()
	if err != nil {
		return false, err
	}

	verified, err := crypto.Verify(rawData, v.Signature, publicKey)
	if err != nil {
		return false, err
	}

	if !verified {
		return false, common.ErrSignatureVerificationFailed
	}

	// Add the verified vote to the vote set
	added, conflicting := vs.addVerifiedVote(v, tesseractGroupID, 1)
	if conflicting != nil {
		return added, common.ErrConflictingVote
	}

	if !added {
		log.Panicln("expected to add non-conflicting vote")
	}

	return added, nil
}

// getVoteByIndex retrieves the vote at the specified index in the vote-set.
func (vs *VoteSet) getVoteByIndex(index int) (*ktypes.Vote, error) {
	if index < 0 || index >= len(vs.votes) {
		return nil, errors.New("index out of bound")
	}

	return vs.votes[index], nil
}

// GetVoteBits returns the array of bits representing the presence of votes for each validator in the vote-set.
func (vs *VoteSet) GetVoteBits() *common.ArrayOfBits {
	vs.mtx.RLock()
	defer vs.mtx.RUnlock()

	return vs.votesBitArray.Copy()
}

func (vs *VoteSet) addVerifiedVote(
	vote *ktypes.Vote,
	tsHash common.Hash,
	votePower int32,
) (added bool, conflicting *ktypes.Vote) {
	// Fetch the index of the validator placing the vote and the sum index for that validator
	valIndex := vote.ValidatorIndex

	sumIndex, err := vs.getSumIndex(valIndex)
	if err != nil {
		log.Fatal(err)
	}

	// Check if the vote already exists in the set of votes
	if existingVote := vs.votes[valIndex]; existingVote != nil {
		// Panic if the ts hash match for the existing vote and new vote
		if bytes.Equal(existingVote.TSHash.Bytes(), tsHash.Bytes()) {
			return true, existingVote
		} else {
			// Set the conflicting vote
			conflicting = existingVote
		}

		// Check if tesseract hash of vote matches voteset's majority
		if (vs.maj23 != nil) && *vs.maj23 == tsHash {
			// Add the vote to the set and update the bit array to reflect that the vote for the validator exists
			vs.votes[valIndex] = vote
			vs.votesBitArray.SetIndex(int(valIndex), true)
		}
	} else {
		// Add the unseen vote to the set and update the bit array to reflect that the vote for the validator exists
		vs.votes[valIndex] = vote

		vs.votesBitArray.SetIndex(int(valIndex), true)
		// Update the sum of the voteset for the validator
		vs.updateSum(valIndex, votePower)
	}

	// Get the tesseract vote set for the tesseract hash
	tesseractVotes, ok := vs.votesByTesseract[string(tsHash.Bytes())]
	// If the tesseract vote set exists, and there is a conflicting vote while the tesseract vote has no maj23, return
	if ok {
		if conflicting != nil && !tesseractVotes.peermaj23 {
			return false, conflicting
		}
	} else {
		// Return the conflict vote if there is one
		if conflicting != nil {
			return false, conflicting
		}

		// Create a new tesseract vote set and add it to the vote set to start tracking the tesseract
		tesseractVotes = newTesseractVoteSet(len(vs.sum), false, vs.valset.Size())
		vs.votesByTesseract[string(tsHash.Bytes())] = tesseractVotes
	}

	// Get the voting powers of the validators
	quorum := vs.valset.GetQuorum()

	// Get the sum set from the tesseract votes. Add the vote and then get the new sum
	// prevotesum := tesseractVotes.sum
	tesseractVotes.addVerifiedVote(sumIndex, vote, votePower)
	postVoteSum := tesseractVotes.sum

	vs.logger.Debug("### Printing quorum ###", "quorum", quorum, "ts-hash", tsHash.Hex(), "sum", postVoteSum)

	if vs.maj23 == nil {
		// Check if the quorum threshold was just crossed. Only the first quorum reach is considered
		if areGreater(postVoteSum, quorum) {
			// Assign the tesseract hash that 2/3 validators have agreed on.
			vs.maj23 = &vote.TSHash
			// Add the votes from the tesseract votes into the vote-set
			for i, vote := range tesseractVotes.votes {
				if vote != nil {
					vs.votes[i] = vote
				}
			}
		}
	}

	// Return the add confirmation and any conflict if it occurred
	return true, conflicting
}

// getSumIndex is a method of VoteSet that retrieves the index on the sum set for a given validator index
func (vs *VoteSet) getSumIndex(valIndex int32) ([]int32, error) {
	slotsSum := make([]int32, 0, 3)

	slots, _, _, _ := vs.valset.NodeSet.GetKramaID(valIndex)

	if slots == nil {
		return nil, errors.New("invalid validator index")
	}

	for _, slot := range slots {
		slotsSum = append(slotsSum, int32(slot/2))
	}

	return slotsSum, nil
}

// updateSum is a method of VoteSet that updates the sum value at a given validator index by a given vote power value
func (vs *VoteSet) updateSum(valIndex int32, vp int32) {
	indexes, err := vs.getSumIndex(valIndex)
	if err != nil {
		return
	}

	for _, index := range indexes {
		vs.sum[index] += 1
		vs.votingPowerSum[index] += vp
	}
}

// RoundVoteSet is a struct that represents a set for a votes for a single round.
type RoundVoteSet struct {
	// Represents the pre-votes for the round
	Prevotes *VoteSet

	// Represents the pre-commits for the round
	Precommits *VoteSet
}

func (rvs RoundVoteSet) VoteBitSet() *ktypes.VoteBitSet {
	v := new(ktypes.VoteBitSet)

	if rvs.Prevotes.GetVoteBits().IsEmpty() {
		return nil
	}

	v.Prevotes = rvs.Prevotes.GetVoteBits()

	if !rvs.Precommits.GetVoteBits().IsEmpty() {
		v.Precommits = rvs.Precommits.GetVoteBits()
	}

	return v
}
