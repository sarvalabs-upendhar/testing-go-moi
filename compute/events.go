package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type EventStream struct {
	logicID identifiers.LogicID
	events  []common.Log
}

// NewEventStream generates a new EventStream to emit the events on
func NewEventStream(logicID identifiers.LogicID) *EventStream {
	return &EventStream{
		logicID: logicID,
		events:  make([]common.Log, 0),
	}
}

func (stream *EventStream) Logic() identifiers.LogicID { return stream.logicID }
func (stream *EventStream) Count() uint64              { return uint64(len(stream.events)) }
func (stream *EventStream) Collect() []common.Log      { return stream.events }

func (stream *EventStream) Reset() {
	stream.events = make([]common.Log, 0)
}

func (stream *EventStream) Insert(event common.Log) {
	stream.events = append(stream.events, event)
}

func (stream *EventStream) Fetch(index uint64) (common.Log, bool) {
	if index >= stream.Count() {
		return common.Log{}, false
	}

	return stream.events[index], true
}

func (stream *EventStream) Iterate() <-chan common.Log {
	ch := make(chan common.Log)

	go func() {
		defer close(ch)

		for _, event := range stream.events {
			ch <- event
		}
	}()

	return ch
}
