package consensus

//
// import (
//	"testing"
//
//	"github.com/pkg/errors"
//	"github.com/sarvalabs/go-moi-identifiers"
//	"github.com/sarvalabs/go-moi/common"
//	"github.com/sarvalabs/go-moi/common/tests"
//	"github.com/sarvalabs/go-moi/crypto/vrf"
//	"github.com/sarvalabs/go-moi/state"
//	"github.com/stretchr/testify/require"
//	blst "github.com/supranational/blst/bindings/go"
//)
//
// func TestOperatorSortition_computeICSSeed(t *testing.T) {
//	addresses := tests.GetRandomAddressList(t, 2)
//	tsHases := tests.GetHashes(t, 2)
//	seeds := tests.GetHashes(t, 3)
//	testcases := []struct {
//		name          string
//		ps            common.Participants
//		icsSeeds      map[identifiers.Identifier][32]byte
//		expected      [32]byte
//		expectedError error
//	}{
//		{
//			name: "unique participants and tesseracts without genesis accounts",
//			ps: common.Participants{
//				addresses[0]: {TesseractHash: tsHases[0], IsGenesis: false},
//				addresses[1]: {TesseractHash: tsHases[1], IsGenesis: false},
//			},
//			icsSeeds: map[identifiers.Identifier][32]byte{
//				addresses[0]: seeds[0],
//				addresses[1]: seeds[1],
//			},
//			expected:      tests.XORBytes(t, seeds[0], seeds[1]),
//			expectedError: nil,
//		},
//		{
//			name: "avoid considering genesis participant",
//			ps: common.Participants{
//				addresses[0]: {TesseractHash: tsHases[0], IsGenesis: true},
//				addresses[1]: {TesseractHash: tsHases[1], IsGenesis: false},
//			},
//			icsSeeds: map[identifiers.Identifier][32]byte{
//				addresses[0]: seeds[0],
//				addresses[1]: seeds[1],
//			},
//			expected:      seeds[1], // Only addr1's seed is considered
//			expectedError: nil,
//		},
//		{
//			name: "avoid considering duplicate tesseracts",
//			ps: common.Participants{
//				addresses[0]: {TesseractHash: tsHases[0], IsGenesis: false},
//				addresses[1]: {TesseractHash: tsHases[0], IsGenesis: false},
//			},
//			icsSeeds: map[identifiers.Identifier][32]byte{
//				addresses[0]: seeds[0],
//				addresses[1]: seeds[0],
//			},
//			expected:      seeds[0], // Only addr0's seed is considered
//			expectedError: nil,
//		},
//		{
//			name: "seed does not exist",
//			ps: common.Participants{
//				addresses[0]: {TesseractHash: tsHases[0], IsGenesis: false},
//			},
//			icsSeeds:      map[identifiers.Identifier][32]byte{},
//			expectedError: errors.New("seed not found"),
//		},
//		{
//			name: "should consider sarga seed only if sarga is one of the participants",
//			ps: common.Participants{
//				addresses[0]:        {TesseractHash: tsHases[0], IsGenesis: false},
//				common.SargaAddress: {TesseractHash: tsHases[0], IsGenesis: false},
//			},
//			icsSeeds: map[identifiers.Identifier][32]byte{
//				addresses[0]:        seeds[0],
//				common.SargaAddress: seeds[1],
//			},
//			expected:      seeds[1], // Only addr1's seed is considered
//			expectedError: nil,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			sm := &MockStateManager{icsSeed: test.icsSeeds}
//			os := &OperatorSelection{state: sm}
//
//			icsSeed, err := os.computeICSSeed(test.ps)
//			if test.expectedError != nil {
//				require.Error(t, err)
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//			require.Equal(t, test.expected[:], icsSeed[:])
//		})
//	}
//}
//
// func TestOperatorSortition_Select(t *testing.T) {
//	testcases := []struct {
//		name              string
//		ixHash            common.Hash
//		icsSeed           [32]byte
//		params            *testOperatorSortitionParams
//		testFn            func(ixHash common.Hash, os *OperatorSelection)
//		expectedErr       error
//		operatorIncentive uint64
//	}{
//		{
//			name:    "failed to retrieve total incentive state object not found",
//			ixHash:  tests.RandomHash(t),
//			icsSeed: [32]byte{1, 2, 5, 6, 9, 70},
//			params: &testOperatorSortitionParams{
//				vault: createSortitionVault(t, true),
//			},
//			expectedErr: common.ErrObjectNotFound,
//		},
//		{
//			name:    "guardian info does not exist",
//			ixHash:  tests.RandomHash(t),
//			icsSeed: [32]byte{1, 2, 5, 6, 9, 70},
//			params: &testOperatorSortitionParams{
//				vault: createSortitionVault(t, true),
//				smCallback: func(sm *MockStateManager) {
//					so := state.NewStateObject(
//						common.GuardianLogicAddr, nil, tests.NewTestTreeCache(),
//						nil, common.Account{}, state.NilMetrics(),
//						false,
//					)
//
//					err := so.CreateStorageTreeForLogic(common.GuardianLogicID)
//					require.NoError(t, err)
//
//					sm.setLatestStateObject(common.GuardianLogicAddr, so)
//				},
//			},
//			expectedErr: common.ErrKeyNotFound,
//		},
//		{
//			name:    "total incentive less than expected sortition size",
//			ixHash:  tests.RandomHash(t),
//			icsSeed: [32]byte{1, 2, 5, 6, 9, 70},
//			params: &testOperatorSortitionParams{
//				vault: createSortitionVault(t, false),
//				smCallback: func(sm *MockStateManager) {
//					updateTotalIncentives(t, sm, 10)
//				},
//			},
//			expectedErr: ErrInvalidTotalIncentives,
//		},
//		{
//			name:    "operator is eligible",
//			ixHash:  tests.RandomHash(t),
//			icsSeed: [32]byte{1, 2, 5, 6, 9, 70},
//			params: &testOperatorSortitionParams{
//				vault: createSortitionVault(t, true),
//				smCallback: func(sm *MockStateManager) {
//					updateTotalIncentives(t, sm, 50)
//				},
//			},
//			operatorIncentive: 30,
//		},
//		{
//			name:    "operator is not eligible",
//			ixHash:  tests.RandomHash(t),
//			icsSeed: [32]byte{1, 2, 5, 6, 9, 70},
//			params: &testOperatorSortitionParams{
//				vault: createSortitionVault(t, false),
//				smCallback: func(sm *MockStateManager) {
//					updateTotalIncentives(t, sm, 40)
//				},
//			},
//			expectedErr: ErrOperatorNotEligible,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			os := createTestOperatorSortition(t, test.params)
//
//			if test.testFn != nil {
//				test.testFn(test.ixHash, os)
//			}
//
//			icsOutput, _, err := os.computeVRFOutput(test.icsSeed)
//			require.NoError(t, err)
//
//			_, err = os.Select(test.operatorIncentive, icsOutput)
//
//			if test.expectedErr != nil {
//				require.Error(t, err)
//				require.ErrorContains(t, test.expectedErr, err.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// func TestOperatorSortition_VerifySelection(t *testing.T) {
//	vault := createSortitionVault(t, true)
//	sk := new(blst.SecretKey)
//	sk.Deserialize(vault.GetConsensusPrivateKey().Bytes())
//
//	icsSeed := [32]byte{1, 2, 5, 6, 9, 70}
//	vrfPrivKey := vrf.NewVRFSigner(sk)
//	icsOutput, icsProof, _ := vrfPrivKey.Evaluate(icsSeed[:])
//
//	testcases := []struct {
//		name        string
//		icsSeed     [32]byte
//		icsOutput   [32]byte
//		icsProof    []byte
//		params      *testOperatorSortitionParams
//		expectedErr error
//	}{
//		{
//			name:      "valid selection",
//			icsSeed:   [32]byte{1, 2, 5, 6, 9, 70},
//			icsOutput: icsOutput,
//			icsProof:  icsProof,
//			params: &testOperatorSortitionParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes())
//					updateTotalIncentives(t, sm, uint64(ExpectedSortionSize))
//					updateGuardianIncentive(t, sm, vault.KramaID(), 5)
//				},
//			},
//		},
//		{
//			name:      "invalid public key",
//			icsSeed:   [32]byte{1, 2, 5, 6, 9, 70},
//			icsOutput: icsOutput,
//			icsProof:  icsProof,
//			params: &testOperatorSortitionParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes()[3:])
//				},
//			},
//			expectedErr: errors.New("invalid VRF proof"),
//		},
//		{
//			name:        "public key does not exist",
//			icsSeed:     [32]byte{1, 2, 5, 6, 9, 70},
//			icsOutput:   icsOutput,
//			icsProof:    icsProof,
//			expectedErr: common.ErrPublicKeyNotFound,
//		},
//		{
//			name:      "invalid proof",
//			icsSeed:   [32]byte{1, 2, 5, 6, 9, 70},
//			icsOutput: icsOutput,
//			icsProof:  icsProof[4:],
//			params: &testOperatorSortitionParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes())
//				},
//			},
//			expectedErr: errors.New("invalid VRF proof"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			os := createTestOperatorSortition(t, test.params)
//
//			_, err := os.VerifySelection(vault.kramaID, test.icsSeed, test.icsOutput, test.icsProof)
//
//			if test.expectedErr != nil {
//				require.Error(t, err)
//				require.ErrorContains(t, test.expectedErr, err.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// type testOperatorSortitionParams struct {
//	vault      *MockVault
//	smCallback func(sm *MockStateManager)
//}
//
// func createTestOperatorSortition(t *testing.T, params *testOperatorSortitionParams) *OperatorSelection {
//	t.Helper()
//
//	var (
//		sm               = NewMockStateManager()
//		vault            = newMockVault()
//		kramaID, privKey = tests.GetKramaIDAndConsensusKey(t, 0)
//	)
//
//	vault.setKramaID(kramaID)
//	vault.setConsensusPrivateKey(privKey)
//
//	if params == nil {
//		params = &testOperatorSortitionParams{}
//	}
//
//	if params.smCallback != nil {
//		params.smCallback(sm)
//	}
//
//	if params.vault != nil {
//		vault = params.vault
//	}
//
//	operatorSortition, err := NewOperatorSelection(
//		vault.kramaID,
//		vault,
//		sm,
//	)
//	require.NoError(t, err)
//
//	return operatorSortition
//}
