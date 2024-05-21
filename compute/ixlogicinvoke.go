package compute

import (
	"context"
	"math"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicInvoke performs the given IxLogicInvoke interaction.
// The stateObjectRetriever must contain state objects for the sender and receiver of the Interaction.
//
// The Interaction must have a LogicPayload and the output receipt will have a LogicInvokeReceipt.
// The logic call is verified and executed with the output/error being returned in the receipt.
func RunLogicInvoke(
	ix *common.Interaction,
	ctx *common.ExecutionContext,
	tank *FuelTank,
	objects state.ObjectMap,
) *common.Receipt {
	// Obtain the Logic Payload from the Interaction
	payload, _ := ix.GetLogicPayload()

	// Generate a new receipt
	receipt := common.NewReceipt(ix)

	// Generate the address of the target logic account from the LogicID
	logicAddress := payload.Logic.Address()
	// Obtain the invoker and logic account state objects
	invoker := objects.GetObject(ix.Sender())
	logicacc := objects.GetObject(logicAddress)

	// Create an options chain
	options := make([]LogicInvokeOption, 0, 3)
	// Append invoker options for invoker state and fuel limit
	options = append(options, InvokerState(invoker))
	options = append(options, InvokeFuelLimit(tank.Level()))

	// Create an event stream to emit the events on
	eventstream := NewEventStream(ix.LogicID())

	consumption, receiptPayload, err := InvokeLogic(ix, ctx, logicacc, eventstream, options...)
	if err != nil {
		receipt.Status = common.ReceiptStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		receipt.Status = common.ReceiptFuelExhausted
	}

	// Set the fuel consumption
	receipt.SetFuelUsed(tank.Consumed)
	// Set the extra data of the receipt
	common.SetReceiptExtraData(receipt, *receiptPayload)

	// Set the status of the receipt
	if receiptPayload.Error != nil {
		receipt.Status = common.ReceiptExceptionRaised
	}

	// Set the logs in the receipt
	receipt.SetLogs(eventstream.GetAsLogs())

	return receipt
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
	ixn *common.Interaction,
	ctx *common.ExecutionContext,
	state *state.Object,
	eventstream *EventStream,
	opts ...LogicInvokeOption,
) (
	uint64, *common.LogicInvokeReceipt, error,
) {
	// Generate basic invoke config
	invoker := &logicInvoker{
		logicState: state,
		logicID:    ixn.LogicID(),
		fueltank:   NewFuelTank(math.MaxUint64),
	}

	// Apply all invoke options on the config
	for _, opt := range opts {
		if err := opt(invoker); err != nil {
			return 0, nil, err
		}
	}

	// Verify that the callsite is not empty
	if ixn.Callsite() == "" {
		return 0, nil, errors.New("callsite cannot be empty")
	}

	var err error

	// Fetch the logic object from the state object
	invoker.logicObject, err = invoker.logicState.FetchLogicObject(invoker.logicID)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := invoker.logicObject.GetCallsite(ixn.Callsite()); !ok {
		return 0, nil, errors.Errorf("callsite '%v' does not exist for logic", ixn.Callsite())
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
		invoker.logicState.GenerateLogicContextObject(invoker.logicObject.ID),
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
		senderCtx = invoker.senderState.GenerateLogicContextObject(invoker.logicObject.ID)
	}

	// Perform execution call on the engine
	result, err := instance.Call(context.Background(), ixn, senderCtx)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !invoker.fueltank.Exhaust(result.Fuel()) {
		return 0, nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	// Check the execution result
	if !result.Ok() {
		return invoker.fueltank.Consumed, &common.LogicInvokeReceipt{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the return data
	return invoker.fueltank.Consumed, &common.LogicInvokeReceipt{Outputs: result.Outputs()}, nil
}

type logicInvoker struct {
	logicID     identifiers.LogicID
	logicObject *state.LogicObject

	fueltank    *FuelTank
	logicState  *state.Object
	senderState *state.Object
}
