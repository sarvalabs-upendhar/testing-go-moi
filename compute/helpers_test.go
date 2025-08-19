package compute

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestStateObject(t *testing.T) *state.Object {
	t.Helper()

	id := tests.RandomIdentifier(t)

	return state.NewStateObject(
		id, nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)
}

func createTestSystemObject(t *testing.T) *state.SystemObject {
	t.Helper()

	return state.NewSystemObject(createTestStateObject(t))
}

func createTestAsset(t *testing.T, supply *big.Int) (*state.Object, *state.Object, identifiers.AssetID) {
	t.Helper()

	creator := state.NewStateObject(
		tests.RandomIdentifier(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	asset := state.NewStateObject(
		tests.RandomIdentifier(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	assetID, err := createAsset(creator, asset, &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	})

	require.NoError(t, err)

	_, err = creator.Commit()
	require.NoError(t, err)

	_, err = asset.Commit()
	require.NoError(t, err)

	return creator, asset, assetID
}

func checkAssetCreate(
	t *testing.T, assetID identifiers.AssetID,
	creatorAcc, assetAcc *state.Object,
	payload *common.AssetCreatePayload,
) {
	t.Helper()

	creatorObject, err := creatorAcc.FetchAssetObject(assetID, true)

	require.NoError(t, err)
	require.Equal(t, creatorObject.Balance, payload.Supply)

	assetObject, err := assetAcc.FetchAssetObject(assetID, true)

	require.NoError(t, err)
	require.Equal(t, assetObject.Properties.Supply, payload.Supply)
	require.Equal(t, assetObject.Properties.Symbol, payload.Symbol)
	require.Equal(t, assetObject.Properties.Standard, payload.Standard)
}

func checkAssetTransfer(
	t *testing.T, assetID identifiers.AssetID,
	sender, target *state.Object,
	expectedSenderBalance, expectedTargetBalance *big.Int,
) {
	t.Helper()

	senderObject, err := sender.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	targetObject, err := target.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	require.Equal(t, 0, expectedSenderBalance.Cmp(senderObject.Balance))
	require.Equal(t, 0, expectedTargetBalance.Cmp(targetObject.Balance))
}

func checkMandateConsumption(
	t *testing.T, assetID identifiers.AssetID, sender, target, benefactor *state.Object,
	expectedBenefactorBalance, expectedTargetBalance, expectedMandateBalance *big.Int,
) {
	t.Helper()

	targetObject, err := target.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	benefactorObject, err := benefactor.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	require.Equal(t, 0, expectedBenefactorBalance.Cmp(benefactorObject.Balance))
	require.Equal(t, 0, expectedTargetBalance.Cmp(targetObject.Balance))

	require.NotNil(t, benefactorObject.Mandate[sender.Identifier()])
	require.Equal(t, 0, expectedMandateBalance.Cmp(benefactorObject.Mandate[sender.Identifier()].Amount))
}

func checkAssetApprove(t *testing.T, sender *state.Object, payload *common.AssetActionPayload) {
	t.Helper()

	mandate, err := sender.GetMandate(payload.AssetID, payload.Beneficiary)
	require.NoError(t, err)
	require.Equal(t, payload.Amount, mandate.Amount)
}

func checkAssetLockup(t *testing.T, sender *state.Object, payload *common.AssetActionPayload) {
	t.Helper()

	lockupAmount, err := sender.GetLockup(payload.AssetID, payload.Beneficiary)
	require.NoError(t, err)
	require.Equal(t, payload.Amount, lockupAmount)
}

func checkAssetRevoke(t *testing.T, sender *state.Object, payload *common.AssetActionPayload) {
	t.Helper()

	_, err := sender.GetMandate(payload.AssetID, payload.Beneficiary)
	require.Error(t, err)
}

func checkAssetRelease(
	t *testing.T, sender, beneficiary, benefactor *state.Object,
	payload *common.AssetActionPayload, expectedAmount *big.Int,
) {
	t.Helper()

	// Check the beneficiary account
	ao, err := beneficiary.FetchAssetObject(payload.AssetID, true)
	require.NoError(t, err)
	require.Equal(t, payload.Amount, ao.Balance)

	// Check whether the lockup amount got deducted
	lockupAmount, err := benefactor.GetLockup(payload.AssetID, sender.Identifier())
	require.NoError(t, err)
	require.Equal(t, expectedAmount, lockupAmount)
}

func createTestAssetID(
	t *testing.T, id identifiers.Identifier,
	payload *common.AssetCreatePayload,
) identifiers.AssetID {
	t.Helper()

	assetID, err := identifiers.GenerateAssetIDv0(
		id.Fingerprint(),
		id.Variant(),
		uint16(payload.Standard),
		payload.Flags()...,
	)

	require.NoError(t, err)

	return assetID
}

func createMandate(t *testing.T, sender *state.Object, payload *common.AssetActionPayload) {
	t.Helper()

	err := sender.CreateMandate(payload.AssetID, payload.Beneficiary, payload.Amount, payload.Timestamp)
	assert.NoError(t, err)
}

func insertTestAssetObject(
	t *testing.T, creatorAcc *state.Object,
	assetID identifiers.AssetID, assetObject *state.AssetObject,
) {
	t.Helper()

	err := creatorAcc.InsertNewAssetObject(assetID, assetObject)
	assert.NoError(t, err)
}

func insertGuardianEntry(
	t *testing.T, system *state.SystemObject, validator *common.Validator,
) {
	t.Helper()

	err := system.SetValidators([]*common.Validator{validator})
	require.NoError(t, err)
}

func createTestDeedsEntry(
	t *testing.T, id identifiers.Identifier,
	creatorAcc *state.Object, payload *common.AssetCreatePayload,
) {
	t.Helper()

	assetID, err := identifiers.GenerateAssetIDv0(
		id.Fingerprint(),
		id.Variant(),
		uint16(payload.Standard),
		payload.Flags()...,
	)

	require.NoError(t, err)

	err = creatorAcc.CreateDeedsEntry(assetID.AsIdentifier())
	assert.NoError(t, err)
}

func createTestSargaStateObject(t *testing.T) *state.Object {
	t.Helper()

	sarga := state.NewStateObject(common.SargaAccountID, nil, nil, nil, common.Account{
		AccType: common.SargaAccount,
	}, state.NilMetrics(), false)

	err := sarga.CreateStorageTreeForLogic(common.SargaLogicID)
	require.NoError(t, err)

	return sarga
}

func registerParticipant(t *testing.T, sarga *state.Object, id identifiers.Identifier) {
	t.Helper()

	err := sarga.SetStorageEntry(common.SargaLogicID, id.Bytes(), id.Bytes())
	assert.NoError(t, err)
}

func createTestMandate(
	t *testing.T, sender, beneficiary *state.Object,
	assetID identifiers.AssetID, amount *big.Int, timestamp uint64,
) {
	t.Helper()

	err := approveAsset(sender, &common.AssetActionPayload{
		Beneficiary: beneficiary.Identifier(),
		AssetID:     assetID,
		Amount:      amount,
		Timestamp:   timestamp,
	})
	assert.NoError(t, err)
}

func createLockup(
	t *testing.T, sender *state.Object, beneficiary identifiers.Identifier,
	assetID identifiers.AssetID, amount *big.Int,
) {
	t.Helper()

	err := lockupAsset(sender, &common.AssetActionPayload{
		Beneficiary: beneficiary,
		AssetID:     assetID,
		Amount:      amount,
	})
	assert.NoError(t, err)
}

func setupAssetAccount(t *testing.T, operator, assetAcc *state.Object, assetID identifiers.AssetID) {
	t.Helper()

	insertTestAssetObject(
		t, assetAcc, assetID, state.NewAssetObject(big.NewInt(5000), nil),
	)

	err := assetAcc.SetState(
		assetID,
		common.NewAssetDescriptor(operator.Identifier(), common.AssetCreatePayload{}),
	)

	assert.NoError(t, err)
}

func checkGuardianRegister(t *testing.T, system *state.SystemObject, payload *common.GuardianRegisterPayload) {
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.Equal(t, payload.Amount, validator.PendingStakeAdditions)
	require.Equal(t, payload.WalletID, validator.WalletAddress)
	require.Equal(t, payload.ConsensusKey, validator.ConsensusPubKey)
	require.Equal(t, payload.KYCProof, validator.KYCProof)
}

func checkStakeGuardian(t *testing.T, system *state.SystemObject, payload *common.GuardianActionPayload) {
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.Equal(t, payload.Amount, validator.PendingStakeAdditions)
}

func checkUnstakeGuardian(t *testing.T, system *state.SystemObject, payload *common.GuardianActionPayload) {
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.Equal(t, payload.Amount, validator.PendingStakeRemovals[common.Epoch(0)])
}

func checkGuardianWithdraw(
	t *testing.T, sender *state.Object, system *state.SystemObject,
	payload *common.GuardianActionPayload, expectedStake *big.Int, expectedBalance *big.Int,
) {
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.True(t, expectedStake.Cmp(validator.InactiveStake) == 0)

	assetObject, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	require.NoError(t, err)

	require.True(t, expectedBalance.Cmp(assetObject.Balance) == 0)
}

func checkGuardianClaim(
	t *testing.T, sender *state.Object, system *state.SystemObject,
	payload *common.GuardianActionPayload, expectedBalance *big.Int,
) {
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.True(t, validator.Rewards.Cmp(big.NewInt(0)) == 0)

	assetObject, err := sender.FetchAssetObject(common.KMOITokenAssetID, true)
	require.NoError(t, err)

	require.True(t, expectedBalance.Cmp(assetObject.Balance) == 0)
}
