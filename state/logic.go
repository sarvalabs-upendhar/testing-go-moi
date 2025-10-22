package state

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
)

// LogicObject is a generic container for representing an executable logic. It contains fields
// for representing the kind and metadata of the logic along with references to callable endpoints
// and all its logic elements. Implement the Logic interface defined in jug/types.
// Its fields are exported for to make it serializable.
type LogicObject struct {
	// Represents the Logic ID
	ID identifiers.Identifier
	// Represents the Logic Engine
	EngineKind engineio.EngineKind
	// Represents the hash of the Logic Manifest
	Manifest common.Hash

	Sealed   bool
	Artifact []byte
}

// NewLogicObject generates a new LogicObject for a given LogicID, LogicDescriptor and Storage Namespace key
func NewLogicObject(id identifiers.Identifier, descriptor engineio.LogicDescriptor) *LogicObject {
	return &LogicObject{
		ID:         id,
		EngineKind: descriptor.Engine,
		Manifest:   descriptor.ManifestHash,
		Artifact:   descriptor.Artifact,
		Sealed:     false,
	}
}

func (logic LogicObject) LogicID() identifiers.Identifier { return logic.ID }
func (logic LogicObject) Engine() engineio.EngineKind     { return logic.EngineKind }
func (logic LogicObject) ManifestHash() [32]byte          { return logic.Manifest }
func (logic LogicObject) IsSealed() bool                  { return logic.Sealed }

func (logic *LogicObject) Bytes() ([]byte, error) {
	data, err := polo.Polorize(logic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize logic object")
	}

	return data, err
}

func (logic *LogicObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(logic, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize logic object")
	}

	return nil
}

func GetManifestHashFromRawLogicObject(raw []byte) (common.Hash, error) {
	depolorizer, err := polo.NewDepolorizer(raw)
	if err != nil {
		return common.NilHash, err
	}

	depolorizer, err = depolorizer.Unpacked()
	if err != nil {
		return common.NilHash, err
	}

	if depolorizer.IsNull() {
		return common.NilHash, nil
	}

	// Skip the first field
	if _, err = depolorizer.DepolorizeAny(); err != nil {
		return common.NilHash, err
	}

	// Skip the second field
	if _, err = depolorizer.DepolorizeAny(); err != nil {
		return common.NilHash, err
	}

	var ManifestHash common.Hash
	if err = depolorizer.Depolorize(&ManifestHash); err != nil {
		return common.NilHash, err
	}

	return ManifestHash, nil
}

type LogicStorageObject struct {
	obj *Object
}

func NewLogicStorageObject(obj *Object) *LogicStorageObject {
	return &LogicStorageObject{
		obj: obj,
	}
}

func (lso *LogicStorageObject) Root() [32]byte { return lso.obj.Identifier() }

func (lso *LogicStorageObject) Identifier() [32]byte {
	return lso.obj.Identifier()
}

func (lso *LogicStorageObject) ReadPersistentStorage(logicID [32]byte, key [32]byte) ([]byte, error) {
	return lso.obj.GetStorageEntry(logicID, key[:])
}

func (lso *LogicStorageObject) WritePersistentStorage(logicID [32]byte, key [32]byte, value []byte) error {
	return lso.obj.SetStorageEntry(logicID, key[:], value)
}

func (lso *LogicStorageObject) DeletePersistentStorage(logicID [32]byte, key [32]byte) (uint64, error) {
	val, err := lso.obj.GetStorageEntry(logicID, key[:])
	if err != nil {
		return 0, err
	}

	if err = lso.obj.SetStorageEntry(logicID, key[:], nil); err != nil {
		return 0, err
	}

	return uint64(len(val)), nil
}

func (lso *LogicStorageObject) WriteTransientStorage(logicID [32]byte, key [32]byte, value []byte) error {
	// Transient storage is not supported in this implementation
	return common.ErrTransientStorageNotSupported
}

func (lso *LogicStorageObject) DeleteTransientStorage(logicID, key [32]byte) (uint64, error) {
	return 0, common.ErrTransientStorageNotSupported
}

func (lso *LogicStorageObject) ReadTransientStorage(logicID [32]byte, key [32]byte) ([]byte, error) {
	// Transient storage is not supported in this implementation
	return nil, common.ErrTransientStorageNotSupported
}

type Storage map[string][]byte

func (storage Storage) Copy() Storage {
	clone := make(Storage)

	for key, value := range storage {
		v := make([]byte, len(value))

		copy(v, value)
		clone[key] = v
	}

	return clone
}
