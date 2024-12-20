package compute

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// RunParticipantCreate performs the given IxParticipantCreate operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have an ParticipantCreatePayload and the output receipt will have a ParticipantCreateResult.
// The KMOI asset balance is debited from the sender and credited to the Target state objects.
// Returns an error if any of given amounts are invalid (negative)
// or if the sender does not have enough balance for that asset ID
// or if the KMOI asset object already exists in the target account
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
	sarga := transition.GetObject(common.SargaAddress)

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleParticipantCreate) {
		return opResult.WithStatus(common.ResultFuelExhausted)
	}

	// Validate participant create payload
	if err := validateParticipantCreate(sender, target, sarga, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	// Register the target account by creating and inserting a new KMOI asset object
	// into the target account's asset tree.
	if err := createParticipant(sender, target, payload); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	if err := addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		return opResult.WithStatus(common.ResultStateReverted)
	}

	return opResult.WithStatus(common.ResultOk)
}

func validateParticipantCreate(sender, target, sarga *state.Object, payload *common.ParticipantCreatePayload) error {
	// Check if the account is already registered
	// Fetch the account info from genesis state
	_, err := sarga.GetStorageEntry(common.SargaLogicID, target.Address().Bytes())
	if !errors.Is(err, common.ErrKeyNotFound) {
		return common.ErrAlreadyRegistered
	}

	assetObject, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Check if sender has sufficient balance
	if assetObject.Balance.Cmp(payload.Amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

// createParticipant registers a new participant by inserting a KMOI asset object into the target account's asset tree.
func createParticipant(sender, target *state.Object, payload *common.ParticipantCreatePayload) error {
	// Deduct the transfer amount from the sender's asset balance
	if err := sender.SubBalance(common.KMOITokenAssetID, payload.Amount); err != nil {
		return err
	}

	// Insert a new asset object with the specified amount into the target's asset tree.
	return target.InsertNewAssetObject(common.KMOITokenAssetID, state.NewAssetObject(payload.Amount, nil))
}
