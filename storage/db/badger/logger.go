package badger

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
)

// BadgerLogger implements the badger.Logger interface.
type BadgerLogger struct {
	logger hclog.Logger
}

// Errorf logs an ERROR message to the logger
func (bl *BadgerLogger) Errorf(format string, args ...interface{}) {
	bl.logger.Error(fmt.Sprintf(format, args...))
}

// Infof logs an INFO message to the logger
func (bl *BadgerLogger) Infof(format string, args ...interface{}) {
	bl.logger.Info(fmt.Sprintf(format, args...))
}

// Debugf logs a DEBUG message to the logger
func (bl *BadgerLogger) Debugf(format string, args ...interface{}) {
	bl.logger.Debug(fmt.Sprintf(format, args...))
}

// Warningf logs a WARNING message to the logger
func (bl *BadgerLogger) Warningf(format string, args ...interface{}) {
	bl.logger.Warn(fmt.Sprintf(format, args...))
}
