package compute

import (
	"math/big"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/state"
)

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

	// Transfer the asset amount from the sender to target account
	if err := transferAsset(sender, target, payload); err != nil {
		status = common.ResultStateReverted
	}

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
// The asset is created and an entry is registered on the registry of both the operator and target accounts.
// The created supply of the asset is credited to the balances of the asset operator.
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

	// Obtain the operator and asset account state objects
	operator := transition.GetObject(op.Sender())
	assetacc := transition.GetObject(op.Target())

	// todo: [asset logics] handle logic deployment for logical assets
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.

	// Create a new asset on the operator state object and get the asset ID
	assetID, err := createAsset(operator, assetacc, payload)
	if err != nil {
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
// The asset supply is increased and the new tokens are credited to the balance of the asset operator.
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

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.Sender())
	assetacc := objects.GetObject(op.Target())

	// Obtain the registry entry for the asset from the asset account
	supply, err := mintAsset(operator, assetacc, payload)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		status = common.ResultFuelExhausted
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(supply),
	})

	opResult.SetStatus(status)

	return opResult
}

// RunAssetBurn performs the given IxAssetBurn operation.
// The stateObjectRetriever must contain state objects for the operator and Target of the op.
//
// The IxOp must have an AssetSupplyPayload and the output receipt will have a AssetSupplyResult.
// The asset supply is decreased and the tokens are debited from the balances of the asset operator.
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

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.Sender())
	assetacc := objects.GetObject(op.Target())

	// Burn the asset supply from asset account
	supply, err := burnAsset(operator, assetacc, payload)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		status = common.ResultFuelExhausted
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(supply),
	})

	opResult.SetStatus(status)

	return opResult
}

// helper function

// createAsset creates a new asset and assigns it to the operator.
func createAsset(operator, assetacc *state.Object, payload *common.AssetCreatePayload) (identifiers.AssetID, error) {
	// Generate a new Asset Descriptor
	descriptor := common.NewAssetDescriptor(operator.Address(), *payload)

	// Create a new asset on the asset state object and get the asset ID
	assetID, err := assetacc.CreateAsset(assetacc.Address(), descriptor)
	if err != nil {
		return "", err
	}

	assetObject := state.NewAssetObject(descriptor.Supply, nil)

	// Create a new asset on the operator state object
	if err = operator.InsertNewAssetObject(assetID, assetObject); err != nil {
		return "", err
	}

	// Registers a new asset in the operator's deeds registry.
	if err = operator.CreateDeedsEntry(string(assetID)); err != nil {
		return "", err
	}

	return assetID, nil
}

// transferAsset transfers the asset balance from sender to target account.
func transferAsset(sender, target *state.Object, payload *common.AssetActionPayload) error {
	// Deduct the transfer amount from the sender's asset balance
	if err := sender.SubBalance(payload.AssetID, payload.Amount); err != nil {
		return err
	}

	assetObject, _ := target.FetchAssetObject(payload.AssetID, true)
	if assetObject == nil {
		// Insert a new asset object if the asset doesn't exist
		return target.InsertNewAssetObject(payload.AssetID, state.NewAssetObject(payload.Amount, nil))
	}

	// Increment the asset balance if the asset already exists
	return target.AddBalance(payload.AssetID, payload.Amount)
}

// mintAsset increases the asset supply and operator's balance.
func mintAsset(operator, assetacc *state.Object, payload *common.AssetSupplyPayload) (big.Int, error) {
	// Mint the asset supply in asset account
	supply, err := assetacc.MintAsset(payload.AssetID, payload.Amount)
	if err != nil {
		return *big.NewInt(0), err
	}

	// Credit the minted tokens to operator account
	if err = operator.AddBalance(payload.AssetID, payload.Amount); err != nil {
		return *big.NewInt(0), err
	}

	return supply, nil
}

// burnAsset reduces the asset supply and operator's balance.
func burnAsset(operator, assetacc *state.Object, payload *common.AssetSupplyPayload) (big.Int, error) {
	// Debit the tokens from operator account
	if err := operator.SubBalance(payload.AssetID, payload.Amount); err != nil {
		return *big.NewInt(0), err
	}

	// Burn the supply in asset account
	supply, err := assetacc.BurnAsset(payload.AssetID, payload.Amount)
	if err != nil {
		return *big.NewInt(0), err
	}

	return supply, nil
}
