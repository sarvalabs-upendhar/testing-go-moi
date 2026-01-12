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

func getCirculatingSupply(t *testing.T, assetAcc *state.Object, assetID identifiers.AssetID) *big.Int {
	t.Helper()

	assetObject, err := assetAcc.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	return assetObject.Properties.CirculatingSupply
}

func assetObjectWithToken(t *testing.T, tokenID common.TokenID, balance *big.Int) *state.AssetObject {
	t.Helper()

	ao := &state.AssetObject{
		Balance: tests.TokenWithoutExpiry(t, tokenID, balance),
	}

	return ao
}

func assetObjectWithManager(
	t *testing.T,
	assetID identifiers.AssetID, manager identifiers.Identifier,
) *state.AssetObject {
	t.Helper()

	return state.NewAssetObject(&common.AssetDescriptor{
		AssetID: assetID,
		Manager: manager,
	})
}

func createTestStateObject(t *testing.T) *state.Object {
	t.Helper()

	id := tests.RandomIdentifier(t)

	return state.NewStateObject(
		id, nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)
}

func createTestAsset(
	t *testing.T, maxSupply, circulatingSupply *big.Int,
) (*state.Object, *state.Object, identifiers.AssetID) {
	t.Helper()

	creator := state.NewStateObject(
		tests.RandomIdentifier(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	asset := state.NewStateObject(
		tests.RandomIdentifier(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	assetID := identifiers.RandomAssetIDv0()

	assetID, err := createAsset(creator, asset, &common.AssetDescriptor{
		AssetID:           assetID,
		Symbol:            "MOI",
		MaxSupply:         maxSupply,
		CirculatingSupply: big.NewInt(0),
		Creator:           creator.Identifier(),
		Manager:           creator.Identifier(),
	})
	require.NoError(t, err)

	err = mintAsset(creator, asset, assetID, common.DefaultTokenID, circulatingSupply)
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
	payload *common.AssetDescriptor,
) {
	t.Helper()

	assetObject, err := assetAcc.FetchAssetObject(assetID, true)

	require.NoError(t, err)
	deeds, err := creatorAcc.Deeds()
	require.NoError(t, err)

	_, ok := deeds.Entries[payload.AssetID.AsIdentifier()]
	require.True(t, ok, "Deed entry not found")

	require.Equal(t, assetObject.Properties.AssetID, payload.AssetID)
	require.Equal(t, assetObject.Properties.Symbol, payload.Symbol)
}

func checkAssetTransfer(
	t *testing.T, assetID identifiers.AssetID, tokenID common.TokenID,
	sender, target *state.Object,
	expectedSenderBalance, expectedTargetBalance *big.Int,
) {
	t.Helper()

	senderObject, err := sender.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	targetObject, err := target.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	require.Equal(t, 0, expectedSenderBalance.Cmp(senderObject.GetBalance(tokenID)))
	require.Equal(t, 0, expectedTargetBalance.Cmp(targetObject.Balance[tokenID]))
}

func checkMandateConsumption(
	t *testing.T, assetID identifiers.AssetID, tokenID common.TokenID, sender, target, benefactor *state.Object,
	expectedBenefactorBalance, expectedTargetBalance, expectedMandateBalance *big.Int,
) {
	t.Helper()

	targetObject, err := target.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	benefactorObject, err := benefactor.FetchAssetObject(assetID, true)
	require.NoError(t, err)

	require.Equal(t, 0, expectedBenefactorBalance.Cmp(benefactorObject.Balance[tokenID]))
	require.Equal(t, 0, expectedTargetBalance.Cmp(targetObject.Balance[tokenID]))

	require.NotNil(t, benefactorObject.Mandate[sender.Identifier()])
	require.Equal(t, 0, expectedMandateBalance.Cmp(benefactorObject.Mandate[sender.Identifier()][tokenID].Amount))
}

func checkAssetApprove(
	t *testing.T,
	sender *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID,
	beneficiary identifiers.Identifier, amount *big.Int,
) {
	t.Helper()

	mandate, err := sender.GetMandate(assetID, tokenID, beneficiary)
	require.NoError(t, err)
	require.Equal(t, amount, mandate.Amount)
}

func checkAssetLockup(t *testing.T,
	sender *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID,
	beneficiary identifiers.Identifier, amount *big.Int,
) {
	t.Helper()

	lockup, err := sender.GetLockup(assetID, tokenID, beneficiary)
	require.NoError(t, err)
	require.Equal(t, amount, lockup.Amount)
}

func checkAssetRevoke(t *testing.T,
	sender *state.Object,
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	beneficiary identifiers.Identifier,
) {
	t.Helper()

	_, err := sender.GetMandate(assetID, tokenID, beneficiary)
	require.Error(t, err)
}

func checkAssetRelease(
	t *testing.T, sender, beneficiary, benefactor *state.Object,
	amount *big.Int, assetID identifiers.AssetID, tokenID common.TokenID, expectedAmount *big.Int,
) {
	t.Helper()

	// Check the beneficiary account
	ao, err := beneficiary.FetchAssetObject(assetID, true)
	require.NoError(t, err)
	require.Equal(t, amount, ao.Balance[common.DefaultTokenID])

	// Check whether the lockup amount got deducted
	lockup, err := benefactor.GetLockup(assetID, tokenID, sender.Identifier())
	require.NoError(t, err)
	require.Equal(t, expectedAmount, lockup.Amount)
}

func createMandate(
	t *testing.T,
	sender *state.Object,
	beneficiary identifiers.Identifier,
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	amount *big.Int,
	timestamp uint64,
) {
	t.Helper()

	err := sender.CreateMandate(assetID, tokenID, beneficiary, amount, timestamp)
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

func insertGuardianEntry( //nolint
	t *testing.T, system *state.SystemObject, validator *common.Validator,
) {
	t.Helper()

	err := system.SetValidators([]*common.Validator{validator})
	require.NoError(t, err)
}

func createTestDeedsEntry(
	t *testing.T,
	creatorAcc *state.Object, payload *common.AssetDescriptor,
) {
	t.Helper()

	err := creatorAcc.CreateDeedsEntry(payload.AssetID.AsIdentifier())
	assert.NoError(t, err)
}

func createTestSargaStateObject(t *testing.T) *state.Object {
	t.Helper()

	sarga := state.NewStateObject(common.SargaAccountID, nil, nil, nil, common.Account{
		AccType: common.SystemAccount,
	}, state.NilMetrics(), false)

	err := sarga.CreateStorageTreeForLogic(common.SargaLogicID.AsIdentifier())
	require.NoError(t, err)

	return sarga
}

func registerParticipant(t *testing.T, sarga *state.Object, id identifiers.Identifier) {
	t.Helper()

	err := sarga.SetStorageEntry(common.SargaLogicID.AsIdentifier(), id.Bytes(), id.Bytes())
	assert.NoError(t, err)
}

func createTestMandate(
	t *testing.T, benefactor, beneficiary *state.Object,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int, timestamp uint64,
) {
	t.Helper()

	err := approveAsset(benefactor, assetID, tokenID, beneficiary.Identifier(), amount, timestamp)
	assert.NoError(t, err)
}

func createLockup(
	t *testing.T, sender *state.Object, beneficiary identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID, amount *big.Int,
) {
	t.Helper()

	err := lockupAsset(sender, beneficiary, assetID, tokenID, amount)

	assert.NoError(t, err)
}

func setupAssetAccount(t *testing.T, operator, assetAcc *state.Object, assetID identifiers.AssetID) { //nolint
	t.Helper()

	insertTestAssetObject(
		t, assetAcc, assetID, state.NewAssetObject(common.NewAssetDescriptor(
			assetID,
			"MOI",
			0,
			0,
			operator.Identifier(), operator.Identifier(),
			big.NewInt(5000), nil, nil, false, identifiers.Nil)))
}

func checkGuardianRegister(t *testing.T, system *state.SystemObject, payload *common.GuardianRegisterPayload) { //nolint
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

func checkStakeGuardian(t *testing.T, system *state.SystemObject, payload *common.GuardianActionPayload) { //nolint
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.Equal(t, payload.Amount, validator.PendingStakeAdditions)
}

func checkUnstakeGuardian(t *testing.T, system *state.SystemObject, payload *common.GuardianActionPayload) { //nolint
	t.Helper()

	validator, err := system.ValidatorByKramaID(payload.KramaID)
	require.NoError(t, err)

	require.NotNil(t, validator)
	require.Equal(t, payload.KramaID, validator.KramaID)
	require.Equal(t, payload.Amount, validator.PendingStakeRemovals[common.Epoch(0)])
}

func checkGuardianWithdraw( //nolint
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

	require.True(t, expectedBalance.Cmp(assetObject.Balance[common.DefaultTokenID]) == 0)
}

func checkGuardianClaim( //nolint
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

	require.True(t, expectedBalance.Cmp(assetObject.Balance[common.DefaultTokenID]) == 0)
}
