package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa/drivers"
	"github.com/sarvalabs/go-polo"
)

type EventStream struct {
	stream engineio.EventDriver
}

func (es EventStream) Count() uint64 {
	return es.stream.Size()
}

func (es EventStream) Empty() bool {
	return es.stream.Size() == 0
}

func (es EventStream) Emit(event drivers.Event) error {
	logicEvent := engineio.LogicEvent{
		Address: event.Address,
		Topics:  event.Topics,
		Data:    event.Data.Bytes(),
	}

	return es.stream.Emit(logicEvent)
}

func (es EventStream) Reset() error {
	return es.stream.Reset()
}

func (es EventStream) Get(index uint64) (drivers.Event, bool) {
	if index >= es.stream.Size() {
		return drivers.Event{}, false
	}

	return es.GetAll()[index], true
}

func (es EventStream) GetAll() []drivers.Event {
	events := make([]drivers.Event, 0)

	for _, event := range es.stream.Get() {
		data := make(polo.Document)
		_ = polo.Depolorize(&data, event.Data)

		drvEvent := drivers.Event{
			Address: event.Address,
			Topics:  event.Topics,
			Data:    data,
		}

		events = append(events, drvEvent)
	}

	return events
}
