package kbft

import (
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/types"
)

// timeoutInfo is a struct that represents some timeout information that are emitted by the Ticker struct
type timeoutInfo struct {
	// Represents the duration of the timeout
	Duration time.Duration `json:"duration"`
	// Represents the height that the timeout applies for
	Height map[types.Address]uint64 `json:"height"`
	// Represents the round that the timout applies for
	Round int32 `json:"round"`
	// Represents the round step type that the timout applies for
	Step RoundStepType `json:"step"`
}

// Ticker is a struct that represent timout ticker
type Ticker struct {
	logger hclog.Logger

	// Represents the internal timer clock
	timer *time.Timer

	// Represents the channel for timeout start signals
	tick chan timeoutInfo

	// Represents the channel for timeout end signals
	tock chan timeoutInfo

	// Represents the channel for closing the ticker
	quit chan struct{}
}

// NewTicker is a constructor function that generates and returns a new Ticker
func NewTicker(logger hclog.Logger) *Ticker {
	t := &Ticker{
		logger: logger.Named("Ticker"),
		timer:  time.NewTimer(0),
		tick:   make(chan timeoutInfo, 15),
		tock:   make(chan timeoutInfo, 15),
		quit:   make(chan struct{}),
	}
	t.Stop()

	return t
}

// TimeOutChan is a method of Ticker that returns its timeout/tock channel.
// The returned channel can only be read from.
func (t *Ticker) TimeOutChan() <-chan timeoutInfo {
	return t.tock
}

// QuitChan is a method of Ticker that returns the quit channel.
// The returned channel can only be read from.
func (t *Ticker) QuitChan() <-chan struct{} {
	return t.quit
}

// Start is a method of Ticker that starts the ticker's clock
func (t *Ticker) Start() error {
	// Start the timeout routine
	go t.timeoutRoutine()

	return nil
}

// Stop is a method of Ticker that stops the ticker's clock
func (t *Ticker) Stop() {
	// Check if the timer is running
	if !t.timer.Stop() {
		select {
		// Drain the channel
		case <-t.timer.C:
		default:
			t.logger.Debug("Ticker cannot be stopped. Not running")
		}
	}

	// Log the ticker stop
	t.logger.Trace("Ticker stopped")
}

// ScheduleTimeout is a method of Ticker that schedules a new timeout.
// Passes the new timeout information into the tick channel.
func (t *Ticker) ScheduleTimeout(ti timeoutInfo) {
	t.tick <- ti
}

// timeoutRoutine is a method of Ticker that handles the timeout routine.
// Listens on the tick channel for new timeouts to start, pushes to tock channel upon a
// timeout and ends a timeout session when a signal from the quit channel is received.
func (t *Ticker) timeoutRoutine() {
	var info timeoutInfo

	for {
		select {
		case newTimeoutInfo := <-t.tick:
			// Skip scheduling if current timeout is still running AND new timeout height is greater than current
			if len(info.Height) > 0 && areHeightsGreater(info.Height, newTimeoutInfo.Height) {
				continue
			} else if len(info.Height) > 0 && areHeightsEqual(newTimeoutInfo.Height, info.Height) {
				// Skip scheduling if new timeout round is less than current
				if newTimeoutInfo.Round < info.Round {
					continue
				} else if newTimeoutInfo.Round == info.Round {
					// For same rounds, skip scheduling if current timeout has a running round
					// AND new timeout step is less than current
					if info.Step > 0 && newTimeoutInfo.Step <= info.Step {
						continue
					}
				}
			}

			// Stop the ticker clock
			t.Stop()
			// Update the timeout information
			info = newTimeoutInfo
			// Reset the ticker for the duration of the new timeout
			t.timer.Reset(info.Duration)
		// Timeout
		case <-t.timer.C:
			// Asynchronously send a tock
			go func(ti timeoutInfo) { t.tock <- ti }(info)

		// Ticker Close
		case <-t.QuitChan():
			// Log the close and return from the routine
			return
		}
	}
}

// Close is a method of Ticker that closes all ticker routines.
// Discards all scheduled timeouts as well.
func (t *Ticker) Close() {
	t.logger.Trace("Closing timer")
	close(t.quit)
}
