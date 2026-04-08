package state

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

func (so *SystemObject) Validators() []*common.Validator {
	return so.vals
}

func (so *SystemObject) Validator(index uint64) (*common.Validator, error) {
	if index >= uint64(len(so.vals)) {
		return nil, errors.New("index validator index")
	}

	return so.vals[index], nil
}

func (so *SystemObject) ValidatorByKramaID(id identifiers.KramaID) (*common.Validator, error) {
	for _, val := range so.Validators() {
		if val.KramaID == id {
			return val, nil
		}
	}

	return nil, common.ErrKramaIDNotFound
}

func (so *SystemObject) TotalValidators() uint64 {
	return so.totalValidators
}

func (so *SystemObject) GetValidatorPublicKeys(kramaIDs []identifiers.KramaID) ([][]byte, error) {
	pubkeys := make([][]byte, 0, len(kramaIDs))
	list := make(map[identifiers.KramaID][]byte)

	for _, val := range so.Validators() {
		list[val.KramaID] = val.ConsensusPubKey
	}

	for _, id := range kramaIDs {
		pubkey, ok := list[id]
		if !ok {
			return nil, common.ErrKramaIDNotFound
		}

		pubkeys = append(pubkeys, pubkey)
	}

	if len(pubkeys) != len(kramaIDs) {
		return nil, common.ErrKramaIDNotFound
	}

	return pubkeys, nil
}

func (so *SystemObject) GetValidatorsByKramaID(kramaIDs []identifiers.KramaID) ([]*common.ValidatorInfo, error) {
	vals := make([]*common.ValidatorInfo, 0, len(kramaIDs))
	list := make(map[identifiers.KramaID]*common.ValidatorInfo)

	for _, val := range so.Validators() {
		list[val.KramaID] = &common.ValidatorInfo{
			ID:          val.ID,
			KramaID:     val.KramaID,
			PublicKey:   val.ConsensusPubKey,
			VotingPower: val.VotingPower(),
		}
	}

	for _, id := range kramaIDs {
		val, ok := list[id]
		if !ok {
			return nil, common.ErrKramaIDNotFound
		}

		vals = append(vals, val)
	}

	if len(vals) != len(kramaIDs) {
		return nil, common.ErrKramaIDNotFound
	}

	return vals, nil
}

func (so *SystemObject) GetValidatorKramaIDs(indices []common.ValidatorIndex) ([]identifiers.KramaID, error) {
	ids := make([]identifiers.KramaID, 0, len(indices))

	for _, index := range indices {
		val, err := so.Validator(uint64(index))
		if err != nil {
			return nil, err
		}

		ids = append(ids, val.KramaID)
	}

	return ids, nil
}

func (so *SystemObject) GetValidators(indices ...common.ValidatorIndex) ([]*common.ValidatorInfo, error) {
	ids := make([]*common.ValidatorInfo, 0, len(indices))

	for _, index := range indices {
		val, err := so.Validator(uint64(index))
		if err != nil {
			return nil, err
		}

		ids = append(ids, &common.ValidatorInfo{
			ID:          val.ID,
			KramaID:     val.KramaID,
			PublicKey:   val.ConsensusPubKey,
			VotingPower: val.VotingPower(),
		})
	}

	return ids, nil
}

func (so *SystemObject) GetActiveValidatorIndices(minimumStake *big.Int) ([]uint64, error) {
	indices := make([]uint64, 0, len(so.vals))

	for i, val := range so.vals {
		if val.IsActive(minimumStake) {
			indices = append(indices, uint64(common.ValidatorIndex(i)))
		}
	}

	return indices, nil
}
