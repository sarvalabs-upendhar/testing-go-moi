package compute

import (
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ValidateAssetCreate(t *testing.T) {
	operator := createTestStateObject(t)
	assetAcc := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetAcc.Identifier())
	assetID2 := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	assetID3 := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	insertTestAssetObject(
		t, operator, assetID, state.NewAssetObject(nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetCreatePayload
		assetID       identifiers.AssetID
		expectedError error
	}{
		{
			name:          "asset already registered",
			sender:        operator,
			assetID:       assetID,
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name:    "invalid asset standard",
			sender:  operator,
			assetID: assetID2,
			payload: &common.AssetCreatePayload{
				Standard: common.AssetStandard(500),
			},
			expectedError: common.ErrInvalidAssetStandard,
		},
		{
			name:    "invalid asset logic",
			sender:  operator,
			assetID: assetID3,
			payload: &common.AssetCreatePayload{
				Standard: common.MASX,
				Logic:    nil, // empty logic
			},
			expectedError: common.ErrEmptyManifest,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetCreate(test.sender, test.assetID, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_CreateAsset(t *testing.T) {
	testcases := []struct {
		name          string
		descriptor    *common.AssetDescriptor
		preTestFn     func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor)
		expectedError error
	}{
		{
			name: "asset created successfully",
			descriptor: &common.AssetDescriptor{
				AssetID:           tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:            "MOI",
				CirculatingSupply: big.NewInt(5000),
			},
		},
		{
			name: "asset already exists in asset account",
			descriptor: &common.AssetDescriptor{
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:  "ETH",
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor) {
				insertTestAssetObject(t, assetAcc, assetID, state.NewAssetObject(payload))
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "asset already exists in deeds registry",
			descriptor: &common.AssetDescriptor{
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:  "ETH",
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor) {
				createTestDeedsEntry(t, creatorAcc, payload)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			assetObject := createTestStateObject(t)
			creatorObject := createTestStateObject(t)

			if test.preTestFn != nil {
				test.preTestFn(test.descriptor.AssetID, creatorObject, assetObject, test.descriptor)
			}

			assetID, err := createAsset(creatorObject, assetObject, test.descriptor)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetCreate(t, assetID, creatorObject, assetObject, test.descriptor)
		})
	}
}

func Test_ValidateAssetTransfer(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	insertTestAssetObject(
		t,
		sender,
		assetID,
		assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(50000)),
	)

	testcases := []struct {
		name          string
		benefactor    *state.Object
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		amount        *big.Int
		beneficiary   identifiers.Identifier
		preTestFn     func(target *state.Object)
		accessFn      func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int
		expectedError error
	}{
		{
			name:        "beneficiary not registered",
			benefactor:  createTestStateObject(t),
			beneficiary: tests.RandomIdentifier(t),
			amount:      big.NewInt(5000),
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:        "asset not found",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "insufficient balance",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(70000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "missing access for benefactor",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "missing access for beneficiary",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for benefactor (read lock)",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.ReadLock),
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for beneficiary (read lock)",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.ReadLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "empty access list",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "valid asset transfer operation",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(benefactor, beneficiary identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetTransfer(
				test.benefactor, target, sarga, test.assetID, common.DefaultTokenID,
				test.amount, test.accessFn(test.benefactor.Identifier(), test.beneficiary))
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_TransferAsset(t *testing.T) {
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(1000000), big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name                  string
		sender                *state.Object
		assetID               identifiers.AssetID
		amount                *big.Int
		preTestFn             func(assetID identifiers.AssetID, target *state.Object)
		expectedSenderBalance *big.Int
		expectedTargetBalance *big.Int
		expectedError         error
	}{
		{
			name:          "asset not found",
			sender:        sender1,
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(1000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:                  "initialize asset balance if asset doesn't exist",
			sender:                sender0,
			amount:                big.NewInt(3000),
			assetID:               assetID0,
			expectedSenderBalance: big.NewInt(0),
			expectedTargetBalance: big.NewInt(3000),
		},
		{
			name:                  "asset balance incremented successfully",
			sender:                sender1,
			assetID:               assetID1,
			amount:                big.NewInt(1000),
			expectedSenderBalance: big.NewInt(4000),
			expectedTargetBalance: big.NewInt(1000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := createTestStateObject(t)

			err := transferAsset(test.sender, target, test.assetID, common.DefaultTokenID, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetTransfer(
				t, test.assetID, common.DefaultTokenID, test.sender, target,
				test.expectedSenderBalance, test.expectedTargetBalance,
			)
		})
	}
}

func Test_ValidateMandateConsume(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testBeneficiaryID := tests.RandomIdentifier(t)
	testBenefactorID := tests.RandomIdentifier(t)

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		benefactor    identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		preTestFn     func(target, benefactor *state.Object)
		accessFn      func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int
		expectedError error
	}{
		{
			name:        "beneficiary not registered",
			sender:      createTestStateObject(t),
			beneficiary: testBeneficiaryID,
			benefactor:  testBenefactorID,
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					tests.RandomIdentifier(t): int(common.MutateLock),
				}
			},
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:        "asset not found",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			benefactor:  tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "missing access for beneficiary",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "missing access for benefactor",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for beneficiary (read)",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.ReadLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for benefactor (read)",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.ReadLock),
				}
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "empty access list",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{}
			},
			expectedError: common.ErrInvalidAccess,
		},

		{
			name:        "insufficient balance",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(7000),
			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				registerParticipant(t, sarga, target.Identifier())
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "mandate not found",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:        "mandate expired",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(-1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrMandateExpired,
		},
		{
			name:   "insufficient mandate funds",
			sender: sender,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(1000), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "valid asset consume operation",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor,
					sender, assetID, common.DefaultTokenID, big.NewInt(4000), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			accessFn: func(beneficiary, benefactor identifiers.Identifier) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.MutateLock),
					benefactor:  int(common.MutateLock),
				}
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)
			benefactor := state.NewStateObject(
				test.benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := validateAssetConsume(
				test.sender.Identifier(),
				target, benefactor, sarga,
				test.assetID, common.DefaultTokenID,
				test.amount, test.accessFn(test.beneficiary, test.benefactor))
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_ConsumeMandate(t *testing.T) {
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(1000000), big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name                      string
		sender                    *state.Object
		assetID                   identifiers.AssetID
		benefactor                identifiers.Identifier
		beneficiary               identifiers.Identifier
		amount                    *big.Int
		preTestFn                 func(target, benefactor *state.Object)
		expectedBenefactorBalance *big.Int
		expectedTargetBalance     *big.Int
		expectedMandateBalance    *big.Int
		expectedError             error
	}{
		{
			name:   "asset not found",
			sender: sender0,

			assetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:  big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "initialize asset balance if asset doesn't exist",
			sender: sender1,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID1,
			amount:      big.NewInt(1500),

			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID1, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender1,
					assetID1, common.DefaultTokenID,
					big.NewInt(2000),
					uint64(time.Now().Unix()))
			},
			expectedBenefactorBalance: big.NewInt(3500),
			expectedTargetBalance:     big.NewInt(1500),
			expectedMandateBalance:    big.NewInt(500),
		},
		{
			name:   "asset mandate consumed successfully",
			sender: sender0,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID0,
			amount:      big.NewInt(1500),

			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID0, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				insertTestAssetObject(
					t, target, assetID0, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(3000)),
				)
				createTestMandate(
					t,
					benefactor, sender0,
					assetID0, common.DefaultTokenID,
					big.NewInt(2000), uint64(time.Now().Unix()))
			},
			expectedBenefactorBalance: big.NewInt(3500),
			expectedTargetBalance:     big.NewInt(4500),
			expectedMandateBalance:    big.NewInt(500),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := createTestStateObject(t)
			benefactor := createTestStateObject(t)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := consumeMandate(
				test.sender.Identifier(),
				benefactor, target,
				test.assetID, common.DefaultTokenID,
				test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkMandateConsumption(
				t,
				test.assetID, common.DefaultTokenID,
				test.sender, target, benefactor, test.expectedBenefactorBalance,
				test.expectedTargetBalance, test.expectedMandateBalance,
			)
		})
	}
}

