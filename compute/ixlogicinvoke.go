package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicInvoke performs the given IxLogicInvoke operation.
// The stateObjectRetriever must contain state objects for the sender and beneficiary of the op.
//
// The IxOp must have a LogicPayload and the output receipt will have a LogicInvokeResult.
// The logic call is verified and executed with the output/error being returned as the result.
func RunLogicInvoke(
	op *common.IxOp,
	ctx *engineio.RuntimeContext,
	tank *FuelTank,
	_ *state.Transition,
) *common.IxOpResult {
	// Create a new op result
	opResult := common.NewIxOpResult(op.Type())

	// FIXME: Add more validation checks
	if err := ValidateLogicInvoke(op); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	result := ctx.Runtime.Call(op.Target(), op, &engineio.FuelGauge{
		Compute: tank.ComputeCapacity,
		Storage: tank.StorageCapacity, // TODO: Fix this
	})

	if !tank.Exhaust(result.ComputeEffort, result.StorageEffort) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	opResult.SetLogs(result.Logs)

	if result.IsError() {
		common.SetResultPayload(opResult, common.LogicInvokeResult{Error: result.Err})

		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	common.SetResultPayload(opResult, common.LogicInvokeResult{Outputs: result.Out.Bytes()})

	return opResult.WithStatus(common.ResultOk)
}

func ValidateLogicInvoke(
	op *common.IxOp,
) error {
	_, err := op.GetLogicPayload()
	if err != nil {
		return err
	}

	return nil
}
