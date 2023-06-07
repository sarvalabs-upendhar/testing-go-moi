package jug

import (
	"context"
	"math/big"

	"github.com/sarvalabs/moichain/common/hexutil"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

// ValueTransfer performs the IxValueTransfer interaction on the given sender and receiver StateObjects.
// The given amount for the given assetID is decremented from the sender and incremented on the receiver.
// Returns an error if the given amount is invalid (negative) or if the sender does not have enough balance.
func (executor IxExecutor) ValueTransfer(
	sender, receiver *guna.StateObject,
	assetID types.AssetID,
	amount *big.Int,
) (
	engineio.Fuel, error,
) {
	// Check if given transfer amount is valid
	if amount.Sign() <= 0 {
		return nil, errors.New("invalid transfer amount")
	}

	// Fetch sender balance object
	senderBalance, err := sender.BalanceOf(assetID)
	if err != nil {
		return nil, err
	}

	// Check if sender has sufficient balance
	if senderBalance.Cmp(amount) == -1 {
		return nil, errors.New("insufficient balance")
	}

	// Remove amount from sender balance for asset
	sender.SubBalance(assetID, amount)
	// Add amount to receiver balance for asset
	receiver.AddBalance(assetID, amount)

	return FuelSimpleValueTransfer, nil
}

// CreateAsset performs the IxCreateAsset interaction on the given creator StateObject.
// The given asset creation spec is used to create the asset which is then
// inserted into the state object of the creator (sender of interaction)
func (executor IxExecutor) CreateAsset(
	creator, assetAccount *guna.StateObject,
	payload types.AssetCreatePayload,
) (
	engineio.Fuel, *types.AssetCreationReceipt, error,
) {
	// todo: if payload.LogicID or LogicCode is set: handle
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.
	asset := types.NewAssetDescriptor(creator.Address(), payload)

	// Create a new asset on the creator state object and get the asset ID
	assetID, err := creator.CreateAsset(assetAccount.Address(), asset)
	if err != nil {
		return nil, nil, err
	}

	creator.AddBalance(assetID, asset.Supply)

	_, err = assetAccount.CreateAsset(assetAccount.Address(), asset)
	if err != nil {
		return nil, nil, err
	}

	// Return the string form of the asset ID
	return FuelAssetCreation, &types.AssetCreationReceipt{AssetID: assetID, AssetAccount: assetAccount.Address()}, nil
}

func (executor IxExecutor) MintAsset(
	assetOwner, assetAccount *guna.StateObject,
	payload types.AssetMintOrBurnPayload,
) (
	engineio.Fuel, *types.AssetMintOrBurnReceipt, error,
) {
	registry, err := assetAccount.GetRegistryEntry(payload.Asset.String())
	if err != nil {
		return nil, nil, err
	}

	ad := new(types.AssetDescriptor)
	if err = ad.FromBytes(registry); err != nil {
		return nil, nil, err
	}

	// update the supply and asset registry
	ad.Supply.Add(ad.Supply, payload.Amount)

	rawData, err := ad.Bytes()
	if err != nil {
		return nil, nil, err
	}

	if err = assetAccount.UpdateRegistryEntry(payload.Asset.String(), rawData); err != nil {
		return nil, nil, err
	}

	// add minted tokens to assetOwner account
	assetOwner.AddBalance(payload.Asset, payload.Amount)

	return FuelSimpleValueTransfer, &types.AssetMintOrBurnReceipt{TotalSupply: (hexutil.Big)(*ad.Supply)}, nil
}

func (executor IxExecutor) BurnAsset(
	assetOwner, assetAccount *guna.StateObject,
	payload types.AssetMintOrBurnPayload,
) (
	engineio.Fuel, *types.AssetMintOrBurnReceipt, error,
) {
	bal, err := assetOwner.BalanceOf(payload.Asset)
	if err != nil {
		return nil, nil, err
	}

	if bal.Cmp(payload.Amount) < 0 {
		return nil, nil, types.ErrInsufficientFunds
	}

	// burn tokens from assetOwner account
	assetOwner.SubBalance(payload.Asset, payload.Amount)

	registry, err := assetAccount.GetRegistryEntry(payload.Asset.String())
	if err != nil {
		return nil, nil, err
	}

	ad := new(types.AssetDescriptor)
	if err = ad.FromBytes(registry); err != nil {
		return nil, nil, err
	}

	// update the supply and asset registry
	ad.Supply.Sub(ad.Supply, payload.Amount)

	rawData, err := ad.Bytes()
	if err != nil {
		return nil, nil, err
	}

	if err = assetAccount.UpdateRegistryEntry(payload.Asset.String(), rawData); err != nil {
		return nil, nil, err
	}

	return FuelSimpleAssetMint, &types.AssetMintOrBurnReceipt{TotalSupply: (hexutil.Big)(*ad.Supply)}, nil
}

// LogicDeploy performs the IxLogicDeploy interaction on the given logic state.
// The given logic deployment payload is used to create a logic which is then
// inserted into the state object. The logic is then initialised (if stateful)
func (executor IxExecutor) LogicDeploy(
	tank *engineio.FuelTank,
	logicstate, deployer *guna.StateObject,
	payload *types.LogicPayload,
) (
	engineio.Fuel, *types.LogicDeployReceipt, error,
) {
	// Create an options chain
	options := make([]LogicDeployOption, 0, 3)
	// Append deploy options for deployer state and fuel limit
	options = append(options, DeployerState(deployer))
	options = append(options, DeployFuelLimit(tank.Level()))

	// If no callsite is provided, do not append an option for the deployment call
	if payload.Callsite != "" {
		options = append(options, DeploymentCall(payload.Callsite, payload.Calldata))
	}

	return DeployLogic(payload.Manifest, logicstate, options...)
}

// LogicInvoke performs the IxLogicInvoke interface on the given state (logic account).
// The given logic payload describes the callsite and calldata for the execution.
func (executor IxExecutor) LogicInvoke(
	tank *engineio.FuelTank,
	logicstate, deployer *guna.StateObject,
	payload *types.LogicPayload,
) (
	engineio.Fuel, *types.LogicInvokeReceipt, error,
) {
	// Fetch the logic object from the state object
	logicObject, err := logicstate.FetchLogicObject(payload.Logic)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not fetch logic object")
	}

	// Check that the logic contains the payload callsite
	if _, ok := logicObject.GetCallsite(payload.Callsite); !ok {
		return nil, nil, errors.Wrap(err, "callsite does not exists for logic")
	}

	// Obtain the runtime for the logic engine of the logic object
	runtime, ok := engineio.FetchEngineRuntime(logicObject.Engine())
	if !ok {
		return nil, nil, errors.Errorf("missing engine factory: %v", logicObject.Engine())
	}

	// Create a new engine for the execution
	engine, err := runtime.SpawnEngine(
		tank.Level(), logicObject,
		logicstate.GenerateLogicContextObject(logicObject.LogicID()),
		engineio.NewEnvDriver(),
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Create an IxnObject
	ixn := engineio.NewIxnObject(types.IxLogicInvoke, payload.Callsite, payload.Calldata)
	// Perform an Invokable Call
	result, err := engine.Call(context.Background(), ixn, deployer.GenerateLogicContextObject(logicObject.LogicID()))
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not perform call")
	}

	// Check the execution result
	if !result.Ok() {
		return result.Consumed, &types.LogicInvokeReceipt{Error: result.Error}, nil
	}

	// Return the total fuel consumed and the return data
	return result.Consumed, &types.LogicInvokeReceipt{Outputs: result.Outputs}, nil
}