func Test_ValidateAssetMint(t *testing.T) {
	operator, assetAcc, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))
	beneficiary := createTestStateObject(t)
	randomAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	defaultAccess := map[[32]byte]int{
		assetID.AsIdentifier():   int(common.MutateLock),
		beneficiary.Identifier(): int(common.MutateLock),
	}
	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   *state.Object
		assetID       identifiers.AssetID
		amount        *big.Int
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:        "asset not found",
			sender:      createTestStateObject(t),
			beneficiary: beneficiary,
			assetID:     randomAssetID,
			access: map[[32]byte]int{
				randomAssetID:            int(common.MutateLock),
				beneficiary.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "manager mismatch",
			sender:        createTestStateObject(t),
			beneficiary:   beneficiary,
			assetID:       assetID,
			amount:        big.NewInt(7000),
			access:        defaultAccess,
			expectedError: common.ErrManagerMismatch,
		},
		{
			name:        "missing access for asset account",
			sender:      operator,
			beneficiary: beneficiary,
			assetID:     assetID,
			amount:      big.NewInt(5000),
			access: map[[32]byte]int{
				beneficiary.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "missing access for beneficiary",
			sender:      operator,
			beneficiary: beneficiary,
			assetID:     assetID,
			amount:      big.NewInt(5000),
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for asset account (read lock)",
			sender:      operator,
			beneficiary: beneficiary,
			assetID:     assetID,
			amount:      big.NewInt(5000),
			access: map[[32]byte]int{
				assetID.AsIdentifier():   int(common.ReadLock),
				beneficiary.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for beneficiary (read lock)",
			sender:      operator,
			beneficiary: beneficiary,
			assetID:     assetID,
			amount:      big.NewInt(5000),
			access: map[[32]byte]int{
				assetID.AsIdentifier():   int(common.MutateLock),
				beneficiary.Identifier(): int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "empty access list",
			sender:        operator,
			beneficiary:   beneficiary,
			assetID:       assetID,
			amount:        big.NewInt(5000),
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "valid asset mint operation",
			sender:      operator,
			beneficiary: beneficiary,
			assetID:     assetID,
			amount:      big.NewInt(5000),
			access:      defaultAccess,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetMint(
				test.sender.Identifier(),
				test.beneficiary.Identifier(),
				assetAcc,
				test.assetID,
				test.amount,
				test.access,
			)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_MintAsset(t *testing.T) {
	creator, asset, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name           string
		AssetID        identifiers.AssetID
		Amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",

			AssetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			Amount:        big.NewInt(1000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:           "asset minted successfully",
			AssetID:        assetID,
			Amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(6000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := mintAsset(creator, asset, test.AssetID, common.DefaultTokenID, test.Amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, getCirculatingSupply(t, asset, assetID))
		})
	}
}

func Test_ValidateAssetBurn(t *testing.T) {
	operator, assetAcc, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	// Pre-create objects for tests that need consistent references
	nonExistentAssetSender := createTestStateObject(t)
	nonExistentAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	operatorMismatchSender := createTestStateObject(t)
	defaultAccess := map[[32]byte]int{
		assetID.AsIdentifier(): int(common.MutateLock),
		operator.Identifier():  int(common.MutateLock),
	}

	testcases := []struct {
		name          string
		sender        *state.Object
		assetID       identifiers.AssetID
		amount        *big.Int
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:    "asset not found",
			sender:  nonExistentAssetSender,
			assetID: nonExistentAssetID,
			access: map[[32]byte]int{
				nonExistentAssetID.AsIdentifier():   int(common.MutateLock),
				nonExistentAssetSender.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:    "operator mismatch",
			sender:  operatorMismatchSender,
			assetID: assetID,
			amount:  big.NewInt(7000),
			access: map[[32]byte]int{
				assetID.AsIdentifier():              int(common.MutateLock),
				operatorMismatchSender.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrManagerMismatch,
		},
		{
			name:    "missing access for asset account",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(1000),
			access: map[[32]byte]int{
				operator.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "missing access for benefactor",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(1000),
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "invalid lock for asset account (read lock)",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(1000),
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.ReadLock),
				operator.Identifier():  int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "invalid lock for benefactor (read lock)",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(1000),
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
				operator.Identifier():  int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "empty access list",
			sender:        operator,
			assetID:       assetID,
			amount:        big.NewInt(1000),
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:   "insufficient funds",
			sender: operator,

			assetID:       assetID,
			amount:        big.NewInt(50000),
			access:        defaultAccess,
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:    "valid asset burn operation",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(5000),
			access:  defaultAccess,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetBurn(
				test.sender, assetAcc,
				test.assetID, common.DefaultTokenID,
				test.amount, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_BurnAsset(t *testing.T) {
	creator, asset, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",

			assetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:  big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:           "asset burned successfully",
			assetID:        assetID,
			amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(4000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := burnAsset(creator, asset, test.assetID, common.DefaultTokenID, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, getCirculatingSupply(t, asset, assetID))
		})
	}
}

func Test_ValidateAssetApprove(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
	)

	testcases := []struct {
		name        string
		sender      *state.Object
		beneficiary identifiers.Identifier
		amount      *big.Int
		assetID     identifiers.AssetID
		access      map[[32]byte]int

		expectedError error
	}{
		{
			name:   "asset not found",
			sender: sender,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.MutateLock),
			},

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(7000),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.MutateLock),
			},

			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:          "empty access list",
			sender:        sender,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       assetID,
			amount:        big.NewInt(1000),
			access:        map[[32]byte]int{}, // Empty access map - sender has no access
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid lock for sender (read lock)",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(1000),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "no lock for sender",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(1000),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.NoLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid amount (zero)",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(0),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAmount,
		},
		{
			name:        "invalid amount (negative)",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(-100),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.MutateLock),
			},
			expectedError: common.ErrInvalidAmount,
		},
		{
			name:        "valid asset approve operation",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			access: map[[32]byte]int{
				sender.Identifier(): int(common.MutateLock),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetApprove(test.sender, test.assetID, common.DefaultTokenID, test.amount, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetApprove(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		Timestamp     uint64
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: creator,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(5000),
			Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset approved successfully",
			sender:      creator,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(5000),
			Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := approveAsset(
				test.sender,
				test.assetID, common.DefaultTokenID,
				test.beneficiary,
				test.amount, 0)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetApprove(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary, test.amount)
		})
	}
}

func Test_ValidateAssetRevoke(t *testing.T) {
	sender := createTestStateObject(t)
	defaultAccess := map[[32]byte]int{
		sender.Identifier(): int(common.MutateLock),
	}
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)
	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		access        map[[32]byte]int
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:        "mandate not found",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			access:        defaultAccess,
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:        "invalid access for sender",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			access: map[[32]byte]int{
				sender.Identifier(): int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "empty access list",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "valid asset revoke operation",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				createTestMandate(t,
					sender, target,
					assetID, common.DefaultTokenID,
					big.NewInt(2000), uint64(time.Now().Unix()),
				)
			},
			access: defaultAccess,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetRevoke(test.sender, test.beneficiary, test.assetID, common.DefaultTokenID, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetRevoke(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		Timestamp     uint64
		preTestFn     func(sender *state.Object, assetID identifiers.AssetID, beneficiary identifiers.Identifier)
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        creator,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(5000),
			Timestamp:     uint64(time.Now().Add(1 * time.Hour).Unix()),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "asset revoked successfully",
			sender: creator,
			preTestFn: func(sender *state.Object, assetID identifiers.AssetID, beneficiary identifiers.Identifier) {
				createMandate(
					t,
					sender, beneficiary,
					assetID, common.DefaultTokenID,
					big.NewInt(500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},

			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.sender, test.assetID, test.beneficiary)
			}

			err := revokeAsset(test.sender, test.beneficiary, test.assetID, common.DefaultTokenID)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetRevoke(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary)
		})
	}
}

