package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
)

type Result struct {
	consumed uint64
	outdata  []byte
	errdata  []byte
}

func (result Result) Engine() engineio.EngineKind { return engineio.PISA }

func (result Result) Ok() bool                  { return result.errdata == nil }
func (result Result) Fuel() engineio.EngineFuel { return result.consumed }

func (result Result) Outputs() []byte { return result.outdata }
func (result Result) Error() []byte   { return result.errdata }

type Error struct {
	err pisa.ErrorResult
}

func NewError(kind, err string, revert bool, trace []string) Error {
	return Error{err: pisa.ErrorResult{
		Kind:   kind,
		Error:  err,
		Revert: revert,
		Trace:  trace,
	}}
}

func (err Error) Engine() engineio.EngineKind { return engineio.PISA }

func (err Error) Bytes() []byte  { return err.err.Bytes() }
func (err Error) String() string { return err.err.String() }

func (err Error) Reverted() bool { return err.err.Revert }
