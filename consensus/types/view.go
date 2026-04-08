package types

import (
	"sync/atomic"
	"time"
)

type View struct {
	id        atomic.Uint64
	startTime atomic.Value
	deadline  atomic.Value
}

func NewView(id uint64, startTime, deadline time.Time) *View {
	var view View

	view.id.Store(id)
	view.startTime.Store(startTime)
	view.deadline.Store(deadline)

	return &view
}

func (v *View) ID() uint64 {
	return v.id.Load()
}

func (v *View) IsEqualID(id uint64) bool {
	return v.ID() == id
}

func (v *View) IsNextView(id uint64) bool {
	return id-v.ID() == 1
}

func (v *View) Deadline() time.Time {
	return v.deadline.Load().(time.Time) //nolint:forcetypeassert
}

func (v *View) StartTime() time.Time {
	return v.startTime.Load().(time.Time) //nolint:forcetypeassert
}

func (v *View) SetID(id uint64) {
	v.id.Store(id)
}

func (v *View) SetDeadline(t time.Time) {
	v.deadline.Store(t)
}
