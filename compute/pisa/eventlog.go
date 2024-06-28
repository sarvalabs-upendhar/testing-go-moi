package pisa

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa/drivers"
	"github.com/sarvalabs/go-polo"
)

type EventStream struct {
	driver engineio.EventDriver
}

func (stream EventStream) Count() uint64 { return stream.driver.Count() }
func (stream EventStream) Empty() bool   { return stream.driver.Count() == 0 }

func (stream EventStream) Reset() error {
	stream.driver.Reset()

	return nil
}

func (stream EventStream) Emit(event drivers.Event) error {
	log := common.Log{
		LogicID: stream.driver.Logic(),
		Address: event.Address,
		Data:    event.Data.Bytes(),
	}

	for _, topic := range event.Topics {
		log.Topics = append(log.Topics, topic)
	}

	stream.driver.Insert(log)

	return nil
}

func (stream EventStream) Get(index uint64) (drivers.Event, bool) {
	log, ok := stream.driver.Fetch(index)
	if !ok {
		return drivers.Event{}, false
	}

	return Log2Event(log), true
}

func (stream EventStream) GetAll() []drivers.Event {
	events := make([]drivers.Event, 0)

	for log := range stream.driver.Iterate() {
		event := Log2Event(log)
		events = append(events, event)
	}

	return events
}

func Log2Event(log common.Log) drivers.Event {
	return drivers.Event{
		Address: log.Address,
		Topics: func() [][32]byte {
			topics := make([][32]byte, 0, len(log.Topics))

			for _, topic := range log.Topics {
				topics = append(topics, topic)
			}

			return topics
		}(),
		Data: func() polo.Document {
			data := make(polo.Document)
			_ = polo.Depolorize(&data, log.Data)

			return data
		}(),
	}
}
