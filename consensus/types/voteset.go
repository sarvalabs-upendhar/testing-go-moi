package types

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
)

// VoteSet is a struct that represents a set of consensus Votes
type VoteSet struct {
	logger hclog.Logger

	// Represent the height for which the vote-set applies
	heights map[identifiers.Identifier]uint64

	// Represents the view for which the vote-set applies
	view uint64

	// Represents the type consensus vote for the vote-set
	votetype common.ConsensusMsgType

	// Represents a set of validators
	cs *ClusterState

	// Represents an access lock on the vote-set
	mtx sync.RWMutex

	// Represents the sum of seen votes, discounting conflicts
	sum []uint32

	// Represents the sum of voting power for seen votes
	votingPowerSum []int32

	// Represents the votes in the vote-set
	votes []*Vote

	// Represents an array of bits. Each index of the array corresponds to a validator and
	// the value at that index represents whether a vote for the validator exists in the set
	votesBitArray *common.ArrayOfBits

	// Represents the tesseractVoteSets in the vote-set. The
	votesByTesseract map[common.Hash]*tesseractVoteSet

	// Represents the ts voted for by atleast 2/3rds
	maj23TsHash *common.Hash

	maj23VoteSet *common.ArrayOfBits

	maj23aggSignature []byte
}

// NewVoteSet is a constructor function that generates and returns a new VoteSet.
// Accepts a slice of heights, the vote view, the type of votes in the set and a set of validators for the VoteSet.
func NewVoteSet(
	heights map[identifiers.Identifier]uint64,
	view uint64,
	voteType common.ConsensusMsgType,
	validatorSet *ClusterState,
	logger hclog.Logger,
) *VoteSet {
	// Log the creation and the set of validators
	logger.Info("Creating new vote set with validators.", "size", validatorSet.Size())

	return &VoteSet{
		logger:           logger.Named("Votes-Set"),
		heights:          heights,
		view:             view,
		votetype:         voteType,
		cs:               validatorSet,
		mtx:              sync.RWMutex{},
		votes:            make([]*Vote, validatorSet.Size()),
		votingPowerSum:   make([]int32, len(heights)+1),
		votesBitArray:    common.NewArrayOfBits(validatorSet.Size()),
		votesByTesseract: make(map[common.Hash]*tesseractVoteSet, validatorSet.Size()),
		sum:              make([]uint32, validatorSet.committee.Size()),
	}
}

// getVote is a method of voteset that retrieves a particular vote from the set.
// Accepts a validator index as an int32 and a tesseract hash
// Returns the Vote and a bool indicating the success status of the fetch.
func (vs *VoteSet) getVote(valIndex int32, tsHash common.Hash) (vote *Vote, ok bool) {
	// Attempt to retrieve the vote from the slice of votes
	// Return the vote if its ts hash matches the given hash.
	if existingVote := vs.votes[valIndex]; existingVote != nil && existingVote.TSHash == tsHash {
		return existingVote, true
	}

	// Attempt to retrieve the vote from the tesseractVote map using the ts hash as
	// the key and then the index from that with the index and return it if found.
	if existingVote := vs.votesByTesseract[tsHash].getByIndex(valIndex); existingVote != nil {
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

	return vs.maj23TsHash != nil
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
		if sumVal < vs.cs.GetQuorum()[index] {
			return false
		}
	}

	return true
}

// SuperMajority is a method of VoteSet that returns a TesseractHash that 2/3 majority has agreed
// on and a boolean reflecting if that majority has been reached in the first place.
func (vs *VoteSet) SuperMajority() (hash common.Hash, ok bool) {
	// No majority if vote-set is null
	if vs == nil {
		return common.NilHash, false
	}

	// Acquire lock
	vs.mtx.RLock()
	defer vs.mtx.RUnlock()

	if vs.maj23TsHash != nil {
		// Return the ts hash agreed on by the majority of votes
		return *vs.maj23TsHash, true
	}

	// No majority
	return common.NilHash, false
}