func Test_ValidateAssetLockup(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	defaultAccess := map[[32]byte]int{
		sender.Identifier(): int(common.MutateLock),
	}

	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(10000)),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		access        map[[32]byte]int
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        sender,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(1000),
			access:        defaultAccess,
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "insufficient balance",
			sender:        sender,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       assetID,
			amount:        big.NewInt(12000),
			access:        defaultAccess,
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:          "nil beneficiary",
			sender:        sender,
			beneficiary:   identifiers.Nil,
			assetID:       assetID,
			access:        defaultAccess,
			amount:        big.NewInt(4000),
			expectedError: common.ErrInvalidBeneficiary,
		},
		{
			name:        "valid asset lockup operation",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			access:      defaultAccess,
			amount:      big.NewInt(4000),
		},
		{
			name:        "invalid access for sender",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			access: map[[32]byte]int{
				sender.Identifier(): int(common.ReadLock),
			},
			amount:        big.NewInt(4000),
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "invalid access list",
			sender:        sender,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       assetID,
			access:        map[[32]byte]int{},
			amount:        big.NewInt(4000),
			expectedError: common.ErrInvalidAccess,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetLockup(
				test.sender,
				test.beneficiary,
				test.assetID,
				common.DefaultTokenID,
				test.amount,
				test.access,
			)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetLockup(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        creator,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(5000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset locked up successfully",
			sender:      creator,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(5000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := lockupAsset(
				test.sender,
				test.beneficiary,
				test.assetID, common.DefaultTokenID,
				test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetLockup(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary, test.amount)
		})
	}
}

func Test_ValidateAssetRelease(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name          string
		sender        *state.Object
		benefactor    identifiers.Identifier
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		Amount        *big.Int
		accessFn      func(benefactor, beneficiary [32]byte) map[[32]byte]int
		preTestFn     func(target, benefactor *state.Object)
		expectedError error
	}{
		{
			name:        "lockup not found",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			accessFn: func(benefactor, beneficiary [32]byte) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrLockupNotFound,
		},
		{
			name:        "valid asset release operation",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(1000),
			accessFn: func(benefactor, beneficiary [32]byte) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.MutateLock),
					beneficiary: int(common.MutateLock),
				}
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2000))
			},
		},
		{
			name:        "invalid access for benefactor",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(1000),
			accessFn: func(benefactor, beneficiary [32]byte) map[[32]byte]int {
				return map[[32]byte]int{
					benefactor:  int(common.ReadLock),
					beneficiary: int(common.MutateLock),
				}
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2000))
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid access for beneficiary",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(1000),
			accessFn: func(benefactor, beneficiary [32]byte) map[[32]byte]int {
				return map[[32]byte]int{
					beneficiary: int(common.ReadLock),
					benefactor:  int(common.MutateLock),
				}
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2000))
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:        "invalid access list",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(1000),
			accessFn: func(benefactor, beneficiary [32]byte) map[[32]byte]int {
				// returning empty access map
				return map[[32]byte]int{}
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2000))
			},
			expectedError: common.ErrInvalidAccess,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			benefactor := state.NewStateObject(
				test.benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := validateAssetRelease(
				test.sender.Identifier(),
				benefactor,
				test.beneficiary,
				test.assetID, common.DefaultTokenID,
				test.Amount, test.accessFn(test.benefactor, test.beneficiary))
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetRelease(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name           string
		sender         *state.Object
		benefactor     identifiers.Identifier
		beneficiary    identifiers.Identifier
		assetID        identifiers.AssetID
		amount         *big.Int
		preTestFn      func(target, benefactor *state.Object)
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:        "asset not found",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset released successfully",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(1000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2500))
			},
			expectedAmount: big.NewInt(1500),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			benefactor := state.NewStateObject(
				test.benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := releaseAsset(sender.Identifier(), benefactor, target, test.assetID, common.DefaultTokenID, test.amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetRelease(
				t,
				sender, target,
				benefactor,
				test.amount, test.assetID,
				common.DefaultTokenID, test.expectedAmount,
			)
		})
	}
}

