package state

import (
	"encoding/binary"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-polo"
)

const TotalFieldCount = 2

type (
	fieldIndex int
	reference  struct {
		refs uint
		lock sync.RWMutex
	}
)

func (r *reference) Refs() uint {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.refs
}

func (r *reference) AddRef() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.refs++
}

func (r *reference) MinusRef() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.refs > 0 {
		r.refs--
	}
}

const (
	fieldTotalValidators fieldIndex = iota
	fieldGenesisTime
	fieldValidators
)

var ValidatorPrefix = []byte("val")

var (
	GenesisTimeKey     = []byte("genesis_time")
	TotalValidatorsKey = []byte("total_validators")
)

func ValidatorKey(index uint64) []byte {
	bytes := make([]byte, 11)

	copy(bytes[0:3], ValidatorPrefix)
	binary.BigEndian.PutUint64(bytes[3:], index)

	return bytes
}

/*
Keys:
TotalValidators : "total_validators"
Validator: "validator_{index}
*/

type TotalValidators uint64

func Uint64FromBytes(input []byte) uint64 {
	out, _ := binary.Uvarint(input)

	return out
}

func Uint64ToBytes(input uint64) []byte {
	rawBytes := make([]byte, 8)
	binary.PutUvarint(rawBytes, input)

	return rawBytes
}

func Int64ToBytes(input int64) []byte {
	rawBytes := make([]byte, 8)
	binary.PutVarint(rawBytes, input)

	return rawBytes
}

func Int64FromBytes(input []byte) int64 {
	out, _ := binary.Varint(input)

	return out
}

type SystemRegistry struct {
	mtx          sync.RWMutex
	systemObject *SystemObject
}

func NewSystemObjectRegistry() *SystemRegistry {
	return &SystemRegistry{
		mtx:          sync.RWMutex{},
		systemObject: nil,
	}
}

func (sor *SystemRegistry) GetSystemObject() *SystemObject {
	sor.mtx.RLock()
	defer sor.mtx.RUnlock()

	return sor.systemObject
}

func (sor *SystemRegistry) SetSystemObject(so *SystemObject) {
	sor.mtx.Lock()
	defer sor.mtx.Unlock()

	sor.systemObject = so
}

type SystemObject struct {
	totalValidators uint64
	genesisTime     time.Time
	vals            []*common.Validator

	dirtyFields           map[fieldIndex]interface{}
	dirtyIndices          map[fieldIndex][]uint64
	sharedFieldReferences map[fieldIndex]*reference
	*Object
}

func NewSystemObject(obj *Object) *SystemObject {
	return &SystemObject{
		vals:         make([]*common.Validator, 0),
		dirtyFields:  make(map[fieldIndex]interface{}),
		dirtyIndices: make(map[fieldIndex][]uint64),
		sharedFieldReferences: map[fieldIndex]*reference{
			fieldValidators: {
				lock: sync.RWMutex{},
				refs: 1,
			},
		},
		Object: obj,
	}
}

func (so *SystemObject) Init() error {
	if err := so.loadGenesisTime(); err != nil {
		return err
	}

	if err := so.loadValidators(); err != nil {
		return err
	}

	return nil
}

func (so *SystemObject) copyValidators() []*common.Validator {
	copiedVals := make([]*common.Validator, len(so.vals))
	for i, val := range so.vals {
		copiedVals[i] = val.Copy()
	}

	return copiedVals
}

func (so *SystemObject) markFieldAsDirty(field fieldIndex) {
	_, ok := so.dirtyFields[field]
	if !ok {
		so.dirtyFields[field] = true
	}
}

func (so *SystemObject) markIndexAsDirty(field fieldIndex, index ...uint64) {
	_, ok := so.dirtyIndices[field]
	if !ok {
		so.dirtyIndices[field] = make([]uint64, 0)
	}

	so.dirtyIndices[field] = append(so.dirtyIndices[field], index...)
}

func (so *SystemObject) Copy() *SystemObject {
	newObj := &SystemObject{
		totalValidators: so.totalValidators,
		genesisTime:     so.genesisTime,

		// copy on write fields
		vals: so.vals,

		dirtyFields:           make(map[fieldIndex]interface{}, TotalFieldCount),
		dirtyIndices:          make(map[fieldIndex][]uint64, TotalFieldCount),
		sharedFieldReferences: make(map[fieldIndex]*reference),
	}

	for field, ref := range so.sharedFieldReferences {
		ref.AddRef()
		newObj.sharedFieldReferences[field] = ref
	}

	for i := range so.dirtyFields {
		newObj.dirtyFields[i] = true
	}

	for i, indices := range so.dirtyIndices {
		newObj.dirtyIndices[i] = make([]uint64, len(indices))
		copy(newObj.dirtyIndices[i], indices)
	}

	runtime.SetFinalizer(newObj, func(b *SystemObject) {
		for _, v := range b.sharedFieldReferences {
			v.MinusRef()
		}
	})

	so.Object = so.Object.Copy()

	return newObj
}

func (so *SystemObject) Commit() (common.Hash, error) {
	for field := range so.dirtyFields {
		switch field {
		case fieldTotalValidators:
			err := so.SetStorageEntry(common.SystemLogicID, TotalValidatorsKey, Uint64ToBytes(so.totalValidators))
			if err != nil {
				return common.NilHash, err
			}
		case fieldGenesisTime:
			err := so.SetStorageEntry(common.SystemLogicID, GenesisTimeKey, Int64ToBytes(so.genesisTime.Unix()))
			if err != nil {
				return common.NilHash, err
			}
		case fieldValidators:
			if _, ok := so.dirtyIndices[fieldValidators]; !ok {
				return common.NilHash, errors.New("dirty indices doesn't exist")
			}

			for _, index := range so.dirtyIndices[fieldValidators] {
				data, err := so.vals[index].Bytes()
				if err != nil {
					return common.NilHash, err
				}

				err = so.SetStorageEntry(common.SystemLogicID, ValidatorKey(index), data)
				if err != nil {
					return common.NilHash, err
				}
			}
		}

		delete(so.dirtyFields, field)
	}

	return so.Object.Commit()
}

func (so *SystemObject) flush() error {
	return so.Object.flush()
}

func (so *SystemObject) loadValidators() error {
	rawBytes, err := so.GetStorageEntry(common.SystemLogicID, TotalValidatorsKey)
	if err != nil {
		return errors.Wrap(err, "failed to fetch total copyValidators key")
	}

	so.totalValidators = Uint64FromBytes(rawBytes)
	so.vals = make([]*common.Validator, so.totalValidators)

	for index := range so.totalValidators {
		val, err := so.getValidatorByIndex(index)
		if err != nil {
			return err
		}

		so.vals[index] = val
	}

	return nil
}

func (so *SystemObject) loadGenesisTime() error {
	rawBytes, err := so.GetStorageEntry(common.SystemLogicID, GenesisTimeKey)
	if err != nil {
		return errors.Wrap(err, "failed to fetch genesis time key")
	}

	so.genesisTime = time.Unix(Int64FromBytes(rawBytes), 0)

	return nil
}

func (so *SystemObject) getValidatorByIndex(index uint64) (*common.Validator, error) {
	rawVal, err := so.GetStorageEntry(common.SystemLogicID, ValidatorKey(index))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch validator %d", index)
	}

	val := new(common.Validator)

	if err = polo.Depolorize(val, rawVal); err != nil {
		return nil, errors.Wrapf(err, "failed to depolorize validator %d", index)
	}

	return val, nil
}
