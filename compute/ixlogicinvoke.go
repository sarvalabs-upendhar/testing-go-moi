package compute

import (
	"context"
	"math"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"

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
	ctx *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	status := common.ResultOk

	// Create a new op result
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the invoker and logic account state objects
	invoker := objects.GetObject(op.SenderID())
	logicacc := objects.GetObject(op.Target())

	// Create an options chain
	options := make([]LogicInvokeOption, 0, 3)
	// Append invoker options for invoker state and fuel limit
	options = append(options, InvokerState(invoker))
	options = append(options, InvokeFuelLimit(tank.Level()))

	// Create an event stream to emit the events on
	eventstream := NewEventStream(op.LogicID())

	consumption, receiptPayload, err := InvokeLogic(op, ctx, logicacc, eventstream, options...)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		status = common.ResultFuelExhausted
	}

	// Set the payload for the op
	if receiptPayload != nil {
		common.SetResultPayload(opResult, *receiptPayload)

		// Set the status of the receipt
		if receiptPayload.Error != nil {
			status = common.ResultExceptionRaised
		}
	}

	// Set the logs in the receipt
	opResult.SetLogs(eventstream.Collect())
	opResult.SetStatus(status)

	return opResult
}

// LogicInvokeOption is an option for InvokeLogic and modifies the logic invoke behaviour
type LogicInvokeOption func(invoker *logicInvoker) error

// InvokerState returns a LogicInvokeOption to provide the state object of the invoking account.
func InvokerState(invoker *state.Object) LogicInvokeOption {
	return func(config *logicInvoker) error {
		config.senderState = invoker

		return nil
	}
}

// InvokeFuelLimit returns a LogicInvokeOption to provide the fuel limit for logic deployment.
func InvokeFuelLimit(limit uint64) LogicInvokeOption {
	return func(config *logicInvoker) error {
		config.fueltank = NewFuelTank(limit)

		return nil
	}
}

// InvokeLogic invokes the Logic at the given Logic ID from the given state object.
// Invocation behavior can be extended with LogicInvokeOption functions to set fuel
// limit for the invocation or provide invoker state or invoke call parameters.
// Uses unlimited fuel limit unless otherwise specified with the InvokeFuelLimit option.
func InvokeLogic(
	op *common.IxOp,
	ctx *common.ExecutionContext,
	state *state.Object,
	eventstream *EventStream,
	opts ...LogicInvokeOption,
) (
	uint64, *common.LogicInvokeResult, error,
) {
	// Generate basic invoke config
	invoker := &logicInvoker{
		logicState: state,
		logicID:    op.LogicID(),
		fueltank:   NewFuelTank(math.MaxUint64),
	}

	// Apply all invoke options on the config
	for _, opt := range opts {
		if err := opt(invoker); err != nil {
			return 0, nil, err
		}
	}

	// Verify that the callsite is not empty
	if op.Callsite() == "" {
		return 0, nil, errors.New("callsite cannot be empty")
	}

	var err error

	// Fetch the logic object from the state object
	invoker.logicObject, err = invoker.logicState.FetchLogicObject(invoker.logicID)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := invoker.logicObject.GetCallsite(op.Callsite()); !ok {
		return 0, nil, errors.Errorf("callsite '%v' does not exist for logic", op.Callsite())
	}

	// Obtain the runtime engine fro the logic object
	engine, ok := engineio.FetchEngine(invoker.logicObject.Engine())
	if !ok {
		return 0, nil, errors.Errorf("missing engine factory: %v", invoker.logicObject.Engine())
	}

	// Create a new engine for the execution
	instance, err := engine.SpawnInstance(
		invoker.logicObject,
		invoker.fueltank.Level(),
		invoker.logicState.GenerateLogicStorageObject(invoker.logicObject.ID),
		ctx,
		eventstream,
	)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Declare sender context driver
	var senderCtx engineio.StateDriver
	// Create the deployer context driver if not nil
	if invoker.senderState != nil {
		senderCtx = invoker.senderState.GenerateLogicStorageObject(invoker.logicObject.ID)
	}

	// Perform execution call on the engine
	result, err := instance.Call(context.Background(), op, senderCtx)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !invoker.fueltank.Exhaust(result.Fuel()) {
		return 0, nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	// Check the execution result
	if !result.Ok() {
		return invoker.fueltank.Consumed, &common.LogicInvokeResult{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the return data
	return invoker.fueltank.Consumed, &common.LogicInvokeResult{Outputs: result.Outputs()}, nil
}

type logicInvoker struct {
	logicID     identifiers.LogicID
	logicObject *state.LogicObject

	fueltank    *FuelTank
	logicState  *state.Object
	senderState *state.Object
}

func (manager *Manager) ValidateLogicInvoke(
	op *common.IxOp,
	callerAcc, logicAcc *state.Object,
) error {
	// Fetch logic ID
	logicID := op.LogicID()

	// TODO:Fix this
	// Check if the logic has an ephemeral state
	if ok := logicID.Flag(identifiers.LogicIntrinsic); ok {
		// Check if the call has a storage tree for the logic
		// This means the account has enlisted (as required)
		ok, err := callerAcc.HasStorageTree(logicID)
		if err != nil {
			return err
		}

		if !ok {
			return errors.New("caller not enlisted with ephemeral logic")
		}
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