//nolint:dupl
func Test_ValidateSetStaticMetaData(t *testing.T) {
	managerID := tests.RandomIdentifier(t)
	nonManagerID := tests.RandomIdentifier(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	nonExistentAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	assetStateObj := createTestStateObject(t)

	// Insert asset with manager
	insertTestAssetObject(t, assetStateObj, assetID, assetObjectWithManager(t, assetID, managerID))

	testcases := []struct {
		name          string
		assetStateObj *state.Object
		assetID       identifiers.AssetID
		participantID identifiers.Identifier
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:          "valid metadata update",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
		},
		{
			name:          "missing access for asset account",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "invalid lock for asset account (read lock)",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "no lock for asset account",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.NoLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "asset not found",
			assetStateObj: assetStateObj,
			assetID:       nonExistentAssetID,
			participantID: managerID,
			access: map[[32]byte]int{
				nonExistentAssetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "manager mismatch",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: nonManagerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrManagerMismatch,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateSetStaticMetaData(
				test.assetStateObj,
				test.assetID,
				test.participantID,
				test.access,
			)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_ValidateSetDynamicMetaData(t *testing.T) {
	managerID := tests.RandomIdentifier(t)
	nonManagerID := tests.RandomIdentifier(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	nonExistentAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	assetStateObj := createTestStateObject(t)

	// Insert asset with manager
	insertTestAssetObject(t, assetStateObj, assetID, assetObjectWithManager(t, assetID, managerID))

	testcases := []struct {
		name          string
		assetStateObj *state.Object
		assetID       identifiers.AssetID
		participantID identifiers.Identifier
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:          "valid metadata update",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
		},
		{
			name:          "missing access for asset account",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "invalid lock for asset account (read lock)",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.ReadLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "no lock for asset account",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: managerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.NoLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "asset not found",
			assetStateObj: assetStateObj,
			assetID:       nonExistentAssetID,
			participantID: managerID,
			access: map[[32]byte]int{
				nonExistentAssetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "manager mismatch",
			assetStateObj: assetStateObj,
			assetID:       assetID,
			participantID: nonManagerID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
			expectedError: common.ErrManagerMismatch,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateSetDynamicMetaData(
				test.assetStateObj,
				test.assetID,
				test.participantID,
				test.access,
			)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_GetStaticMetaData(t *testing.T) {
	assetStateObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetStateObj.Identifier())
	nonExistentAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	// Set up asset state object with the asset
	insertTestAssetObject(t, assetStateObj, assetID, state.NewAssetObject(&common.AssetDescriptor{
		AssetID: assetID,
		StaticMetaData: map[string][]byte{
			"test-key": []byte("test-value"),
		},
	}))

	_, err := assetStateObj.Commit()
	require.NoError(t, err)

	// Create object map and transition
	objects := state.ObjectMap{
		assetID.AsIdentifier(): assetStateObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:    "valid read with read lock",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.ReadLock),
			},
		},
		{
			name:    "invalid access for asset account (no lock)",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.NoLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "valid read with mutate lock",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
		},
		{
			name:          "missing access for asset account",
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "asset not found",
			assetID: nonExistentAssetID,
			access: map[[32]byte]int{
				nonExistentAssetID.AsIdentifier(): int(common.ReadLock),
			},
			expectedError: common.ErrObjectNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := engine.GetStaticMetaData(test.assetID, "test-key", test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_GetDynamicMetaData(t *testing.T) {
	assetStateObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetStateObj.Identifier())
	nonExistentAssetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	// Set up asset state object with the asset
	insertTestAssetObject(t, assetStateObj, assetID, state.NewAssetObject(&common.AssetDescriptor{
		AssetID: assetID,
		DynamicMetaData: map[string][]byte{
			"test-key": []byte("test-value"),
		},
	}))

	_, err := assetStateObj.Commit()
	require.NoError(t, err)

	// Create object map and transition
	objects := state.ObjectMap{
		assetID.AsIdentifier(): assetStateObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		access        map[[32]byte]int
		expectedError error
	}{
		{
			name:    "valid read with read lock",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.ReadLock),
			},
		},
		{
			name:    "invalid lock for asset account (no lock)",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.NoLock),
			},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "valid read with mutate lock",
			assetID: assetID,
			access: map[[32]byte]int{
				assetID.AsIdentifier(): int(common.MutateLock),
			},
		},
		{
			name:          "missing access for asset account",
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:    "asset not found",
			assetID: nonExistentAssetID,
			access: map[[32]byte]int{
				nonExistentAssetID.AsIdentifier(): int(common.ReadLock),
			},
			expectedError: common.ErrObjectNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err = engine.GetDynamicMetaData(test.assetID, "test-key", test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

//nolint:dupl
func Test_MaxSupply(t *testing.T) {
	// Set up asset state object with the asset
	assetStateObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetStateObj.Identifier())

	expectedMaxSupply := big.NewInt(1000000)
	assetDescriptor := &common.AssetDescriptor{
		AssetID:   assetID,
		Manager:   tests.RandomIdentifier(t),
		MaxSupply: expectedMaxSupply,
	}
	insertTestAssetObject(t, assetStateObj, assetID, state.NewAssetObject(assetDescriptor))
	_, err := assetStateObj.Commit()
	require.NoError(t, err)

	// Set up non-existent asset state object
	nonExistentStateObj := createTestStateObject(t)
	nonExistentAssetID := tests.GetRandomAssetID(t, nonExistentStateObj.Identifier())

	// Create object map and transition
	objects := state.ObjectMap{
		assetID.AsIdentifier():            assetStateObj,
		nonExistentAssetID.AsIdentifier(): nonExistentStateObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		access         map[[32]byte]int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name:           "valid read with read lock",
			assetID:        assetID,
			access:         map[[32]byte]int{assetID.AsIdentifier(): int(common.ReadLock)},
			expectedSupply: expectedMaxSupply,
		},
		{
			name:          "invalid access for asset account (no lock)",
			assetID:       assetID,
			access:        map[[32]byte]int{assetID.AsIdentifier(): int(common.NoLock)},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:           "valid read with mutate lock",
			assetID:        assetID,
			access:         map[[32]byte]int{assetID.AsIdentifier(): int(common.MutateLock)},
			expectedSupply: expectedMaxSupply,
		},
		{
			name:          "missing access for asset account",
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "asset not found",
			assetID:       nonExistentAssetID,
			access:        map[[32]byte]int{nonExistentAssetID.AsIdentifier(): int(common.ReadLock)},
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := engine.MaxSupply(test.assetID, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, supply)
		})
	}
}

//nolint:dupl
func Test_CirculatingSupply(t *testing.T) {
	// Set up asset state object with the asset
	assetStateObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetStateObj.Identifier())

	expectedCirculatingSupply := big.NewInt(500000)
	assetDescriptor := &common.AssetDescriptor{
		AssetID:           assetID,
		Manager:           tests.RandomIdentifier(t),
		MaxSupply:         big.NewInt(1000000),
		CirculatingSupply: expectedCirculatingSupply,
	}
	insertTestAssetObject(t, assetStateObj, assetID, state.NewAssetObject(assetDescriptor))
	_, err := assetStateObj.Commit()
	require.NoError(t, err)

	// Set up non-existent asset state object
	nonExistentStateObj := createTestStateObject(t)
	nonExistentAssetID := tests.GetRandomAssetID(t, nonExistentStateObj.Identifier())

	// Create object map and transition
	objects := state.ObjectMap{
		assetID.AsIdentifier():            assetStateObj,
		nonExistentAssetID.AsIdentifier(): nonExistentStateObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		access         map[[32]byte]int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name:           "valid read with read lock",
			assetID:        assetID,
			access:         map[[32]byte]int{assetID.AsIdentifier(): int(common.ReadLock)},
			expectedSupply: expectedCirculatingSupply,
		},
		{
			name:          "invalid access for asset account (no lock)",
			assetID:       assetID,
			access:        map[[32]byte]int{assetID.AsIdentifier(): int(common.NoLock)},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:           "valid read with mutate lock",
			assetID:        assetID,
			access:         map[[32]byte]int{assetID.AsIdentifier(): int(common.MutateLock)},
			expectedSupply: expectedCirculatingSupply,
		},
		{
			name:          "missing access for asset account",
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "asset not found",
			assetID:       nonExistentAssetID,
			access:        map[[32]byte]int{nonExistentAssetID.AsIdentifier(): int(common.ReadLock)},
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := engine.CirculatingSupply(test.assetID, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, supply)
		})
	}
}

func Test_BalanceOf(t *testing.T) {
	// Set up participant state object with asset balance
	participantObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	expectedBalance := big.NewInt(75000)
	insertTestAssetObject(t, participantObj, assetID, assetObjectWithToken(t, common.DefaultTokenID, expectedBalance))
	_, err := participantObj.Commit()
	require.NoError(t, err)

	// Set up non-existent participant
	nonExistentParticipant := tests.RandomIdentifier(t)

	// Create object map and transition
	objects := state.ObjectMap{
		participantObj.Identifier(): participantObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name            string
		address         identifiers.Identifier
		assetID         identifiers.AssetID
		access          map[[32]byte]int
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:            "valid read with read lock",
			address:         participantObj.Identifier(),
			assetID:         assetID,
			access:          map[[32]byte]int{participantObj.Identifier(): int(common.ReadLock)},
			expectedBalance: expectedBalance,
		},
		{
			name:          "invalid access for participant account (no lock)",
			address:       participantObj.Identifier(),
			assetID:       assetID,
			access:        map[[32]byte]int{participantObj.Identifier(): int(common.NoLock)},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:            "valid read with mutate lock",
			address:         participantObj.Identifier(),
			assetID:         assetID,
			access:          map[[32]byte]int{participantObj.Identifier(): int(common.MutateLock)},
			expectedBalance: expectedBalance,
		},
		{
			name:          "missing access for participant account",
			address:       participantObj.Identifier(),
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "participant not found",
			address:       nonExistentParticipant,
			assetID:       assetID,
			access:        map[[32]byte]int{nonExistentParticipant: int(common.ReadLock)},
			expectedError: common.ErrObjectNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := engine.BalanceOf(test.address, test.assetID, common.DefaultTokenID, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedBalance, balance)
		})
	}
}

