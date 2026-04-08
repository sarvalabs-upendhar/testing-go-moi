package answer

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const (
	ActorAnswerSlot = 0
	LogicAnswerSlot = 0
)

type InputActorID struct {
	Identifier identifiers.Identifier `polo:"actorId"`
}

type InputActorAnswer struct {
	Identifier identifiers.Identifier `polo:"actorId"`
	Answer     uint64                 `polo:"new_answer"`
}

func GetActorAnswer(storage engineio.StorageReader) (uint64, error) {
	// Generate a storage access key
	key := pisa.GenerateStorageKey(ActorAnswerSlot)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return 0, err
	}

	var value uint64
	if err = polo.Depolorize(&value, val); err != nil {
		return 0, err
	}

	return value, nil
}

func GetLogicAnswer(storage engineio.StorageReader) (uint64, error) {
	// Generate a storage access key
	key := pisa.GenerateStorageKey(LogicAnswerSlot)

	// Retrieve the value for the storage key
	val, err := storage.ReadPersistentStorage(storage.Identifier(), key)
	if err != nil {
		return 0, err
	}

	var value uint64
	if err = polo.Depolorize(&value, val); err != nil {
		return 0, err
	}

	return value, nil
}
