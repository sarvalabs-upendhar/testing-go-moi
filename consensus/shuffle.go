package consensus

import (
	eth2_shuffle "github.com/protolambda/eth2-shuffle"
	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"golang.org/x/crypto/blake2b"
)

const ShufflingRounds = 90

func HashFn(data []byte) []byte {
	x := blake2b.Sum256(data)

	return x[:]
}

func (k *Engine) ShuffledList(
	cs *ktypes.ClusterState,
	exemptedNodes map[common.ValidatorIndex]struct{},
) ([]common.ValidatorIndex, error) {
	// TODO: should we exemptNodes ?
	// TODO: avoid conversion from uint64 to ValidatorIndex
	activeValIndices, err := cs.SystemObject.GetActiveValidatorIndices(k.minimumStake)
	if err != nil {
		return nil, err
	}

	eth2_shuffle.UnshuffleList(HashFn, activeValIndices, ShufflingRounds, cs.GetSeed())

	return ValidatorIndicesList(activeValIndices), nil
}

func ValidatorIndicesList(ls []uint64) []common.ValidatorIndex {
	indices := make([]common.ValidatorIndex, len(ls))
	for i, v := range ls {
		indices[i] = common.ValidatorIndex(v)
	}

	return indices
}
