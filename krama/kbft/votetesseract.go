package kbft

import (
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/mudra"
	"github.com/sarvalabs/moichain/types"
)

// tesseractVoteSet is a struct that represents a set of votes for a Tesseract
type tesseractVoteSet struct {
	// Represents whether the peer claims to have maj23
	peermaj23 bool
	// Represents an array of bits. Each index of the array corresponds to a validator and
	// the value at that index represents whether a vote for the validator exists in the set
	bitarray *types.ArrayOfBits
	// Represents the tesseract votes of each validator by index
	votes []*ktypes.Vote
	// Represents the sum of voting powers
	sum []int32

	votingPowerSum []int32
}

// newTesseractVoteSet is a constructor function that generates and returns a new set of Tesseract votes.
// Accepts the size of sum set, whether the peer has a maj23 and the number of the validators.
func newTesseractVoteSet(size int, peermaj23 bool, valcount int) *tesseractVoteSet {
	return &tesseractVoteSet{
		peermaj23:      peermaj23,
		bitarray:       types.NewArrayOfBits(valcount),
		votes:          make([]*ktypes.Vote, valcount),
		sum:            make([]int32, size),
		votingPowerSum: make([]int32, size),
	}
}

// getByIndex is a method of tesseractVoteSet that retrieves a vote from the set for a given index.
func (tv *tesseractVoteSet) getByIndex(index int32) *ktypes.Vote {
	// Return nil if voteset is empty
	if tv == nil {
		return nil
	}

	// Retrieve the vote and return it
	return tv.votes[index]
}

// addVerifiedVote is a method of tesseractVoteSet that adds a verified vote to the set.
// Accepts the sum index, the vote and the voting power of the validator placing the vote.
func (tv *tesseractVoteSet) addVerifiedVote(sumIndexs []int32, vote *ktypes.Vote, votingpower int32) {
	// Fetch the index of the validator placing the vote
	valindex := vote.ValidatorIndex

	// Check if the vote already exists in the set
	if existingvote := tv.getByIndex(valindex); existingvote == nil {
		// Set the bitarray to reflect that the vote for the validator exists in the set
		tv.bitarray.SetIndex(int(valindex), true)
		tv.votes[valindex] = vote

		for _, index := range sumIndexs {
			tv.sum[index] += votingpower
			tv.votingPowerSum[index] += votingpower
		}
	}
}

func (tv *tesseractVoteSet) AggregateSignatures() ([]byte, error) {
	sigs := make([][]byte, 0, tv.bitarray.TrueIndicesSize())

	for _, index := range tv.bitarray.GetTrueIndices() {
		sigs = append(sigs, tv.votes[index].Signature)
	}

	return mudra.AggregateSignatures(sigs)
}
