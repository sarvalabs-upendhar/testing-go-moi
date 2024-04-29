package pisa

import (
	"github.com/sarvalabs/go-pisa/exception"

	"github.com/sarvalabs/go-moi/compute/engineio"
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
	except exception.Exception
}

func (err Error) Engine() engineio.EngineKind { return engineio.PISA }

func (err Error) Bytes() []byte  { return err.except.Bytes() }
func (err Error) String() string { return err.except.String() }

func (err Error) Reverted() bool { return err.except.Revert }
