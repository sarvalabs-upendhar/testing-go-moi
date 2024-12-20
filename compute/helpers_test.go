package compute

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
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
