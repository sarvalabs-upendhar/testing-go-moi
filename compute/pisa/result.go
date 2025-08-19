package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
)

func NewError(kind, err string, revert bool, trace []string) engineio.ErrorResult {
	return engineio.ErrorResult{
		Kind:   kind,
		Error:  err,
		Revert: revert,
		Trace:  trace,
	}
}
