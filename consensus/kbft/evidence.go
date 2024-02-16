package kbft

import (
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

type EvidenceEngine struct {
	// mtx sync.Mutex

	evidences map[common.ClusterID]*Evidence
}

func NewEvidenceEngine() *EvidenceEngine {
	e := &EvidenceEngine{
		evidences: make(map[common.ClusterID]*Evidence),
	}

	return e
}

type Evidence struct {
	IxHash   common.Hash
	Operator kramaid.KramaID
	Votes    []*ktypes.Vote
	VoteSet  *common.ArrayOfBits
}

func NewEvidence(ixHash common.Hash, operator kramaid.KramaID, size int) *Evidence {
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

func (e *Evidence) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(e)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize evidence")
	}

	return rawData, nil
}

func (e *Evidence) FlushEvidence() (common.Hash, []byte, error) {
	rawData, err := e.Bytes()
	if err != nil {
		return common.NilHash, nil, err
	}

	return common.GetHash(rawData), rawData, nil
}

func (e *Evidence) AddVoteSet(bitArray *common.ArrayOfBits) {
	e.VoteSet = bitArray
}
