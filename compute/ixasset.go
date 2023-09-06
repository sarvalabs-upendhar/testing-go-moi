package compute

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/state"
)

// RunAssetTransfer performs the given IxAssetTransfer interaction.
// The stateObjectRetriever must contain state objects for the sender and receiver of the Interaction.
//
// Each entry in the TransferValues of the Interaction is considered as a transfer
// request and debited from the sender and credited to the receiver state objects.
//
// Returns an error if any of given amounts are invalid (negative)
// or if the sender does not have enough balance for that asset ID
func RunAssetTransfer(
	ix *common.Interaction,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects state.ObjectMap,
) (
	*common.Receipt, error,
) {
	// Obtain the sender and receiver state objects
	sender := objects.GetObject(ix.Sender())
	receiver := objects.GetObject(ix.Receiver())

	// For each asset, apply the value transfer routine for the given transfer amount.
	for assetID, amount := range ix.TransferValues() {
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

		// Exhaust fuel from tank
		if !tank.Exhaust(FuelSimpleValueTransfer) {
			return nil, common.ErrInsufficientFuel
		}
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	return receipt, nil
}

// RunAssetCreate performs the given IxAssetCreate interaction.
// The stateObjectRetriever must contain state objects for the sender and receiver of the Interaction.
//
// The Interaction must have an AssetCreatePayload and the output receipt will have a AssetCreationReceipt.
// The asset is created and an entry is registered on the registry of both the creator and target accounts.
// The created supply of the asset is credited to the balances of the asset creator.
func RunAssetCreate(
	ix *common.Interaction,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects state.ObjectMap,
) (*common.Receipt, error) {
	// Obtain the Asset Payload from the Interaction
	payload, err := ix.GetAssetPayload()
	if err != nil {
		return nil, errors.Wrap(err, "could not find asset payload")
	}

	// Check that AssetCreate payload is available
	if payload.Create == nil {
		return nil, errors.Errorf("asset create payload is empty")
	}

	// Obtain the creator and asset account state objects
	creator := objects.GetObject(ix.Sender())
	assetacc := objects.GetObject(ix.Receiver())

	// Generate a new Asset Descriptor
	descriptor := common.NewAssetDescriptor(creator.Address(), *payload.Create)

	// todo: [asset logics] handle logic deployment for logical assets
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.

	// Create a new asset on the creator state object and get the asset ID
	assetID, err := creator.CreateAsset(assetacc.Address(), descriptor)
	if err != nil {
		return nil, err
	}

	// Credit the created asset balance to the creator
	creator.AddBalance(assetID, descriptor.Supply)

	// Generate the asset entry on the target object
	if _, err = assetacc.CreateAsset(assetacc.Address(), descriptor); err != nil {
		return nil, err
	}

	// todo: [asset logics] this is a simple value now, but will be include logic deployment cost
	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetCreation) {
		return nil, common.ErrInsufficientFuel
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	// Generate the asset create receipt
	receiptPayload := &common.AssetCreationReceipt{AssetID: assetID, AssetAccount: assetacc.Address()}
	// Set the extra data of the receipt
	if err = receipt.SetExtraData(receiptPayload); err != nil {
		return nil, err
	}

	return receipt, nil
}

func RunAssetMint(
	ix *common.Interaction,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects state.ObjectMap,
) (*common.Receipt, error) {
	// Obtain the Asset Payload from the Interaction
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return nil, errors.Wrap(err, "could not find asset payload")
	}

	// Check that AssetMintOrBurn payload is available
	if assetPayload.Mint == nil {
		return nil, errors.Errorf("asset mint/burn payload is empty")
	}

	// Obtain the mint payload and the asset ID
	payload := *assetPayload.Mint
	assetID := payload.Asset.String()

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(ix.Sender())
	assetacc := objects.GetObject(ix.Receiver())

	// todo: check if asset standard allows supply modulation. must be MAS0

	// Obtain the registry entry for the asset from the asset account
	assetEntry, err := assetacc.GetRegistryEntry(assetID)
	if err != nil {
		return nil, err
	}

	// Decode the asset entry into a AssetDescriptor
	descriptor := new(common.AssetDescriptor)
	if err = descriptor.FromBytes(assetEntry); err != nil {
		return nil, err
	}

	// Update the supply on the descriptor
	descriptor.Supply.Add(descriptor.Supply, payload.Amount)

	// Encode the updated asset descriptor
	encoded, err := descriptor.Bytes()
	if err != nil {
		return nil, err
	}

	// Update the asset entry in the asset account
	if err = assetacc.UpdateRegistryEntry(assetID, encoded); err != nil {
		return nil, err
	}

	// Credit the minted tokens to operator account
	operator.AddBalance(payload.Asset, payload.Amount)

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		return nil, common.ErrInsufficientFuel
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	// Generate the asset mint receipt
	receiptPayload := &common.AssetMintOrBurnReceipt{TotalSupply: (hexutil.Big)(*descriptor.Supply)}
	// Set the extra data of the receipt
	if err = receipt.SetExtraData(receiptPayload); err != nil {
		return nil, err
	}

	return receipt, nil
}

func RunAssetBurn(
	ix *common.Interaction,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects state.ObjectMap,
) (*common.Receipt, error) {
	// Obtain the Asset Payload from the Interaction
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return nil, errors.Wrap(err, "could not find asset payload")
	}

	// Check that AssetMintOrBurn payload is available
	if assetPayload.Mint == nil {
		return nil, errors.Errorf("asset mint/burn payload is empty")
	}

	// Obtain the mint payload and the asset ID
	payload := *assetPayload.Mint
	assetID := payload.Asset.String()

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(ix.Sender())
	assetacc := objects.GetObject(ix.Receiver())

	// todo: check if asset standard allows supply modulation. must be MAS0

	// Obtain the operator's balance of the asset
	balance, err := operator.BalanceOf(payload.Asset)
	if err != nil {
		return nil, err
	}

	// Check that the operator has enough balance for the burn
	if balance.Cmp(payload.Amount) < 0 {
		return nil, common.ErrInsufficientFunds
	}

	// Burn the tokens from operator account
	operator.SubBalance(payload.Asset, payload.Amount)

	// Obtain the registry entry for the asset from the asset account
	assetEntry, err := assetacc.GetRegistryEntry(assetID)
	if err != nil {
		return nil, err
	}

	// Decode the asset entry into a AssetDescriptor
	descriptor := new(common.AssetDescriptor)
	if err = descriptor.FromBytes(assetEntry); err != nil {
		return nil, err
	}

	// Update the asset entry in the asset account
	descriptor.Supply.Sub(descriptor.Supply, payload.Amount)

	// Encode the updated asset descriptor
	encoded, err := descriptor.Bytes()
	if err != nil {
		return nil, err
	}

	// Update the asset entry in the asset account
	if err = assetacc.UpdateRegistryEntry(assetID, encoded); err != nil {
		return nil, err
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		return nil, common.ErrInsufficientFuel
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	// Generate the asset burn receipt
	receiptPayload := &common.AssetMintOrBurnReceipt{TotalSupply: (hexutil.Big)(*descriptor.Supply)}
	// Set the extra data of the receipt
	if err = receipt.SetExtraData(receiptPayload); err != nil {
		return nil, err
	}

	return receipt, nil
}
