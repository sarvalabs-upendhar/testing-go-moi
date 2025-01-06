package compute

import (
	"context"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicEnlist performs the given IxLogicEnlist Operation.
// The state.Transition must contain state objects for the sender and Target of the op.
//
// The IxOp must have a LogicPayload and the output receipt will have a LogicEnlistResult.
// The logic call is verified and executed with the output/error being returned as the result.
func RunLogicEnlist(
	op *common.IxOp,
	ctx *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	status := common.ResultOk

	// Create a new op result
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the invoker and logic account state objects
	invoker := objects.GetObject(op.SenderAddr())
	logicacc := objects.GetObject(op.Target())

	// Create an event stream to emit the events on
	eventstream := NewEventStream(op.LogicID())
	fueltank := NewFuelTank(op.FuelLimit())

	consumption, receiptPayload, err := EnlistLogic(op, ctx, logicacc, invoker, fueltank, eventstream)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		status = common.ResultFuelExhausted
	}

	// Set the result payload
	common.SetResultPayload(opResult, *receiptPayload)

	// Set the status of the receipt
	if receiptPayload.Error != nil {
		status = common.ResultExceptionRaised
	}

	// Set the logs in the receipt
	opResult.SetLogs(eventstream.Collect())
	opResult.SetStatus(status)

	return opResult
}

func EnlistLogic(
	op *common.IxOp,
	ctx *common.ExecutionContext,

	logicState *state.Object,
	senderState *state.Object,

	fueltank *FuelTank,
	eventstream *EventStream,
) (
	uint64, *common.LogicEnlistResult, error,
) {
	// Verify that the callsite is not empty
	if op.Callsite() == "" {
		return 0, nil, errors.New("callsite cannot be empty")
	}

	logicID := op.LogicID()

	// Fetch the logic object from the state object
	logicObject, err := logicState.FetchLogicObject(logicID)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := logicObject.GetCallsite(op.Callsite()); !ok {
		return 0, nil, errors.Errorf("callsite '%v' does not exist for logic", op.Callsite())
	}

	// Obtain the runtime engine for the logic object
	engine, ok := engineio.FetchEngine(logicObject.Engine())
	if !ok {
		return 0, nil, errors.Errorf("missing engine factory: %v", logicObject.Engine())
	}

	// Create a new engine for the execution
	instance, err := engine.SpawnInstance(
		logicObject,
		fueltank.Level(),
		logicState.GenerateLogicStorageObject(logicID),
		ctx,
		eventstream,
	)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Initialise the logic tree for the sender (enlist)
	if err = senderState.InitLogicStorage(logicID); err != nil {
		return 0, nil, errors.Wrap(err, "unable to initialise ephemeral logic storage tree")
	}

	senderCtx := senderState.GenerateLogicStorageObject(logicID)

	// Perform execution call on the engine
	result, err := instance.Call(context.Background(), op, senderCtx)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !fueltank.Exhaust(result.Fuel()) {
		return 0, nil, errors.New("insufficient fuel: could not call logic enlister")
	}

	// Check the execution result
	if !result.Ok() {
		return fueltank.Consumed, &common.LogicEnlistResult{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the return data
	return fueltank.Consumed, &common.LogicEnlistResult{Outputs: result.Outputs()}, nil
}

func (manager *Manager) ValidateLogicEnlist(
	op *common.IxOp, callerAcc, logicAcc *state.Object,
) error {
	// Fetch logic ID
	logicID := op.LogicID()

	// If the caller already has a storage tree for the logic ID,
	// then they are already enlisted.
	ok, err := callerAcc.HasStorageTree(logicID)
	if err != nil {
		return err
	}

	if ok {
		return common.ErrAlreadyEnlisted
	}

	// Fetch the logic object for the ID
	logic, err := logicAcc.FetchLogicObject(logicID)
	if err != nil {
		return err
	}

	runtime, ok := engineio.FetchEngine(logic.Engine())
	if !ok {
		return errors.New("failed to get runtime for logic")
	}

	return runtime.ValidateCalldata(logic, op)
}
