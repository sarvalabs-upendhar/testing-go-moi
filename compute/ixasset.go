package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/state"
)

func RunParticipantCreate(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetParticipantCreatePayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.Sender())
	target := transition.GetObject(op.Target())

	status := common.ResultOk
	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Fetch sender balance object
	senderBalance, _ := sender.BalanceOf(common.KMOITokenAssetID)
	// Check if sender has sufficient balance
	if senderBalance.Cmp(payload.Amount) == -1 {
		status = common.ResultStateReverted
	}

	// Deduct the specified amount from the sender's balance for the given asset
	sender.SubBalance(common.KMOITokenAssetID, payload.Amount)
	// Add the specified amount to the target's balance for the given asset
	target.AddBalance(common.KMOITokenAssetID, payload.Amount)

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleParticipantCreate) {
		status = common.ResultFuelExhausted
	}

	if err := addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		status = common.ResultStateReverted
	}

	opResult.SetStatus(status)

	return opResult
}

// RunAssetTransfer performs the given IxAssetTransfer operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have an AssetActionPayload and the output receipt will have a AssetTransferResult.
// The asset balance is debited from the sender and credited to the Target state objects.
// Returns an error if any of given amounts are invalid (negative)
// or if the sender does not have enough balance for that asset ID
func RunAssetTransfer(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.Sender())
	target := transition.GetObject(op.Target())

	status := common.ResultOk
	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Fetch sender balance object
	senderBalance, _ := sender.BalanceOf(payload.AssetID)
	// Check if sender has sufficient balance
	if senderBalance.Cmp(payload.Amount) == -1 {
		status = common.ResultStateReverted
	}

	// Deduct the specified amount from the sender's balance for the given asset
	sender.SubBalance(payload.AssetID, payload.Amount)
	// Add the specified amount to the target's balance for the given asset
	target.AddBalance(payload.AssetID, payload.Amount)

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleAssetTransfer) {
		status = common.ResultFuelExhausted
	}

	opResult.SetStatus(status)

	return opResult
}

// RunAssetCreate performs the given IxAssetCreate operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have an AssetCreatePayload and the output receipt will have a AssetCreationResult.
// The asset is created and an entry is registered on the registry of both the creator and target accounts.
// The created supply of the asset is credited to the balances of the asset creator.
func RunAssetCreate(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Payload from the Interaction
	payload, _ := op.GetAssetCreatePayload()

	status := common.ResultOk

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the creator and asset account state objects
	creator := transition.GetObject(op.Sender())
	assetacc := transition.GetObject(op.Target())

	// Generate a new Asset Descriptor
	descriptor := common.NewAssetDescriptor(creator.Address(), *payload)

	// todo: [asset logics] handle logic deployment for logical assets
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.

	// Create a new asset on the creator state object and get the asset ID
	assetID, err := creator.CreateAsset(assetacc.Address(), descriptor)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Credit the created asset balance to the creator
	creator.AddBalance(assetID, descriptor.Supply)

	// Generate the asset entry on the target object
	if _, err = assetacc.CreateAsset(assetacc.Address(), descriptor); err != nil {
		status = common.ResultStateReverted
	}

	// todo: [asset logics] this is a simple value now, but will be include logic deployment cost
	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetCreation) {
		status = common.ResultFuelExhausted
	}

	if err = addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), assetacc.Address()); err != nil {
		status = common.ResultStateReverted
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetCreationResult{
		AssetID:      assetID,
		AssetAccount: assetacc.Address(),
	})

	opResult.SetStatus(status)

	return opResult
}

// RunAssetMint performs the given IxAssetMint operation.
// The stateObjectRetriever must contain state objects for the operator and Target of the op.
//
// The IxOp must have an AssetSupplyPayload and the output receipt will have a AssetSupplyResult.
// The asset supply is increased and the new tokens are credited to the balance of the asset creator.
func RunAssetMint(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset mint or burn Payload from the Interaction
	payload, _ := op.GetAssetSupplyPayload()

	status := common.ResultOk

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the mint payload and the asset ID
	assetID := string(payload.AssetID)

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.Sender())
	assetacc := objects.GetObject(op.Target())

	// Obtain the registry entry for the asset from the asset account
	assetEntry, _ := assetacc.GetRegistryEntry(assetID)

	// Decode the asset entry into a AssetDescriptor
	descriptor := new(common.AssetDescriptor)
	_ = descriptor.FromBytes(assetEntry)

	// Update the supply on the descriptor
	descriptor.Supply.Add(descriptor.Supply, payload.Amount)

	// Encode the updated asset descriptor
	encoded, _ := descriptor.Bytes()

	// Update the asset entry in the asset account
	if err := assetacc.UpdateRegistryEntry(assetID, encoded); err != nil {
		status = common.ResultStateReverted
	}

	// Credit the minted tokens to operator account
	operator.AddBalance(payload.AssetID, payload.Amount)

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		status = common.ResultFuelExhausted
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(*descriptor.Supply),
	})

	opResult.SetStatus(status)

	return opResult
}

// RunAssetBurn performs the given IxAssetBurn operation.
// The stateObjectRetriever must contain state objects for the operator and Target of the op.
//
// The IxOp must have an AssetSupplyPayload and the output receipt will have a AssetSupplyResult.
// The asset supply is decreased and the tokens are debited from the balances of the asset creator.
func RunAssetBurn(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Payload from the Interaction
	payload, _ := op.GetAssetSupplyPayload()

	status := common.ResultOk

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the mint payload and the asset ID
	assetID := string(payload.AssetID)

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.Sender())
	assetacc := objects.GetObject(op.Target())

	// Obtain the operator's balance of the asset
	balance, _ := operator.BalanceOf(payload.AssetID)

	// Check that the operator has enough balance for the burn
	if balance.Cmp(payload.Amount) < 0 {
		status = common.ResultStateReverted
	}

	// Burn the tokens from operator account
	operator.SubBalance(payload.AssetID, payload.Amount)

	// Obtain the registry entry for the asset from the asset account
	assetEntry, _ := assetacc.GetRegistryEntry(assetID)

	// Decode the asset entry into a AssetDescriptor
	descriptor := new(common.AssetDescriptor)
	_ = descriptor.FromBytes(assetEntry)

	// Update the asset entry in the asset account
	descriptor.Supply.Sub(descriptor.Supply, payload.Amount)

	// Encode the updated asset descriptor
	encoded, _ := descriptor.Bytes()

	// Update the asset entry in the asset account
	if err := assetacc.UpdateRegistryEntry(assetID, encoded); err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		status = common.ResultFuelExhausted
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(*descriptor.Supply),
	})

	opResult.SetStatus(status)

	return opResult
}
