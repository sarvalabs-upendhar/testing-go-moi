package kbft

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
)

type EvidenceEngine struct {
	//mtx sync.Mutex

	evidences map[ktypes.ClusterID]*Evidence
}

func NewEvidenceEngine() *EvidenceEngine {
	e := &EvidenceEngine{
		evidences: make(map[ktypes.ClusterID]*Evidence),
	}

	return e
}

type Evidence struct {
	IxHash   ktypes.Hash
	Operator id.KramaID
	Votes    []*ktypes.Vote
	VoteSet  *ktypes.ArrayOfBits
}

func NewEvidence(IxHash ktypes.Hash, Operator id.KramaID, size int) *Evidence {
	evidenceInstance := &Evidence{
		IxHash:   IxHash,
		Operator: Operator,
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

func (e *Evidence) FlushEvidence() (ktypes.Hash, []byte) {
	rawData := e.Bytes()

	return ktypes.GetHash(rawData), rawData
}

func (e *Evidence) AddVoteSet(bitArray *ktypes.ArrayOfBits) {
	e.VoteSet = bitArray
}
