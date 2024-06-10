package core

import (
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"
)

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
	eventHash := blake2b.Sum256([]byte(name))

	return func(event Event) bool {
		return event.Topics[0] == eventHash
	}
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

		head, size := env.eventDB.head, env.eventDB.size
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
