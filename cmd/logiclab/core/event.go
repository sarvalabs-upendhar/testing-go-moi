package core

import (
	"fmt"

	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/compute"

	"github.com/sarvalabs/go-polo"

	"github.com/pkg/errors"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"
)

const (
	MaxTopics         = 4
	MaxHead           = 10000
	EventStorageLimit = 1000
)

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
	events := make([]Event, 0)

	for log := range stream.Iterate() {
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

// ReOrgEvents reorganizes events when the head reaches MaxHead
func (env *Environment) ReOrgEvents() error {
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

	env.eventsHead = 0

	return nil
}

// InsertEvents adds events to the eventDB, managing the circular buffer
func (env *Environment) InsertEvents(events []Event) error {
	head, size := env.eventsHead, env.eventsSize
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
		env.eventsHead = excess - 1

		// Check if the new head has reached the maximum allowed value
		if env.eventsHead == MaxHead {
			// If the head is at its maximum, reorganize events to prevent overflow
			if err := env.ReOrgEvents(); err != nil {
				return err
			}
		}

		size = EventStorageLimit
	}

	// Update the size in the eventDB
	env.eventsSize = size

	return nil
}

func (env *Environment) IterEvents() <-chan struct {
	Event
	error
} {
	ch := make(chan struct {
		Event
		error
	})

	go func() {
		defer close(ch)

		head, size := env.eventsHead, env.eventsSize
		for idx := head; idx < size; idx++ {
			value, err := env.database.Get(db.EventKey(env.ID, idx))
			if err != nil {
				ch <- struct {
					Event
					error
				}{Event{}, err}

				return
			}

			event := new(Event)
			if err = polo.Depolorize(event, value); err != nil {
				ch <- struct {
					Event
					error
				}{Event{}, err}

				return
			}

			ch <- struct {
				Event
				error
			}{*event, nil}
		}
	}()

	return ch
}

func (env *Environment) GetEvents(filters ...EventFilter) ([]Event, error) {
	eventsCh := env.IterEvents()
	filteredEvents := make([]Event, 0)

	for event := range eventsCh {
		if event.error != nil {
			return nil, event.error
		}

		includeEvent := true

		for _, filter := range filters {
			if !filter(event.Event) {
				includeEvent = false

				break
			}
		}

		if includeEvent {
			filteredEvents = append(filteredEvents, event.Event)
		}
	}

	return filteredEvents, nil
}

type EventFilter func(event Event) bool

func FilterByIxHash(ixhash common.Hash) EventFilter {
	return func(event Event) bool {
		return event.IxHash == ixhash
	}
}

func FilterByLogicID(logicID identifiers.LogicID) EventFilter {
	return func(event Event) bool {
		return event.LogicID == logicID
	}
}

func FilterByAddress(address identifiers.Address) EventFilter {
	return func(event Event) bool {
		return event.Address == address
	}
}

func FilterByName(name string) EventFilter {
	encoded, _ := polo.Polorize(name)
	eventHash := blake2b.Sum256(encoded)

	return func(event Event) bool {
		return event.Topics[0] == eventHash
	}
}

func FilterByTopic(index int, topicHash common.Hash) EventFilter {
	return func(event Event) bool {
		if index >= 1 && index < len(event.Topics) {
			return event.Topics[index] == topicHash
		}

		return false
	}
}
