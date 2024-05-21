package engineio

import (
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

// Eventdef represents an event definition in a Logic.
// It can be resolved from a string by looking it up on the Logic
type Eventdef struct {
	Ptr  ElementPtr `json:"ptr" yaml:"ptr"`
	Name string     `json:"name" yaml:"name"`
}

type LogicEvent struct {
	Address [32]byte
	LogicID string
	Topics  [][32]byte
	Data    []byte
}

type EventDriver interface {
	Size() uint64
	Reset() error
	Get() []LogicEvent
	Iter() chan<- LogicEvent
	Emit(LogicEvent) error
	Logic() identifiers.LogicID
}

type debugEventDriver struct {
	logicID identifiers.LogicID
	events  []LogicEvent
}

func NewDebugEventDriver(t *testing.T, logicID identifiers.LogicID) EventDriver {
	t.Helper()

	return &debugEventDriver{
		logicID: logicID,
		events:  make([]LogicEvent, 0),
	}
}

func (d *debugEventDriver) Size() uint64 {
	return uint64(len(d.events))
}

func (d *debugEventDriver) Reset() error {
	d.events = make([]LogicEvent, 0)

	return nil
}

func (d *debugEventDriver) Get() []LogicEvent {
	return d.events
}

func (d *debugEventDriver) Iter() chan<- LogicEvent {
	ch := make(chan LogicEvent)

	go func() {
		defer close(ch)

		for _, event := range d.events {
			ch <- event
		}
	}()

	return ch
}

func (d *debugEventDriver) Emit(event LogicEvent) error {
	d.events = append(d.events, event)

	return nil
}

func (d *debugEventDriver) Logic() identifiers.LogicID {
	return d.logicID
}