// AddVote is a method of VoteSet that adds a vote to the set.
// The vote and the validator who placed the vote are verified by checking the vote specs,
// signatures and addresses and then added to the set using the addVerifiedVote method.
func (vs *VoteSet) AddVote(v *Vote) (added bool, err error) {
	// Acquire lock
	vs.mtx.Lock()
	defer vs.mtx.Unlock()

	if v == nil {
		// Empty votes are invalid
		return false, errors.New("invalid vote")
	}

	// Retrieve the validator index and address
	tsHash := v.TSHash

	valIndex := v.SignerIndex
	if valIndex < 0 {
		return false, errors.New("invalid validator details ")
	}

	// Check if the ts hash and view match for the vote and voteset
	if (v.View != vs.view) || (v.Type != vs.votetype) {
		return false, errors.New("invalid view and ts hash details")
	}

	// Retrieve the validator from the validator set
	valKramaID, publicKey := vs.cs.GetByIndex(valIndex)
	if valKramaID == "" {
		return false, errors.New("invalid validator")
	}

	// Check if the vote already exists in the set
	if exists, ok := vs.getVote(valIndex, tsHash); ok {
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

	// Fetch the index of the validator placing the vote and the sum index for that validator
	sumIndex, err := vs.getSumIndex(valIndex)
	if err != nil {
		return false, err
	}

	added, conflicting := vs.addVerifiedVote(v, 1, valIndex, sumIndex, false)
	if conflicting != nil {
		return added, common.ErrConflictingVote
	}

	if !added {
		log.Panicln("expected to add non-conflicting vote")
	}

	return added, nil
}

func (vs *VoteSet) AddQc(v *Vote) (added bool, err error) {
	vs.mtx.Lock()
	defer vs.mtx.Unlock()

	if v == nil {
		// Empty votes are invalid
		return false, errors.New("invalid vote")
	}

	// Retrieve the validator index and address
	tsHash := v.TSHash

	leaderIndex := v.SignerIndex
	if leaderIndex < 0 {
		return false, errors.New("invalid validator details ")
	}

	if vs.maj23TsHash != nil && *vs.maj23TsHash != tsHash {
		return false, common.ErrConflictingVote
	}

	signedVals := v.SignerIndices.GetTrueIndices()
	publicKeys := make([][]byte, 0, len(signedVals))
	sumIndices := make([][]int32, 0, len(signedVals))

	fmt.Println("Signed Validators", signedVals)

	for _, valIndex := range signedVals {
		slots, _, kramaID, publicKey := vs.cs.Committee().GetKramaID(int32(valIndex))
		if kramaID == "" {
			vs.logger.Error("Validator public key not found in ICS", "val-index", valIndex)

			return false, common.ErrPublicKeyNotFound
		}

		publicKeys = append(publicKeys, publicKey)
		sumIndices = append(sumIndices, slots)
	}

	rawVote, err := v.SignBytes()
	if err != nil {
		return false, err
	}

	isvalid, err := crypto.VerifyAggregateSignature(rawVote, v.Signature, publicKeys)
	if !isvalid || err != nil {
		vs.logger.Error("failed to validate QC", "vote-type", v.Type, "error", err)

		return false, err
	}

	for index, valIndex := range signedVals {
		added, conflicting := vs.addVerifiedVote(v, 1, int32(valIndex), sumIndices[index], true)
		if !added || conflicting != nil {
			vs.logger.Error("failed to add verified vote", "val-index", valIndex)
		}
	}

	return true, nil
}

// getVoteByIndex retrieves the vote at the specified index in the vote-set.
func (vs *VoteSet) getVoteByIndex(index int) (*Vote, error) {
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
	vote *Vote,
	votePower int32,
	valIndex int32,
	sumIndex []int32,
	isQc bool,
) (added bool, conflicting *Vote) {
	// Check if the vote already exists in the set of votes
	if existingVote := vs.votes[valIndex]; existingVote != nil {
		if bytes.Equal(existingVote.TSHash.Bytes(), vote.TSHash.Bytes()) {
			return true, existingVote
		} else {
			// Set the conflicting vote
			conflicting = existingVote
		}

		// Check if tesseract hash of vote matches voteset's majority
		if (vs.maj23TsHash != nil) && *vs.maj23TsHash == vote.TSHash {
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
	tesseractVotes, ok := vs.votesByTesseract[vote.TSHash]
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
		tesseractVotes = newTesseractVoteSet(len(vs.sum), false, vs.cs.Size())
		vs.votesByTesseract[vote.TSHash] = tesseractVotes
	}

	// Get the voting powers of the validators
	quorum := vs.cs.GetQuorum()

	// Get the sum set from the tesseract votes. Add the vote and then get the new sum
	// prevotesum := tesseractVotes.sum
	tesseractVotes.addVerifiedVote(valIndex, sumIndex, vote, votePower)
	postVoteSum := tesseractVotes.sum

	vs.logger.Debug("### Printing quorum ###", "quorum", quorum, "ts-hash", vote.TSHash.Hex(), "sum", postVoteSum)

	if vs.maj23TsHash == nil {
		// Check if the quorum threshold was just crossed. Only the first quorum reach is considered
		if areGreater(postVoteSum, quorum) {
			var (
				aggSig []byte
				err    error
			)

			if !isQc {
				aggSig, err = tesseractVotes.AggregateSignatures()
				if err != nil {
					// This should never happen
					vs.logger.Error("failed to aggregate the Signature")
				}
			} else {
				aggSig = make([]byte, len(vote.Signature))
				copy(aggSig, vote.Signature)
			}

			// update the tsHash and aggregated signature
			maj23Ts := vote.TSHash
			vs.maj23TsHash = &maj23Ts
			vs.maj23aggSignature = aggSig
			vs.maj23VoteSet = tesseractVotes.bitarray.Copy()

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
	slots, _, _, _ := vs.cs.Committee().GetKramaID(valIndex)

	if slots == nil {
		return nil, errors.New("invalid validator index")
	}

	return slots, nil
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

func (vs *VoteSet) GetQC() (*common.ArrayOfBits, []byte) {
	return vs.votesBitArray, vs.maj23aggSignature
}

// ViewVoteSet is a struct that represents a set for a votes for a single view.
type ViewVoteSet struct {
	// Represents the pre-votes for the view
	Prevotes *VoteSet

	// Represents the pre-commits for the view
	Precommits *VoteSet
}

func (rvs ViewVoteSet) VoteBitSet() *VoteBitSet {
	v := new(VoteBitSet)

	if rvs.Prevotes.GetVoteBits().IsEmpty() {
		return nil
	}

	v.Prevotes = rvs.Prevotes.GetVoteBits()

	if !rvs.Precommits.GetVoteBits().IsEmpty() {
		v.Precommits = rvs.Precommits.GetVoteBits()
	}

	return v
}

// AreVoteHeightsEqual checks if two sets of heights are equal.
// Accepts two sets of heights and compares them. Returns a bool.
// if heights of respective addresses matches then true is returned.
func AreVoteHeightsEqual(
	voteHeights map[identifiers.Identifier]uint64,
	systemHeights map[identifiers.Identifier]uint64,
) bool {
	if len(voteHeights) != len(systemHeights) {
		return false
	}

	// Iterate over system heights
	for voteAddress, voteHeight := range voteHeights {
		systemHeight, ok := systemHeights[voteAddress]
		if !ok || voteHeight != systemHeight {
			// if system address not found or system heights are not equal, return false
			return false
		}
	}

	// Heights match, return true
	return true
}

func areGreater(oldValues, newValues []uint32) bool {
	// Iterate over system heights
	for idx, value := range oldValues {
		if value < newValues[idx] {
			// Height lesser, return false
			return false
		}
	}

	// All heights are greater, return true
	return true
}
