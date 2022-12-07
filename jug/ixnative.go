package jug

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/types"
)

// ValueTransfer performs the IxValueTransfer interaction on the given sender and receiver StateObjects.
// The given amount for the given assetID is decremented from the sender and incremented on the receiver.
// Returns an error if the given amount is invalid (negative) or if the sender does not have enough balance.
func ValueTransfer(sender, receiver *guna.StateObject, assetID types.AssetID, amount *big.Int) (uint64, error) {
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
// created in the state object of the creator (sender of interaction)
func CreateAsset(creator *guna.StateObject, assetSpec *types.AssetDataInput) (uint64, string, error) {
	// Create a new asset on the creator state object and get the asset ID
	assetID, err := creator.CreateAsset(
		uint8(assetSpec.Dimension),
		assetSpec.IsFungible,
		assetSpec.IsMintable,
		assetSpec.Symbol,
		assetSpec.TotalSupply,
		assetSpec.Code,
	)
	if err != nil {
		return 0, "", err
	}

	// Return the string form of the asset ID
	return 1, string(assetID), nil
}
