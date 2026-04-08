package common

import (
	"time"
)

type ViewTicker struct {
	c    chan uint64
	done chan struct{}
}

// NewViewTicker starts and returns a new SlotTicker instance.
func NewViewTicker(genesisTime time.Time, secondsPerSlot uint64) *ViewTicker {
	if genesisTime.IsZero() {
		panic("zero genesis time")
	}

	ticker := &ViewTicker{
		c:    make(chan uint64),
		done: make(chan struct{}),
	}

	ticker.start(Canonical(genesisTime), secondsPerSlot, Since, Until, time.After)

	return ticker
}

func (s *ViewTicker) start(
	genesisTime time.Time,
	secondsPerSlot uint64,
	since, until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time,
) {
	d := time.Duration(secondsPerSlot) * time.Second

	go func() {
		sinceGenesis := since(genesisTime)

		var (
			nextTickTime time.Time
			view         uint64
		)

		if sinceGenesis < d {
			// Handle when the current time is before the genesis time.
			nextTickTime = genesisTime
			view = 0
		} else {
			nextTick := sinceGenesis.Truncate(d) + d
			nextTickTime = genesisTime.Add(nextTick)
			view = uint64(nextTick / d)
		}

		for {
			waitTime := until(nextTickTime)
			select {
			case <-after(waitTime):
				s.c <- view

				view++

				nextTickTime = nextTickTime.Add(d)
			case <-s.done:
				return
			}
		}
	}()
}

// Done should be called to clean up the ticker.
func (s *ViewTicker) Done() {
	go func() {
		s.done <- struct{}{}
	}()
}

// C returns the ticker channel. Call Cancel afterwards to ensure
// that the goroutine exits cleanly.
func (s *ViewTicker) C() <-chan uint64 {
	return s.c
}
