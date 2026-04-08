package compute

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

func validateParticipantCreate(sender, target, sarga *state.Object, payload *common.ParticipantCreatePayload) error {
	// Check if the account is already registered
	// Fetch the account info from genesis state
	_, err := sarga.GetStorageEntry(common.SargaLogicID.AsIdentifier(), target.Identifier().Bytes())
	if !errors.Is(err, common.ErrKeyNotFound) {
		return common.ErrAlreadyRegistered
	}

	_, err = sender.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	return nil
}

func createAccountKeys(startID int, keysPayload []common.KeyAddPayload) common.AccountKeys {
	accountKeys := make(common.AccountKeys, len(keysPayload))

	for i, key := range keysPayload {
		accountKeys[i] = &common.AccountKey{
			ID:                 uint64(startID + i),
			PublicKey:          key.PublicKey,
			Weight:             key.Weight,
			SignatureAlgorithm: key.SignatureAlgorithm,
			Revoked:            false,
			SequenceID:         0,
		}
	}

	return accountKeys
}

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
	ctx *engineio.RuntimeContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetParticipantCreatePayload()

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the sender and target state objects
	sender, _ := transition.GetObject(op.SenderID())
	target, _ := transition.GetObject(op.Target())

	sarga, err := transition.GetObject(common.SargaAccountID)
	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelSimpleParticipantCreate, 0) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Validate participant create payload
	if err := validateParticipantCreate(sender, target, sarga, payload); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	if err := addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Register the target account by transferring the KMOI asset from sender to target

	result := ctx.Runtime.Call(common.KMOITokenAccountID, op, transition, &engineio.FuelGauge{
		Compute: tank.ComputeCapacity,
		Storage: tank.StorageCapacity, // TODO: Fix this
	})

	if !tank.Exhaust(result.ComputeEffort, result.StorageEffort) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	opResult.SetLogs(result.Logs)

	if result.IsError() {
		common.SetResultPayload(opResult, common.AccountCreationResult{Error: result.Err})

		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	accountKeys := createAccountKeys(0, payload.KeysPayload)

	target.UpdateKeys(accountKeys)

	common.SetResultPayload(opResult, common.AccountCreationResult{
		AccountID: target.Identifier(),
	})

	return opResult.WithStatus(common.ResultOk)
}

func validateAccRevoke(keysCount uint64, revoke []common.KeyRevokePayload) bool {
	for _, revokeKey := range revoke {
		if revokeKey.KeyID >= keysCount {
			return false
		}
	}

	return true
}

func RunAccountConfigure(
	op *common.IxOp,
	_ *engineio.RuntimeContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetAccountConfigurePayload()

	// Obtain the sender and target state objects
	sender, _ := transition.GetObject(op.SenderID())

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAccountConfigure, 0) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	keysCount := sender.KeysLen()

	if len(payload.Add) > 0 {
		accountKeys := createAccountKeys(keysCount, payload.Add)

		if err := sender.AppendAccountKeys(accountKeys); err != nil {
			return opResult.WithStatus(common.ResultExceptionRaised)
		}

		return opResult.WithStatus(common.ResultOk)
	}

	if !validateAccRevoke(uint64(keysCount), payload.Revoke) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	if err := sender.RevokeAccountKeys(payload.Revoke); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	return opResult.WithStatus(common.ResultOk)
}

func validateAccountInherit(sender, sarga *state.Object, logicID identifiers.Identifier,
	payload *common.AccountInheritPayload,
) error {
	// Check if the account is already registered
	// Fetch the account info from genesis state
	if _, err := sarga.GetStorageEntry(common.SargaLogicID.AsIdentifier(), logicID.Bytes()); err != nil {
		return common.ErrTargetAccountNotFound
	}

	_, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if int(payload.SubAccountIndex) != sender.SubAccountCount()+1 {
		return common.ErrInvalidSubAccountCount
	}

	return nil
}

func RunAccountInherit(
	op *common.IxOp,
	ctx *engineio.RuntimeContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Obtain the participant create Payload from the Interaction
	payload, _ := op.GetAccountInheritPayload()

	sender, _ := transition.GetObject(op.SenderID())
	sarga, _ := transition.GetObject(common.SargaAccountID)
	logicID := payload.TargetAccount
	subAccount, _ := transition.GetObject(op.Target())

	// Create a new result for the op
	opResult := common.NewIxOpResult(op.Type())

	// Exhaust fuel from tank
	if !tank.Exhaust(FuelAccountInherit, 0) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Validate account inherit payload
	if err := validateAccountInherit(sender, sarga, logicID, payload); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	_ = sender.UpdateSubAccount(subAccount.Identifier(), logicID)

	if err := addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	result := ctx.Runtime.Call(common.KMOITokenAccountID, op, transition, &engineio.FuelGauge{
		Compute: tank.ComputeCapacity,
		Storage: tank.StorageCapacity, // TODO: Fix this
	})

	if !tank.Exhaust(result.ComputeEffort, result.StorageEffort) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	opResult.SetLogs(result.Logs)

	if result.IsError() {
		common.SetResultPayload(opResult, common.AccountInheritResult{Error: result.Err})

		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	subAccount.InheritAccount(payload, sender)

	common.SetResultPayload(opResult, common.AccountInheritResult{
		SubAccount: subAccount.Identifier(),
	})

	return opResult.WithStatus(common.ResultOk)
}
