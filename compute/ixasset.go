package compute

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunAssetCreate performs the given IxAssetCreate operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have an AssetCreatePayload and the output receipt will have a AssetCreationResult.
// The asset is created and an entry is registered on the registry of both the operator and target accounts.
// The created supply of the asset is credited to the balances of the asset operator.
func RunAssetCreate(
	op *common.IxOp,
	ctx *engineio.RuntimeContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Payload from the Interaction
	payload, _ := op.GetAssetCreatePayload()

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	assetID, _ := op.Target().AsAssetID()

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetCreation, 0) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Obtain the operator and asset account state objects
	operator, err := transition.GetObject(op.SenderID())
	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	asset, err := transition.GetObject(op.Target())
	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	sargaObject, err := transition.GetObject(common.SargaAccountID)
	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Validate asset create payload
	if err = validateAssetCreate(operator, identifiers.MustAssetID(op.Target()), payload); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	logicID := common.CreateLogicIDFromString(payload.Symbol, 0, identifiers.AssetLogical, identifiers.Systemic)

	// Create a new asset using asset engine
	_, err = ctx.Runtime.CreateAsset(
		op.Hash(),
		assetID,
		payload.Symbol, payload.Decimals, payload.Dimension,
		payload.Manager, op.SenderID(),
		payload.MaxSupply, payload.MetaData, payload.EnableEvents, logicID)
	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	var manifest []byte
	if payload.Standard == common.MASX {
		manifest = payload.Logic.Manifest
	} else {
		manifest, err = sargaObject.GetManifestForAsset(payload.Standard)
		if err != nil {
			return opResult.WithStatus(common.ResultExceptionRaised)
		}
	}

	consumption, result, logs, err := DeployLogic(ctx, op, manifest, asset, operator, transition, tank)
	// Exhaust fuel from tank
	if !tank.Exhaust(consumption.Compute, consumption.Storage) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Set the logs in the receipt
	opResult.SetLogs(logs)

	if result != nil {
		common.SetResultPayload(
			opResult,
			common.AssetCreationResult{
				AssetID: assetID,
				Error:   result.Error,
			})

		if result.Error != nil {
			return opResult.WithStatus(common.ResultExceptionRaised)
		}
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetCreationResult{
		AssetID: assetID,
	})

	return opResult.WithStatus(common.ResultOk)
}

// helper function

// validateAssetCreate checks if the asset already exists and returns an error if it is already registered.
func validateAssetCreate(operator *state.Object,
	assetID identifiers.AssetID, payload *common.AssetCreatePayload,
) error {
	// Check if the asset already exists
	assetObject, _ := operator.FetchAssetObject(assetID, true)
	if assetObject != nil {
		return common.ErrAssetAlreadyRegistered
	}

	_, ok := common.ValidAssetStandards[payload.Standard]
	if !ok {
		return common.ErrInvalidAssetStandard
	}

	if payload.Standard == common.MASX {
		lp := payload.Logic

		if lp == nil || len(lp.Manifest) == 0 {
			return common.ErrEmptyManifest
		}
	}

	return nil
}

// createAsset creates a new asset and assigns it to the sender.
func createAsset(sender, assetacc *state.Object, descriptor *common.AssetDescriptor) (identifiers.AssetID, error) {
	// Create a new asset on the asset state object and get the asset ID
	err := assetacc.CreateAsset(assetacc.Identifier(), descriptor)
	if err != nil {
		return identifiers.Nil, err
	}

	// Registers a new asset in the sender's deed registry.
	if err = sender.CreateDeedsEntry(descriptor.AssetID.AsIdentifier()); err != nil {
		return identifiers.Nil, err
	}

	return descriptor.AssetID, nil
}

// validateAssetTransfer ensures the target is registered, asset exists, and the benefactor has sufficient balance.
func validateAssetTransfer(benefactor, beneficiary, sarga *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	if amount.Cmp(big.NewInt(1)) == -1 {
		return common.ErrInvalidAmount
	}

	// Check if the beneficiary account is registered
	// Fetch the account info from genesis state
	ok, err := sarga.IsAccountRegistered(beneficiary.Identifier())
	if !ok || err != nil {
		return common.ErrBeneficiaryNotRegistered
	}

	assetObject, err := benefactor.FetchAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if err = assetObject.HasBalance(tokenID, amount); err != nil {
		return err
	}

	return nil
}

// transferAsset transfers the asset balance from sender to target account.
func transferAsset(sender, target *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	// Deduct the transfer amount from the sender's asset balance
	metadata, err := sender.SubBalance(assetID, tokenID, amount)
	if err != nil {
		return err
	}

	// Increment the asset balance if the asset already exists
	return target.AddBalance(assetID, tokenID, amount, metadata)
}

