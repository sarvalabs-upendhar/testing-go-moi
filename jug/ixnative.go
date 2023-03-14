package jug

import (
	"context"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// ValueTransfer performs the IxValueTransfer interaction on the given sender and receiver StateObjects.
// The given amount for the given assetID is decremented from the sender and incremented on the receiver.
// Returns an error if the given amount is invalid (negative) or if the sender does not have enough balance.
func (executor IxExecutor) ValueTransfer(
	sender, receiver *guna.StateObject,
	assetID types.AssetID,
	amount *big.Int,
) (uint64, error) {
	// Check if given transfer amount is valid
	if amount.Sign() <= 0 {
		return 0, errors.New("invalid transfer amount")
	}

	// Fetch sender balance object
	senderBalance, err := sender.BalanceOf(assetID)
	if err != nil {
		return 0, err
	}

	// Check if sender has sufficient balance
	if senderBalance.Cmp(amount) == -1 {
		return 0, errors.New("insufficient balance")
	}

	// Remove amount from sender balance for asset
	sender.SubBalance(assetID, amount)
	// Add amount to receiver balance for asset
	receiver.AddBalance(assetID, amount)

	return 1, nil
}

// CreateAsset performs the IxCreateAsset interaction on the given creator StateObject.
// The given asset creation spec is used to create the asset which is then
// inserted into the state object of the creator (sender of interaction)
func (executor IxExecutor) CreateAsset(
	creator *guna.StateObject,
	payload types.AssetCreatePayload,
) (uint64, string, error) {
	// todo: if payload.LogicID or LogicCode is set: handle
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.
	asset := types.NewAssetDescriptor(types.NilAddress, payload)

	// Create a new asset on the creator state object and get the asset ID
	assetID, err := creator.CreateAsset(asset)
	if err != nil {
		return 0, "", err
	}

	// Return the string form of the asset ID
	return 1, string(assetID), nil
}

// LogicDeploy performs the IxLogicDeploy interaction on the given state (logic account).
// The given logic deployment payload is used to create a logic which is then inserted into
// the state object. The logic is then initialised (if stateful)
func (executor IxExecutor) LogicDeploy(
	state *guna.StateObject,
	payload *types.LogicPayload,
) (uint64, types.LogicID, error) {
	// Get the current tank level
	available := executor.tank.Level()

	// Decode the manifest data into a ManifestHeader
	header := new(ctypes.ManifestHeader)
	if err := polo.Depolorize(header, payload.Manifest); err != nil {
		return 0, nil, errors.Wrap(err, "could not decode manifest header")
	}

	// Obtain the factory for the logic engine in the header
	factory, ok := executor.exec.factories[header.LogicEngine()]
	if !ok {
		return 0, nil, errors.Errorf("unsupported manifest engine: %v", header.Engine)
	}

	// Create a compiler engine
	compiler := factory.NewExecutionEngine(available)
	// Compile the manifest into a LogicDescriptor
	logicDescriptor, compileResult := compiler.Compile(context.Background(), payload.Manifest)
	// Check compile result
	if !compileResult.Ok() {
		return compileResult.Fuel, nil, errors.Errorf(
			"manifest compile failed [error code: %v]: %v",
			compileResult.ErrCode, compileResult.ErrMessage,
		)
	}

	// Set the manifest data into the state object dirty entries.
	// This manifest will now be content addressed with its hash.
	state.SetDirtyEntry(logicDescriptor.Manifest.Hex(), payload.Manifest)

	// Get the current consumed fuel
	consumed := compileResult.Fuel
	available -= consumed

	if available == 0 {
		return consumed, nil, errors.New("insufficient fuel: could not initialise after compile")
	}

	// Generate the LogicID from the payload
	logicID, err := types.NewLogicIDv0(
		logicDescriptor.PersistentState,
		logicDescriptor.EphemeralState,
		logicDescriptor.AllowsInteractions,
		false,
		0, state.Address(),
	)
	if err != nil {
		return consumed, nil, errors.Wrap(err, "could not generate logic id")
	}

	// Create a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(logicID, logicDescriptor)
	// Insert the LogicObject into the state object of the logic
	if err = state.InsertNewLogicObject(logicID, logicObject); err != nil {
		return consumed, nil, errors.Wrap(err, "could not insert logic object into stateobject")
	}

	// Initialize a storage tree for the LogicID on the state object
	if err = state.CreateStorageTreeForLogic(logicID); err != nil {
		return consumed, nil, errors.Wrap(err, "could not init storage tree for logic")
	}

	// Decode the calldata inputs into a polo.Document
	inputs := new(polo.Document)
	if err = polo.Depolorize(inputs, payload.Calldata); err != nil {
		return consumed, nil, errors.Wrap(err, "could not decode calldata into polo document")
	}

	// Create a new engine for the execution
	engine := factory.NewExecutionEngine(available)
	order := &ctypes.ExecutionOrder{Initialise: true, Inputs: inputs, Callee: state}

	// Execute the initialisation order in the engine
	execResult := engine.Execute(context.Background(), logicObject, order)
	// Check execution result
	if !execResult.Ok() {
		return consumed + execResult.Fuel, nil, errors.Errorf(
			"initialise execution failed [error code: %v]: %v",
			execResult.ErrCode, execResult.ErrMessage,
		)
	}

	// Return the total fuel consumed and the logic ID
	return consumed + execResult.Fuel, logicID, nil
}

// LogicInvoke performs the IxLogicInvoke interface on the given state (logic account).
// The given logic payload describes the callsite and calldata for the execution.
func (executor IxExecutor) LogicInvoke(
	state *guna.StateObject,
	payload *types.LogicPayload,
) (uint64, []byte, error) {
	// Get the current tank level
	available := executor.tank.Level()

	// Fetch the logic object from the state object
	logicObject, err := state.FetchLogicObject(payload.Logic)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := logicObject.GetCallsite(payload.Callsite); !ok {
		return 0, nil, errors.Wrap(err, "callsite does not exists for logic")
	}

	// Decode the calldata inputs into a polo.Document
	inputs := new(polo.Document)
	if err = polo.Depolorize(inputs, payload.Calldata); err != nil {
		return 0, nil, errors.Wrap(err, "could not decode calldata into polo document")
	}

	// Obtain the factory for the logic engine of the logic object
	factory := executor.exec.factories[logicObject.Engine()]

	// Create a new engine for the execution
	engine := factory.NewExecutionEngine(available)
	// Create an execution order with the input data and callsite
	order := &ctypes.ExecutionOrder{Callsite: payload.Callsite, Inputs: inputs, Callee: state}

	// Execute the order in the engine
	execResult := engine.Execute(context.Background(), logicObject, order)
	// Check execution result
	if !execResult.Ok() {
		return execResult.Fuel, nil, errors.Errorf(
			"runtime execution failed [error code: %v]: %v",
			execResult.ErrCode, execResult.ErrMessage,
		)
	}

	// Return the total fuel consumed and the return data
	return execResult.Fuel, execResult.Outputs.Bytes(), nil
}
