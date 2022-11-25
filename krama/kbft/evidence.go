package kbft

import (
	"github.com/sarvalabs/go-polo"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type EvidenceEngine struct {
	// mtx sync.Mutex

	evidences map[types.ClusterID]*Evidence
}

func NewEvidenceEngine() *EvidenceEngine {
	e := &EvidenceEngine{
		evidences: make(map[types.ClusterID]*Evidence),
	}

	return e
}

type Evidence struct {
	IxHash   types.Hash
	Operator id.KramaID
	Votes    []*ktypes.Vote
	VoteSet  *types.ArrayOfBits
}

func NewEvidence(ixHash types.Hash, operator id.KramaID, size int) *Evidence {
	evidenceInstance := &Evidence{
		IxHash:   ixHash,
		Operator: operator,
		Votes:    make([]*ktypes.Vote, size),
	}

	return evidenceInstance
}

func (e *Evidence) AddVote(v *ktypes.Vote) {
	e.Votes = append(e.Votes, v)
}

func (e *Evidence) Bytes() []byte {
	return polo.Polorize(e)
}

func (e *Evidence) FlushEvidence() (types.Hash, []byte) {
	rawData := e.Bytes()

	return types.GetHash(rawData), rawData
}

func (e *Evidence) AddVoteSet(bitArray *types.ArrayOfBits) {
	e.VoteSet = bitArray
}
