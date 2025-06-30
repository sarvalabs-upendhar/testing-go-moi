package compute

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// validateGuardianRegister checks if the guardian node can be registered by verifying uniqueness and sufficient funds.
func validateGuardianRegister(
	sender *state.Object,
	system *state.SystemObject,
	payload *common.GuardianRegisterPayload,
) error {
	// Check if a validator already exists
	if _, err := system.ValidatorByKramaID(payload.KramaID); err == nil {
		return common.ErrGuardianExists
	}

	assetObject, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if the sender has enough balance to cover the required amount
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// registerGuardian registers a new validator node in the system and stakes the specified amount.
func registerGuardian(sender *state.Object, system *state.SystemObject, payload *common.GuardianRegisterPayload) error {
	// Lock the specified amount of KMOI tokens for the GuardianAccount
	err := sender.CreateLockup(
		common.KMOITokenAssetID,
		common.GuardianAccountID,
		payload.Amount,
	)
	if err != nil {
		return err
	}

	// Append the new validator entry to the validator registry
	err = system.AppendValidator(common.NewValidator(
		common.ValidatorIndex(system.TotalValidators()),
		payload.KramaID, payload.Amount, payload.WalletID,
		payload.ConsensusKey, payload.KYCProof, common.KYCStatus(1)),
	)
	if err != nil {
		return err
	}

	return nil
}

// validateGuardianStake ensures that a guardian node is eligible to perform a staking action.
func validateGuardianStake(
	sender *state.Object,
	system *state.SystemObject,
	payload *common.GuardianActionPayload,
) error {
	// Check if the validator with the given KramaID exists
	if _, err := system.ValidatorByKramaID(payload.KramaID); err != nil {
		return err
	}

	// Fetch the sender's KMOI asset object
	assetObject, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if the sender has enough balance to stake the requested amount
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// stakeGuardian updates the validator's pending stake.
func stakeGuardian(sender *state.Object, system *state.SystemObject, payload *common.GuardianActionPayload) error {
	// Lock the specified amount of KMOI tokens for the GuardianAccount
	err := sender.CreateLockup(
		common.KMOITokenAssetID,
		common.GuardianAccountID,
		payload.Amount,
	)
	if err != nil {
		return err
	}

	// Retrieve the validator associated with the given KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	copiedValidator := validator.Copy()

	// Increment the pending stake amount for the validator
	copiedValidator.PendingStakeAdditions.Add(copiedValidator.PendingStakeAdditions, payload.Amount)

	// Update the validator entry in the system with the new stake value
	if err = system.UpdateValidator(uint64(copiedValidator.ID), copiedValidator); err != nil {
		return err
	}

	return nil
}

// validateGuardianUnstake checks if the validator exists and has enough active stake to unstake.
func validateGuardianUnstake(system *state.SystemObject, payload *common.GuardianActionPayload) error {
	// Retrieve the validator associated with the given KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	// Ensure the validator has enough active stake to cover the unstake request
	if validator.ActiveStake.Cmp(payload.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// unstakeGuardian schedules a stake removal for the validator by recording the unstake amount.
func unstakeGuardian(system *state.SystemObject, payload *common.GuardianActionPayload) error {
	// Retrieve the validator associated with the given KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	copiedValidator := validator.Copy()

	// Schedule the stake removal for a future epoch
	// Todo: The EPOCH value has to be updated later once the epoch logic is implemented
	copiedValidator.PendingStakeRemovals[common.Epoch(0)] = payload.Amount

	// Update the validator entry in the system with the updated stake removal record
	if err = system.UpdateValidator(uint64(copiedValidator.ID), copiedValidator); err != nil {
		return err
	}

	return nil
}

// validateGuardianWithdraw verifies if the validator is eligible to withdraw stake.
func validateGuardianWithdraw(
	sender *state.Object,
	system *state.SystemObject,
	payload *common.GuardianActionPayload,
) error {
	// Retrieve the validator associated with the given KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	// Check if the sender matches the validator's wallet address
	if validator.WalletAddress != sender.Identifier() {
		return common.ErrInvalidIdentifier
	}

	// Ensure the validator has enough inactive stake to withdraw
	if validator.InactiveStake.Cmp(payload.Amount) == -1 {
		return errors.New("insufficient inactive stake")
	}

	// Check if the sender has sufficient lockup balance in the Guardian account
	lockupAmount, err := sender.GetLockup(common.KMOITokenAssetID, common.GuardianAccountID)
	if err != nil {
		return common.ErrLockupNotFound
	}

	// Check if the sender has sufficient amount locked up
	if lockupAmount.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// withdrawStake releases locked tokens back to the sender and updates the validator's inactive stake.
func withdrawStake(sender *state.Object, system *state.SystemObject, payload *common.GuardianActionPayload) error {
	// Release the specified amount of KMOI tokens locked under the GuardianAccount
	err := sender.ReleaseLockup(
		common.KMOITokenAssetID, common.GuardianAccountID,
		payload.Amount,
	)
	if err != nil {
		return err
	}

	// Retrieve the validator by KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	copiedValidator := validator.Copy()

	// Deduct the withdrawn amount from the validator's inactive stake
	copiedValidator.InactiveStake.Sub(copiedValidator.InactiveStake, payload.Amount)

	// Update the validator's state with the new inactive stake amount
	if err = system.UpdateValidator(uint64(copiedValidator.ID), copiedValidator); err != nil {
		return err
	}

	// Add the withdrawn amount back to the sender's available balance
	if err = sender.AddBalance(common.KMOITokenAssetID, payload.Amount); err != nil {
		return err
	}

	return nil
}

// validateGuardianClaim checks if the claim is authorized and funds are sufficient.
func validateGuardianClaim(
	sender *state.Object,
	system *state.SystemObject,
	payload *common.GuardianActionPayload,
) error {
	// Retrieve the validator associated with the given KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	// Check if the sender's id matches the validator's wallet id
	if validator.WalletAddress != sender.Identifier() {
		return common.ErrInvalidIdentifier
	}

	// Ensure if the validator has sufficient rewards to cover the claim
	if validator.Rewards.Cmp(payload.Amount) == -1 {
		return errors.New("insufficient rewards")
	}

	// Fetch the system's KMOI token asset object to verify available balance
	assetObject, err := system.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if the system has sufficient KMOI tokens to cover the claim
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

func claimRewards(sender *state.Object, system *state.SystemObject, payload *common.GuardianActionPayload) error {
	// Deduct the transfer amount from the system's asset balance
	if err := system.SubBalance(common.KMOITokenAssetID, payload.Amount); err != nil {
		return err
	}

	// Retrieve the validator by KramaID
	validator, err := system.ValidatorByKramaID(payload.KramaID)
	if err != nil {
		return err
	}

	copiedValidator := validator.Copy()

	// Deduct the claimed amount from the validator's accumulated rewards
	copiedValidator.Rewards.Sub(copiedValidator.Rewards, payload.Amount)

	// Update the validator state with the new rewards balance
	if err = system.UpdateValidator(uint64(copiedValidator.ID), copiedValidator); err != nil {
		return err
	}

	// Add the claimed rewards amount to the sender's balance
	if err = sender.AddBalance(common.KMOITokenAssetID, payload.Amount); err != nil {
		return err
	}

	return nil
}

// RunGuardianRegister handles the execution of a guardian registration operation.
func RunGuardianRegister(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetGuardianRegisterPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	system := transition.GetSystemObject()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelGuardianRegister) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if err := validateGuardianRegister(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := registerGuardian(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunGuardianStake handles execution of a guardian stake operation.
func RunGuardianStake(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetGuardianActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	system := transition.GetSystemObject()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelGuardianStake) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if err := validateGuardianStake(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := stakeGuardian(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunGuardianUnstake handles execution of a guardian unstake operation.
func RunGuardianUnstake(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetGuardianActionPayload()

	// Obtain the sender and target state objects
	system := transition.GetSystemObject()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelGuardianUnstake) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if err := validateGuardianUnstake(system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := unstakeGuardian(system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunGuardianWithdraw handles execution of a guardian withdraw operation.
func RunGuardianWithdraw(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetGuardianActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	system := transition.GetSystemObject()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelGuardianWithdraw) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if err := validateGuardianWithdraw(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := withdrawStake(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

// RunGuardianClaim handles execution of a guardian rewards claim operation.
func RunGuardianClaim(
	op *common.IxOp,
	_ *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetGuardianActionPayload()

	// Obtain the sender and target state objects
	sender := transition.GetObject(op.SenderID())
	system := transition.GetSystemObject()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelGuardianClaim) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	if err := validateGuardianClaim(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := claimRewards(sender, system, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}
