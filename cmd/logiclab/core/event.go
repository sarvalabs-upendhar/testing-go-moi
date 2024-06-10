package core

import (
	"fmt"

	"github.com/sarvalabs/go-moi/compute"

	"github.com/sarvalabs/go-polo"

	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"
)

const (
	MaxHead           = 10000
	EventStorageLimit = 1000
)

type EventDB struct {
	head uint64
	size uint64
}

type Event struct {
	IxHash  common.Hash         `json:"ixhash"`
	Address identifiers.Address `json:"address"`
	LogicID identifiers.LogicID `json:"logicID"`
	Topics  []common.Hash       `json:"topics"`
	Data    string              `json:"data"`
}

func (e *Event) Encode() ([]byte, error) {
	rawData, err := polo.Polorize(e)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize event")
	}

	return rawData, nil
}

func (e *Event) Decode(bytes []byte) error {
	if err := polo.Depolorize(e, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize event")
	}

	return nil
}

func GetEventsFromStream(stream *compute.EventStream, ixhash common.Hash) []Event {
	logs := stream.GetAsLogs()
	events := make([]Event, 0)

	for _, log := range logs {
		event := Event{
			IxHash:  ixhash,
			Address: log.Address,
			LogicID: log.LogicID,
			Topics:  log.Topics,
			Data:    "0x" + common.BytesToHex(log.Data),
		}

		events = append(events, event)
	}

	return events
}

// ReorganizeEvents reorganizes events when the head reaches MaxHead
func (env *Environment) reorganizeEvents() error {
	for idx := uint64(0); idx < MaxHead; idx++ {
		oldPos := idx + MaxHead

		value, err := env.database.Get(db.EventKey(env.ID, oldPos))
		if err != nil {
			return fmt.Errorf("failed to get event value at position %d: %w", oldPos, err)
		}

		err = env.database.Set(db.EventKey(env.ID, idx), value)
		if err != nil {
			return fmt.Errorf("failed to set event value at new position %d: %w", idx, err)
		}
	}

	env.eventDB.head = 0

	return nil
}

// InsertEvent adds events to the eventDB, managing the circular buffer
func (env *Environment) InsertEvent(events []Event) error {
	head, size := env.eventDB.head, env.eventDB.size
	for _, event := range events {
		size++

		value, err := event.Encode()
		if err != nil {
			return fmt.Errorf("failed to encode event: %w", err)
		}

		pos := head + size - 1
		key := db.EventKey(env.ID, pos)
		err = env.database.Set(key, value)

		if err != nil {
			return fmt.Errorf("failed to set event at position %d: %w", pos, err)
		}
	}

	// Check if the size is greater than the event storage limit
	if size > EventStorageLimit {
		excess := size - EventStorageLimit

		// Delete excess events
		for idx := uint64(0); idx < excess; idx++ {
			if err := env.database.Del(db.EventKey(env.ID, idx)); err != nil {
				return fmt.Errorf("failed to delete event at position %d: %w", idx, err)
			}
		}

		// Update the eventDB head to reflect the new starting point
		env.eventDB.head = excess - 1

		// Check if the new head has reached the maximum allowed value
		if env.eventDB.head == MaxHead {
			// If the head is at its maximum, reorganize events to prevent overflow
			if err := env.reorganizeEvents(); err != nil {
				return err
			}
		}

		size = EventStorageLimit
	}

	// Update the size in the eventDB
	env.eventDB.size = size

	return nil
}
