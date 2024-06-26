package compute

import (
	"context"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicEnlist performs the given IxLogicEnlist interaction.
// The state.Transition must contain state objects for the sender and receiver of the Interaction.
//
// The Interaction must have a LogicPayload and the output receipt will have a LogicEnlistReceipt.
// The logic call is verified and executed with the output/error being returned in the receipt.
func RunLogicEnlist(
	ix *common.Interaction,
	ctx *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
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

	// Create an event stream to emit the events on
	eventstream := NewEventStream(ix.LogicID())
	fueltank := NewFuelTank(ix.FuelLimit())

	consumption, receiptPayload, err := EnlistLogic(ix, ctx, logicacc, invoker, fueltank, eventstream)
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
	// Set the logs in the receipt
	receipt.SetLogs(eventstream.Collect())

	// Set the status of the receipt
	if receiptPayload.Error != nil {
		receipt.Status = common.ReceiptExceptionRaised
	}

	return receipt
}

func EnlistLogic(
	ixn *common.Interaction,
	ctx *common.ExecutionContext,

	logicState *state.Object,
	senderState *state.Object,

	fueltank *FuelTank,
	eventstream *EventStream,
) (
	uint64, *common.LogicEnlistReceipt, error,
) {
	// Verify that the callsite is not empty
	if ixn.Callsite() == "" {
		return 0, nil, errors.New("callsite cannot be empty")
	}

	logicID := ixn.LogicID()

	// Fetch the logic object from the state object
	logicObject, err := logicState.FetchLogicObject(logicID)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := logicObject.GetCallsite(ixn.Callsite()); !ok {
		return 0, nil, errors.Errorf("callsite '%v' does not exist for logic", ixn.Callsite())
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
	result, err := instance.Call(context.Background(), ixn, senderCtx)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !fueltank.Exhaust(result.Fuel()) {
		return 0, nil, errors.New("insufficient fuel: could not call logic enlister")
	}

	// Check the execution result
	if !result.Ok() {
		return fueltank.Consumed, &common.LogicEnlistReceipt{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the return data
	return fueltank.Consumed, &common.LogicEnlistReceipt{Outputs: result.Outputs()}, nil
}

func (manager *Manager) ValidateLogicEnlist(ix *common.Interaction, callerAcc, logicAcc *state.Object) error {
	// Fetch logic ID
	logicID := ix.LogicID()

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

	return runtime.ValidateCalldata(logic, ix)
}
