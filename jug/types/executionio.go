package types

import (
	"github.com/sarvalabs/moichain/types"
)

// Values is an interface for some string indexed data
type Values interface {
	Size() int
	Bytes() []byte

	Get(string) []byte
	Set(string, []byte)

	GetObject(string, any) error
	SetObject(string, any) error
}

// ExecutionScope is the input passed into the ExecutionEnvironment
type ExecutionScope struct {
	// Initialise specifies if this is constructor call
	Initialise bool
	// Callsite specifies the engine callsite to execute
	Callsite string
	// Values represents the input value arguments for execution
	Inputs Values

	// Caller represents the storage driver for the execution caller.
	// This is always storage of the invoking participant.
	Caller Storage
	// Callee represents the storage driver for the execution callee.
	// This is the storage of with logic if it is stateful,
	// otherwise is the storage of the another participant.
	Callee Storage

	// Timestamp specifies the current timestamp
	Timestamp string
	// Interaction specifies the interaction hash
	Interaction types.Hash
}

// ExecutionResult is the output emitted from ExecutionEnvironment
type ExecutionResult struct {
	// Values represents the output values from the execution
	Outputs Values

	// Fuel represents the amount fuel consumed for execution
	Fuel uint64
	// Logs represents the execution logs and events emitted during execution
	Logs []string

	// Exception represents the exception code of the execution.
	// 0, if no errors occurred during execution
	Exception uint64
	// ErrMessage represents some errors message from the execution.
	// Empty if Exception code is 0.
	ErrMessage string
}
