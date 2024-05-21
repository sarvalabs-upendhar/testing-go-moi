package compute

import (
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

type EventStream struct {
	logicID identifiers.LogicID
	events  []engineio.LogicEvent
}

// NewEventStream generates a new EventStream to emit the events on
func NewEventStream(logicID identifiers.LogicID) *EventStream {
	return &EventStream{
		logicID: logicID,
		events:  make([]engineio.LogicEvent, 0),
	}
}

func (es *EventStream) Size() uint64 {
	return uint64(len(es.events))
}

func (es *EventStream) Reset() error {
	es.events = make([]engineio.LogicEvent, 0)

	return nil
}

func (es *EventStream) Get() []engineio.LogicEvent {
	return es.events
}

func (es *EventStream) Iter() chan<- engineio.LogicEvent {
	ch := make(chan engineio.LogicEvent)

	go func() {
		defer close(ch)

		for _, event := range es.events {
			ch <- event
		}
	}()

	return ch
}

func (es *EventStream) Emit(event engineio.LogicEvent) error {
	es.events = append(es.events, event)

	return nil
}

func (es *EventStream) Logic() identifiers.LogicID {
	return es.logicID
}

// GetAsLogs converts the events in the EventStream to common.Log format
func (es *EventStream) GetAsLogs() []*common.Log {
	logs := make([]*common.Log, 0)

	// Iterate over each event in the event stream
	for _, event := range es.events {
		log := &common.Log{
			Address: identifiers.Address(event.Address),
			LogicID: es.logicID,
			Topics:  make([]common.Hash, len(event.Topics)),
			Data:    event.Data,
		}

		// Copy topics to log.Topics
		for i, topic := range event.Topics {
			log.Topics[i] = topic
		}

		logs = append(logs, log)
	}

	return logs
}
