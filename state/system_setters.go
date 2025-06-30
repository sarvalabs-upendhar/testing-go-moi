package state

import (
	"time"

	"github.com/sarvalabs/go-moi/common"
)

func (so *SystemObject) SetGenesisTime(genesisTime time.Time) error {
	so.genesisTime = genesisTime
	so.markFieldAsDirty(fieldGenesisTime)

	return nil
}

func (so *SystemObject) SetValidators(vals []*common.Validator) error {
	so.vals = vals
	so.totalValidators = uint64(len(so.vals))
	so.markFieldAsDirty(fieldValidators)
	so.markFieldAsDirty(fieldTotalValidators)
	so.markIndexAsDirty(fieldValidators, func() []uint64 {
		idxs := make([]uint64, so.totalValidators)

		for idx := range idxs {
			idxs[idx] = uint64(idx)
		}

		return idxs
	}()...)

	return nil
}

func (so *SystemObject) AppendValidator(val *common.Validator) error {
	vs := so.vals
	if ref := so.sharedFieldReferences[fieldValidators]; ref.Refs() > 1 {
		vs = so.copyValidators()

		ref.MinusRef()

		so.sharedFieldReferences[fieldValidators] = &reference{refs: 1}
	}

	vs = append(vs, val)
	so.vals = vs

	so.totalValidators++
	so.markFieldAsDirty(fieldValidators)
	so.markFieldAsDirty(fieldTotalValidators)
	so.markIndexAsDirty(fieldValidators, so.totalValidators-1)

	return nil
}

func (so *SystemObject) UpdateValidator(index uint64, val *common.Validator) error {
	vs := so.vals

	if ref := so.sharedFieldReferences[fieldValidators]; ref.Refs() > 1 {
		vs = so.copyValidators()

		ref.MinusRef()

		so.sharedFieldReferences[fieldValidators] = &reference{refs: 1}
	}

	vs[index] = val
	so.vals = vs

	so.markFieldAsDirty(fieldValidators)
	so.markIndexAsDirty(fieldValidators, index)

	return nil
}

func (so *SystemObject) UpdateForEveryValidator(f func(idx int, val *common.Validator) (bool, error)) error {
	vs := so.vals

	if ref := so.sharedFieldReferences[fieldValidators]; ref.Refs() > 1 {
		vs = so.copyValidators()

		ref.MinusRef()

		so.sharedFieldReferences[fieldValidators] = &reference{refs: 1}
	}

	var modifiedValIndices []uint64

	for i, val := range vs {
		changed, err := f(i, val)
		if err != nil {
			return err
		}

		if changed {
			modifiedValIndices = append(modifiedValIndices, uint64(i))
		}
	}

	so.vals = vs

	so.markFieldAsDirty(fieldValidators)
	so.markIndexAsDirty(fieldValidators, modifiedValIndices...)

	return nil
}
