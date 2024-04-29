package pisa

import (
	"context"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
)

type Instance struct {
	logicIO  engineio.LogicDriver
	internal *pisa.Instance
}

// Kind returns the engineio.EngineKind of the PISA
// Implements the engineio.Engine interface for Instance.
func (instance Instance) Kind() engineio.EngineKind { return engineio.PISA }

// Call performs the execution of a logic function from the logic driver in the Instance.
// It will fail if the engine is not already bootstrapped. It will also fail if the callsite from the
// engineio.IxnDriver does not match the call kind or is not consistent with the number of provided contexts.
// Implements the engineio.Engine interface for Instance.
func (instance Instance) Call(
	_ context.Context,
	ixn engineio.InteractionDriver,
	sender engineio.StateDriver,
	_ ...engineio.StateDriver,
) (
	engineio.CallResult, error,
) {
	// Get the callsite information from the logic and verify that it exists
	callsite, ok := instance.logicIO.GetCallsite(ixn.Callsite())
	if !ok {
		return nil, errors.Errorf("callsite '%v' does not exist", ixn.Callsite())
	}

	switch kind := callsite.Kind; kind {
	// Deployer Callsite: IxLogicDeploy
	case engineio.CallsiteDeployer:
		if ixn.Type() != common.IxLogicDeploy {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for IxLogicDeploy", kind)
		}

	// Invokable Callsite: IxLogicInvoke
	case engineio.CallsiteInvokable:
		if ixn.Type() != common.IxLogicInvoke {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for IxLogicInvoke", kind)
		}

	default:
		return nil, errors.Errorf("unsupported callsite kind '%v'", kind)
	}

	if sender == nil {
		return nil, errors.Errorf("sender driver cannot be nil")
	}

	result, err := instance.internal.PerformCall(newIxn(ixn), newState(sender), nil)
	if err != nil {
		return nil, err
	}

	if result.ErrData != nil {
		return Result{
			consumed: result.FuelUsed,
			errdata:  result.ErrData.Bytes(),
		}, nil
	}

	return Result{
		consumed: result.FuelUsed,
		outdata:  result.OutData.Bytes(),
	}, nil
}
