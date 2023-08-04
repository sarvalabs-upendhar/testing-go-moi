package compute

import (
	"context"
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicInvoke performs the given IxLogicInvoke interaction.
// The stateObjectRetriever must contain state objects for the sender and receiver of the Interaction.
//
// The Interaction must have a LogicPayload and the output receipt will have a LogicInvokeReceipt.
// The logic call is verified and executed with the output/error being returned in the receipt.
func RunLogicInvoke(ix *common.Interaction, tank *engineio.FuelTank, objects state.ObjectMap) (*common.Receipt, error) {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return nil, errors.Wrap(err, "could not find logic payload")
	}

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
	options = append(options, InvokeCall(payload.Callsite, payload.Calldata))

	consumption, receiptPayload, err := InvokeLogic(payload.Logic, logicacc, options...)
	if err != nil {
		return nil, err
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		return nil, common.ErrInsufficientFuel
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	// Set the status of the receipt
	if receiptPayload.Error != nil {
		receipt.Status = common.ReceiptFailed
	}

	// Set the extra data of the receipt
	if err = receipt.SetExtraData(receiptPayload); err != nil {
		return nil, err
	}

	return receipt, nil
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

// InvokeCall returns a LogicInvokeOption to provide the invokable callsite and calldata for state setup
func InvokeCall(callsite string, calldata []byte) LogicInvokeOption {
	return func(config *logicInvoker) error {
		config.callsite = callsite
		config.calldata = calldata

		return nil
	}
}

// InvokeFuelLimit returns a LogicInvokeOption to provide the fuel limit for logic deployment.
func InvokeFuelLimit(limit engineio.Fuel) LogicInvokeOption {
	return func(config *logicInvoker) error {
		config.fueltank = engineio.NewFuelTank(limit)

		return nil
	}
}

// InvokeLogic invokes the Logic at the given Logic ID from the given state object.
// Invocation behavior can be extended with LogicInvokeOption functions to set fuel
// limit for the invocation or provide invoker state or invoke call parameters.
// Uses unlimited fuel limit unless otherwise specified with the InvokeFuelLimit option.
func InvokeLogic(logicID common.LogicID, state *state.Object, opts ...LogicInvokeOption) (
	engineio.Fuel, *common.LogicInvokeReceipt, error,
) {
	// Generate basic invoke config
	invoker := &logicInvoker{
		logicID:    logicID,
		logicState: state,
		fueltank: engineio.NewFuelTank(func() engineio.Fuel {
			fuel, _ := new(big.Int).SetString("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 0)

			return fuel
		}()),
	}

	// Apply all invoke options on the config
	for _, opt := range opts {
		if err := opt(invoker); err != nil {
			return nil, nil, err
		}
	}

	// Verify that the callsite is not empty
	if invoker.callsite == "" {
		return nil, nil, errors.New("callsite cannot be empty")
	}

	var err error

	// Fetch the logic object from the state object
	invoker.logicObject, err = invoker.logicState.FetchLogicObject(invoker.logicID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := invoker.logicObject.GetCallsite(invoker.callsite); !ok {
		return nil, nil, errors.Errorf("callsite '%v' does not exist for logic", invoker.callsite)
	}

	// Obtain the runtime for the logic engine of the logic object
	runtime, ok := engineio.FetchEngineRuntime(invoker.logicObject.Engine())
	if !ok {
		return nil, nil, errors.Errorf("missing engine factory: %v", invoker.logicObject.Engine())
	}

	// Create a new engine for the execution
	engine, err := runtime.SpawnEngine(
		invoker.fueltank.Level(), invoker.logicObject,
		invoker.logicState.GenerateLogicContextObject(invoker.logicObject.LogicID()),
		envObject{},
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Create an IxnObject
	// todo: we should pass the raw ixn somehow
	ixn := ixnObject{
		kind:     common.IxLogicInvoke,
		price:    big.NewInt(1),
		limit:    invoker.fueltank.Capacity,
		callsite: invoker.callsite,
		calldata: invoker.calldata,
	}

	// Declare sender context driver
	var senderCtx engineio.CtxDriver
	// Create the deployer context driver if not nil
	if invoker.senderState != nil {
		senderCtx = invoker.senderState.GenerateLogicContextObject(invoker.logicObject.LogicID())
	}

	// Perform execution call on the engine
	result, err := engine.Call(context.Background(), ixn, senderCtx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !invoker.fueltank.Exhaust(result.Consumed) {
		return nil, nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	// Check the execution result
	if !result.Ok() {
		return invoker.fueltank.Consumed, &common.LogicInvokeReceipt{Error: result.Error}, nil
	}

	// Return the total fuel consumed and the return data
	return invoker.fueltank.Consumed, &common.LogicInvokeReceipt{Outputs: result.Outputs}, nil
}

type logicInvoker struct {
	logicID     common.LogicID
	logicObject *state.LogicObject

	callsite string
	calldata []byte

	fueltank    *engineio.FuelTank
	logicState  *state.Object
	senderState *state.Object
}
