package compute

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/state"
)

// RunAssetTransfer performs the given IxAssetTransfer operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have an AssetActionPayload and the output receipt will have a AssetTransferResult.
// The asset balance is debited from the sender/benefactor and credited to the Target state objects.
// Returns an error if any of given amounts are invalid (negative)
// or if the sender/benefactor does not have enough balance for that asset ID
func RunAssetTransfer(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	target := transition.GetObject(op.Target())
	sarga := transition.GetAuxiliaryObject(common.SargaAccountID)

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleParticipantCreate) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if payload.Benefactor.IsNil() {
		// Validate asset transfer payload
		if err := validateAssetTransfer(sender, target, sarga, payload); err != nil {
			return opResult.WithStatus(common.ResultStateReverted)
		}

		// Transfer the asset amount from the sender to target account
		if err := transferAsset(sender, target, payload); err != nil {
			return opResult.WithStatus(common.ResultStateReverted)
		}
	}

	if !payload.Benefactor.IsNil() {
		benefactor := transition.GetObject(payload.Benefactor)

		// Validate asset consume payload
		if err := validateAssetConsume(sender, target, sarga, benefactor, payload); err != nil {
			return opResult.WithStatus(common.ResultStateReverted)
		}

		// Transfer the asset amount from the benefactor to target account
		if err := consumeMandate(sender, target, benefactor, payload); err != nil {
			return opResult.WithStatus(common.ResultStateReverted)
		}
	}

	return opResult.WithStatus(common.ResultOk)
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

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the operator and asset account state objects
	operator := transition.GetObject(op.SenderID())
	assetacc := transition.GetObject(op.Target())

	// todo: [asset logics] handle logic deployment for logical assets
	// If logicCode is give, we need to compile it here and create the logic account.
	// The given logic code must also compile to an asset logic.
	// If the logicID is provided, we ignore any given code. We check if the logic id
	// is for an asset logic and check if such a logic exists.

	// todo: [asset logics] this is a simple value now, but will be include logic deployment cost
	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetCreation) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset create payload
	if err := validateAssetCreate(operator, identifiers.MustAssetID(op.Target())); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Create a new asset on the operator state object and get the asset ID
	assetID, err := createAsset(operator, assetacc, payload)
	if err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err = addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), assetacc.Identifier()); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetCreationResult{
		AssetID: assetID,
	})

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetApprove performs the given IxAssetApprove operation.
// The stateObjectRetriever must contain state objects for the sender, target, and Sarga accounts.
//
// The IxOp must have an AssetActionPayload and the output receipt will contain the result for asset approval.
// The sender authorizes the target to access or use a specified amount of their asset by creating a mandate.
// The mandate is recorded, allowing the target to access the asset in the future based on the sender's approval.
func RunAssetApprove(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleAssetTransfer) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset approve payload
	if err := validateAssetApprove(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Create an asset mandate for the target in the sender account
	if err := approveAsset(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetRevoke performs the given IxAssetRevoke operation.
// The stateObjectRetriever must contain state objects for the sender account.
//
// The IxOp must have an AssetActionPayload and the output receipt will contain a result for asset revocation.
// The asset mandate is revoked for the specified beneficiary, and the asset is no longer accessible for them.
// This operation ensures that the sender's mandates are appropriately updated in the state.
func RunAssetRevoke(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleAssetTransfer) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset revoke payload
	if err := validateAssetRevoke(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Delete the asset mandate from the sender account for the target
	if err := revokeAsset(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetMint performs the given IxAssetMint operation.
// The stateObjectRetriever must contain state objects for the operator and Target of the op.
//
// The IxOp must have an AssetSupplyPayload and the output receipt will have a AssetSupplyResult.
// The asset supply is increased and the new tokens are credited to the balance of the asset operator.
//
//nolint:dupl
func RunAssetMint(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset mint or burn Payload from the Interaction
	payload, _ := op.GetAssetSupplyPayload()

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.SenderID())
	assetacc := objects.GetObject(op.Target())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset mint payload
	if err := validateAssetMint(operator, assetacc, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Obtain the registry entry for the asset from the asset account
	supply, err := mintAsset(operator, assetacc, payload)
	if err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(supply),
	})

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetBurn performs the given IxAssetBurn operation.
// The stateObjectRetriever must contain state objects for the operator and Target of the op.
//
// The IxOp must have an AssetSupplyPayload and the output receipt will have a AssetSupplyResult.
// The asset supply is decreased and the tokens are debited from the balances of the asset operator.
//
//nolint:dupl
func RunAssetBurn(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Payload from the Interaction
	payload, _ := op.GetAssetSupplyPayload()

	//  Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the operator and asset account state objects
	operator := objects.GetObject(op.SenderID())
	assetacc := objects.GetObject(op.Target())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAssetSupplyModulate) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset burn payload
	if err := validateAssetBurn(operator, assetacc, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Burn the asset supply from asset account
	supply, err := burnAsset(operator, assetacc, payload)
	if err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Generate and set the result payload
	common.SetResultPayload(opResult, common.AssetSupplyResult{
		TotalSupply: (hexutil.Big)(supply),
	})

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetLockup performs the given IxAssetLockup operation.
// The stateObjectRetriever must contain the state object for the sender of the operation.
//
// The IxOp must have an AssetActionPayload, and the output receipt will reflect the lockup result.
// The specified asset amount is locked up in the sender's account for the target beneficiary.
func RunAssetLockup(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleAssetTransfer) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset lockup payload
	if err := validateAssetLockup(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Create a lockup in the sender's account for the specified target
	if err := lockupAsset(sender, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunAssetRelease performs the given IxAssetRelease operation.
// The stateObjectRetriever must contain state objects for the sender, target, sarga, and benefactor.
//
// The IxOp must have an AssetActionPayload, and the output receipt will reflect the release result.
// The specified asset amount is released from the benefactor's lockup to the target account.
func RunAssetRelease(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the Asset Transfer Payload from the Interaction
	payload, _ := op.GetAssetActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	target := transition.GetObject(op.Target())
	benefactor := transition.GetObject(payload.Benefactor)
	sarga := transition.GetAuxiliaryObject(common.SargaAccountID)

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleAssetTransfer) {
		opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate asset release payload
	if err := validateAssetRelease(sender, target, sarga, benefactor, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Transfer the lockup amount from the benefactor to target account
	if err := releaseAsset(sender, target, benefactor, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// helper function

// validateAssetCreate checks if the asset already exists and returns an error if it is already registered.
func validateAssetCreate(operator *state.Object, assetID identifiers.AssetID) error {
	// Check if the asset already exists
	assetObject, _ := operator.FetchAssetObject(assetID, true)
	if assetObject != nil {
		return common.ErrAssetAlreadyRegistered
	}

	return nil
}

// createAsset creates a new asset and assigns it to the operator.
func createAsset(operator, assetacc *state.Object, payload *common.AssetCreatePayload) (identifiers.AssetID, error) {
	// Generate a new Asset Descriptor
	descriptor := common.NewAssetDescriptor(operator.Identifier(), *payload)

	// Create a new asset on the asset state object and get the asset ID
	assetID, err := assetacc.CreateAsset(assetacc.Identifier(), descriptor)
	if err != nil {
		return identifiers.Nil, err
	}

	assetObject := state.NewAssetObject(descriptor.Supply, nil)

	// Create a new asset on the operator state object
	if err = operator.InsertNewAssetObject(assetID, assetObject); err != nil {
		return identifiers.Nil, err
	}

	// Registers a new asset in the operator's deed registry.
	if err = operator.CreateDeedsEntry(assetID.AsIdentifier()); err != nil {
		return identifiers.Nil, err
	}

	return assetID, nil
}

// validateAssetTransfer ensures the target is registered, asset exists, and sender has sufficient balance.
func validateAssetTransfer(sender, target, sarga *state.Object, payload *common.AssetActionPayload) error {
	// Check if the target account is registered
	// Fetch the account info from genesis state
	if _, err := sarga.GetStorageEntry(common.SargaLogicID, target.Identifier().Bytes()); err != nil {
		return common.ErrBeneficiaryNotRegistered
	}

	assetObject, err := sender.FetchAssetObject(payload.AssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if sender has sufficient balance
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// transferAsset transfers the asset balance from sender to target account.
func transferAsset(sender, target *state.Object, payload *common.AssetActionPayload,
) error {
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

// validateAssetConsume ensures the target is registered, mandate is valid, and benefactor has sufficient balance.
func validateAssetConsume(sender, target, sarga, benefactor *state.Object, payload *common.AssetActionPayload) error {
	// Check if the target account is registered
	// Fetch the account info from genesis state
	if _, err := sarga.GetStorageEntry(common.SargaLogicID, target.Identifier().Bytes()); err != nil {
		return common.ErrBeneficiaryNotRegistered
	}

	assetObject, err := benefactor.FetchAssetObject(payload.AssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if sender has sufficient balance
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	mandate, ok := assetObject.Mandate[sender.Identifier()]
	if !ok {
		return common.ErrMandateNotFound
	}

	if mandate.ExpiresAt < uint64(time.Now().Unix()) {
		return common.ErrMandateExpired
	}

	if mandate.Amount.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// consumeMandate transfers the asset balance from benefactor to target account.
func consumeMandate(sender, target, benefactor *state.Object, payload *common.AssetActionPayload) error {
	// Deduct the transfer amount from the benefactor's asset and mandate balance
	if err := benefactor.ConsumeMandate(payload.AssetID, sender.Identifier(), payload.Amount); err != nil {
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

// validateAssetApprove checks if the target is registered, asset exists, and sender has sufficient balance to approve.
func validateAssetApprove(sender *state.Object, payload *common.AssetActionPayload) error {
	assetObject, err := sender.FetchAssetObject(payload.AssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if sender has sufficient balance
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// approveAsset creates a mandate for the sender to approve a specified amount to the beneficiary.
func approveAsset(sender *state.Object, payload *common.AssetActionPayload) error {
	if err := sender.CreateMandate(payload.AssetID, payload.Beneficiary, payload.Amount, payload.Timestamp); err != nil {
		return err
	}

	return nil
}

// validateAssetRevoke ensures the sender has a valid mandate for the beneficiary to revoke.
func validateAssetRevoke(sender *state.Object, payload *common.AssetActionPayload) error {
	_, err := sender.GetMandate(payload.AssetID, payload.Beneficiary)
	if err != nil {
		return err
	}

	return nil
}

// revokeAsset deletes the mandate for the specified asset and beneficiary.
func revokeAsset(sender *state.Object, payload *common.AssetActionPayload) error {
	if err := sender.DeleteMandate(payload.AssetID, payload.Beneficiary); err != nil {
		return err
	}

	return nil
}

// validateAssetMint ensures the asset exists and the operator matches the specified asset operator for minting.
func validateAssetMint(operator, assetacc *state.Object, payload *common.AssetSupplyPayload) error {
	assetInfo, err := assetacc.GetState(payload.AssetID)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// only operator can mint asset
	if assetInfo.Operator != operator.Identifier() {
		return common.ErrOperatorMismatch
	}

	return nil
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

// validateAssetBurn ensures the asset exists, the operator matches, and the burn amount does not exceed
// the current balance.
func validateAssetBurn(operator, assetacc *state.Object, payload *common.AssetSupplyPayload) error {
	assetInfo, err := assetacc.GetState(payload.AssetID)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// only operator can burn asset
	if assetInfo.Operator != operator.Identifier() {
		return common.ErrOperatorMismatch
	}

	assetObject, err := operator.FetchAssetObject(payload.AssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// cannot burn amount greater than current balance
	if assetObject.Balance.Cmp(payload.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	return nil
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

// validateAssetLockup ensures the target account is registered, the asset exists, and the sender has
// sufficient balance to lock up the specified amount.
func validateAssetLockup(sender *state.Object, payload *common.AssetActionPayload) error {
	assetObject, err := sender.FetchAssetObject(payload.AssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if sender has sufficient balance
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	if _, ok := assetObject.Lockup[payload.Beneficiary]; ok {
		return common.ErrLockupAlreadyExists
	}

	return nil
}

// lockupAsset creates a lockup for the specified asset and amount.
func lockupAsset(sender *state.Object, payload *common.AssetActionPayload) error {
	if err := sender.CreateLockup(payload.AssetID, payload.Beneficiary, payload.Amount); err != nil {
		return err
	}

	return nil
}

// validateAssetRelease verifies that the target account is registered and that the benefactor has enough funds
// in the lockup to release the specified amount.
func validateAssetRelease(sender, target, sarga, benefactor *state.Object, payload *common.AssetActionPayload) error {
	// Check if the target account is registered
	// Fetch the account info from genesis state
	if _, err := sarga.GetStorageEntry(common.SargaLogicID, target.Identifier().Bytes()); err != nil {
		return common.ErrBeneficiaryNotRegistered
	}

	lockupAmount, err := benefactor.GetLockup(payload.AssetID, sender.Identifier())
	if err != nil {
		return common.ErrLockupNotFound
	}

	if lockupAmount.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// releaseAsset releases the specified amount from sender's lockup and updates the target's balance.
func releaseAsset(sender, target, benefactor *state.Object, payload *common.AssetActionPayload) error {
	if err := benefactor.ReleaseLockup(payload.AssetID, sender.Identifier(), payload.Amount); err != nil {
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
