package engineio

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-polo"
)

type ErrorResult struct {
	Kind   string
	Error  string
	Revert bool
	Trace  []string
}

type CallResult struct {
	Out           polo.Document
	Err           []byte
	Logs          []common.Log
	ComputeEffort uint64
	StorageEffort uint64
}

func (cr *CallResult) IsError() bool {
	return len(cr.Err) > 0
}
