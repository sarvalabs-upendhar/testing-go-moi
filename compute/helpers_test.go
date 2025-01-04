package compute

import (
	"math/big"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestStateObject(t *testing.T) *state.Object {
	t.Helper()

	address := tests.RandomAddress(t)

	return state.NewStateObject(
		address, nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)
}

func createTestAsset(t *testing.T, supply *big.Int) (*state.Object, *state.Object, identifiers.AssetID) {
	t.Helper()

	creator := state.NewStateObject(
		tests.RandomAddress(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	asset := state.NewStateObject(
		tests.RandomAddress(t), nil, tests.NewTestTreeCache(),
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

	require.NotNil(t, benefactorObject.Mandate[sender.Address()])
	require.Equal(t, 0, expectedMandateBalance.Cmp(benefactorObject.Mandate[sender.Address()].Amount))
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
	lockupAmount, err := benefactor.GetLockup(payload.AssetID, sender.Address())
	require.NoError(t, err)
	require.Equal(t, expectedAmount, lockupAmount)
}

func createTestAssetID(
	t *testing.T, assetAddr identifiers.Address,
	payload *common.AssetCreatePayload,
) identifiers.AssetID {
	t.Helper()

	return identifiers.NewAssetIDv0(
		payload.IsLogical,
		payload.IsStateFul,
		payload.Dimension,
		uint16(payload.Standard),
		assetAddr,
	)
}

func createMandate(t *testing.T, sender *state.Object, payload *common.AssetActionPayload) {
	t.Helper()

	assert.NoError(t, sender.CreateMandate(payload.AssetID, payload.Beneficiary, payload.Amount, payload.Timestamp))
}

func insertTestAssetObject(
	t *testing.T, assetID identifiers.AssetID,
	creatorAcc *state.Object, assetObject *state.AssetObject,
) {
	t.Helper()

	assert.NoError(t, creatorAcc.InsertNewAssetObject(assetID, assetObject))
}

func createTestDeedsEntry(
	t *testing.T, assetAddr identifiers.Address,
	creatorAcc *state.Object, payload *common.AssetCreatePayload,
) {
	t.Helper()

	assetID := identifiers.NewAssetIDv0(
		payload.IsLogical,
		payload.IsStateFul,
		payload.Dimension,
		uint16(payload.Standard),
		assetAddr,
	)

	assert.NoError(t, creatorAcc.CreateDeedsEntry(string(assetID)))
}

func createTestSargaStateObject(t *testing.T) *state.Object {
	t.Helper()

	sarga := state.NewStateObject(common.SargaAddress, nil, nil, nil, common.Account{
		AccType: common.SargaAccount,
	}, state.NilMetrics(), false)

	err := sarga.CreateStorageTreeForLogic(common.SargaLogicID)
	require.NoError(t, err)

	return sarga
}

func registerParticipant(t *testing.T, sarga *state.Object, address identifiers.Address) {
	t.Helper()

	assert.NoError(t, sarga.SetStorageEntry(common.SargaLogicID, address.Bytes(), address.Bytes()))
}

func createTestMandate(
	t *testing.T, sender, beneficiary *state.Object,
	assetID identifiers.AssetID, amount *big.Int, timestamp uint64,
) {
	t.Helper()

	assert.NoError(t, approveAsset(sender, &common.AssetActionPayload{
		Beneficiary: beneficiary.Address(),
		AssetID:     assetID,
		Amount:      amount,
		Timestamp:   timestamp,
	}))
}

func createLockup(
	t *testing.T, sender, beneficiary *state.Object,
	assetID identifiers.AssetID, amount *big.Int,
) {
	t.Helper()

	assert.NoError(t, lockupAsset(sender, &common.AssetActionPayload{
		Beneficiary: beneficiary.Address(),
		AssetID:     assetID,
		Amount:      amount,
	}))
}

func setupAssetAccount(t *testing.T, operator, assetAcc *state.Object, assetID identifiers.AssetID) {
	t.Helper()

	insertTestAssetObject(
		t, assetID, assetAcc, state.NewAssetObject(big.NewInt(5000), nil),
	)
	assert.NoError(
		t,
		assetAcc.SetState(assetID, common.NewAssetDescriptor(operator.Address(), common.AssetCreatePayload{})),
	)
}
