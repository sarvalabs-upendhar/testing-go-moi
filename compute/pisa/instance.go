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
	txn engineio.IxDriver,
	sender engineio.StateDriver,
	_ ...engineio.StateDriver,
) (
	engineio.CallResult, error,
) {
	// Get the callsite information from the logic and verify that it exists
	callsite, ok := instance.logicIO.GetCallsite(txn.Callsite())
	if !ok {
		return nil, errors.Errorf("callsite '%v' does not exist", txn.Callsite())
	}

	switch kind := txn.Type(); kind {
	case common.IxLogicInvoke:
		if callsite.Kind != engineio.CallsiteInvoke {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for IxLogicInvoke", callsite.Kind)
		}

	case common.IxLogicDeploy:
		if callsite.Kind != engineio.CallsiteDeploy {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for IxLogicDeploy", callsite.Kind)
		}

	case common.IxLogicEnlist:
		if callsite.Kind != engineio.CallsiteEnlist {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for IxLogicEnlist", callsite.Kind)
		}

	default:
		return nil, errors.Errorf("unsupported callsite kind '%v'", kind)
	}

	if sender == nil {
		return nil, errors.Errorf("sender driver cannot be nil")
	}

	result, err := instance.internal.Call(newTxn(txn), newState(sender), nil)
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

func (instance Instance) FuelReset()        { instance.internal.FuelReset() }
func (instance Instance) FuelLevel() uint64 { return instance.internal.FuelLevel() }

func (instance Instance) GetEventDriver() engineio.EventDriver {
	return instance.internal.GetEventDriver().(EventStream).driver //nolint:forcetypeassert
}

func (instance Instance) GetLocalDriver() engineio.StateDriver {
	return instance.internal.GetLocalDriver().(*State).driver //nolint:forcetypeassert
}

func (instance Instance) SetLocalDriver(driver engineio.StateDriver) {
	instance.internal.SetLocalDriver(newState(driver))
}

func (instance Instance) GetSenderDriver() engineio.StateDriver {
	return instance.internal.GetSenderDriver().(*State).driver //nolint:forcetypeassert
}

func (instance Instance) SetSenderDriver(driver engineio.StateDriver) {
	instance.internal.SetSenderDriver(newState(driver))
}