func Test_Manager(t *testing.T) {
	// Set up asset state object with the asset
	assetStateObj := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetStateObj.Identifier())

	expectedManager := tests.RandomIdentifier(t)
	assetDescriptor := &common.AssetDescriptor{
		AssetID: assetID,
		Manager: expectedManager,
	}
	insertTestAssetObject(t, assetStateObj, assetID, state.NewAssetObject(assetDescriptor))
	_, err := assetStateObj.Commit()
	require.NoError(t, err)

	// Set up non-existent asset state object
	nonExistentStateObj := createTestStateObject(t)
	nonExistentAssetID := tests.GetRandomAssetID(t, nonExistentStateObj.Identifier())

	// Create object map and transition
	objects := state.ObjectMap{
		assetID.AsIdentifier():            assetStateObj,
		nonExistentAssetID.AsIdentifier(): nonExistentStateObj,
	}
	transition := state.NewTransition(nil, objects, nil)
	engine := NewAssetEngine(transition)

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		access          map[[32]byte]int
		expectedManager identifiers.Identifier
		expectedError   error
	}{
		{
			name:            "valid read with read lock",
			assetID:         assetID,
			access:          map[[32]byte]int{assetID.AsIdentifier(): int(common.ReadLock)},
			expectedManager: expectedManager,
		},
		{
			name:          "invalid lock for asset account (no lock)",
			assetID:       assetID,
			access:        map[[32]byte]int{assetID.AsIdentifier(): int(common.NoLock)},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:            "valid read with mutate lock",
			assetID:         assetID,
			access:          map[[32]byte]int{assetID.AsIdentifier(): int(common.MutateLock)},
			expectedManager: expectedManager,
		},
		{
			name:          "missing access for asset account",
			assetID:       assetID,
			access:        map[[32]byte]int{},
			expectedError: common.ErrInvalidAccess,
		},
		{
			name:          "asset not found",
			assetID:       nonExistentAssetID,
			access:        map[[32]byte]int{nonExistentAssetID.AsIdentifier(): int(common.ReadLock)},
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			manager, err := engine.Manager(test.assetID, test.access)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedManager, manager)
		})
	}
}