// validateAssetConsume ensures the target is registered, mandate is valid, and the benefactor has sufficient balance.
func validateAssetConsume(operatorID identifiers.Identifier, beneficiary, benefactor, sarga *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	// Check if the beneficiary account is registered
	// Fetch the account info from genesis state
	if _, err := sarga.GetStorageEntry(common.SargaLogicID.AsIdentifier(), beneficiary.Identifier().Bytes()); err != nil {
		return common.ErrBeneficiaryNotRegistered
	}

	assetObject, err := benefactor.FetchAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if err = assetObject.HasBalance(tokenID, amount); err != nil {
		return err
	}

	mandates, ok := assetObject.Mandate[operatorID]
	if !ok {
		return common.ErrMandateNotFound
	}

	mandate, ok := mandates[tokenID]
	if !ok {
		return common.ErrMandateNotFound
	}

	if mandate.ExpiresAt < uint64(time.Now().Unix()) {
		return common.ErrMandateExpired
	}

	if mandate.Amount.Cmp(amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// consumeMandate transfers the asset balance from benefactor to target account.
func consumeMandate(operatorID identifiers.Identifier, benefactor, beneficiary *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	// Deduct the transfer amount from the benefactor's asset and mandate balance
	metadata, err := benefactor.ConsumeMandate(assetID, tokenID, operatorID, amount)
	if err != nil {
		return err
	}

	// Increment the asset balance if the asset already exists
	return beneficiary.AddBalance(assetID, tokenID, amount, metadata)
}

// validateAssetApprove checks if the target is registered, asset exists, and sender has sufficient balance to approve.
func validateAssetApprove(sender *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	if amount.Cmp(big.NewInt(1)) == -1 {
		return common.ErrInvalidAmount
	}

	assetObject, err := sender.FetchAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if err = assetObject.HasBalance(tokenID, amount); err != nil {
		return err
	}

	return nil
}

// approveAsset creates a mandate for the sender to approve a specified amount to the beneficiary.
func approveAsset(benefactor *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID,
	beneficiary identifiers.Identifier, amount *big.Int, expiresAt uint64,
) error {
	if err := benefactor.CreateMandate(assetID, tokenID, beneficiary, amount, expiresAt); err != nil {
		return err
	}

	return nil
}

// validateAssetRevoke ensures the sender has a valid mandate for the beneficiary to revoke.
func validateAssetRevoke(benefactor *state.Object, beneficiary identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID,
) error {
	_, err := benefactor.GetMandate(assetID, tokenID, beneficiary)
	if err != nil {
		return err
	}

	return nil
}

// revokeAsset deletes the mandate for the specified asset and beneficiary.
func revokeAsset(benefactor *state.Object, beneficiary identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID,
) error {
	if err := benefactor.DeleteMandate(assetID, tokenID, beneficiary); err != nil {
		return err
	}

	return nil
}

// validateAssetMint ensures the asset exists and the beneficiary matches the specified asset manager for minting.
func validateAssetMint(senderID identifiers.Identifier, assetacc *state.Object,
	assetID identifiers.AssetID, amount *big.Int,
) error {
	assetInfo, err := assetacc.GetProperties(assetID)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if amount.Cmp(big.NewInt(1)) == -1 {
		return common.ErrInvalidAmount
	}

	if senderID != assetInfo.Manager {
		return common.ErrManagerMismatch
	}

	if new(big.Int).Add(assetInfo.CirculatingSupply, amount).Cmp(assetInfo.MaxSupply) > 0 {
		return common.ErrMaxSupplyReached
	}

	return nil
}

// mintAsset increases the asset supply and beneficiary's balance.
func mintAsset(beneficiary, assetacc *state.Object, assetID identifiers.AssetID,
	tokenID common.TokenID, amount *big.Int,
) error {
	// Mint the asset supply in asset account
	if _, err := assetacc.MintAsset(assetID, amount); err != nil {
		return err
	}

	// Credit the minted tokens to beneficiary account
	return beneficiary.AddBalance(assetID, tokenID, amount, nil)
}

// validateAssetBurn ensures the asset exists, the operator matches, and the burn amount does not exceed
// the current balance.
func validateAssetBurn(benefactor, assetacc *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	assetInfo, err := assetacc.GetProperties(assetID)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// only manager can burn asset
	if assetInfo.Manager != benefactor.Identifier() {
		return common.ErrManagerMismatch
	}

	assetObject, err := benefactor.FetchAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	bal, ok := assetObject.Balance[tokenID]
	if !ok {
		return common.ErrTokenNotFound
	}

	if amount.Cmp(big.NewInt(1)) == -1 {
		return common.ErrInvalidAmount
	}

	// cannot burn amount greater than current balance
	if bal.Cmp(amount) < 0 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// burnAsset reduces the asset supply and benefactor's balance.
func burnAsset(
	benefactor, assetacc *state.Object, assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	// Debit the tokens from benefactor account
	if _, err := benefactor.SubBalance(assetID, tokenID, amount); err != nil {
		return err
	}

	// Burn the supply in asset account
	_, err := assetacc.BurnAsset(assetID, amount)

	return err
}

// validateAssetLockup ensures the target account is registered, the asset exists, and the sender has
// sufficient balance to lock up the specified amount.
func validateAssetLockup(benefactor *state.Object, beneficiaryID identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	if beneficiaryID.IsNil() {
		return common.ErrInvalidBeneficiary
	}

	if amount.Cmp(big.NewInt(1)) == -1 {
		return common.ErrInvalidAmount
	}

	assetObject, err := benefactor.FetchAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	return assetObject.HasBalance(tokenID, amount)
}

// lockupAsset creates a lockup for the specified asset and amount.
func lockupAsset(benefactor *state.Object, beneficiary identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	return benefactor.CreateLockup(assetID, tokenID, beneficiary, amount)
}

// validateAssetRelease verifies that the target account is registered and that the benefactor has enough funds
// in the lockup to release the specified amount.
func validateAssetRelease(operatorID identifiers.Identifier, benefactor *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	lockup, err := benefactor.GetLockup(assetID, tokenID, operatorID)
	if err != nil {
		return common.ErrLockupNotFound
	}

	if lockup.Amount.Cmp(amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// releaseAsset releases the specified amount from sender's lockup and updates the target's balance.
func releaseAsset(operatorID identifiers.Identifier, benefactor, beneficiary *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) error {
	metadata, err := benefactor.ReleaseLockup(assetID, tokenID, operatorID, amount)
	if err != nil {
		return err
	}

	// Increment the asset balance if the asset already exists
	return beneficiary.AddBalance(assetID, tokenID, amount, metadata)
}
