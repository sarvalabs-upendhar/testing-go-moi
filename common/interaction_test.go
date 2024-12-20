package common_test

import (
	"math/big"
	"reflect"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/stretchr/testify/require"
)

func TestNewInteraction(t *testing.T) {
	assetCreatePayload := common.AssetCreatePayload{Symbol: "MOI", Supply: big.NewInt(500), Standard: common.MAS0}
	rawAssetCreatePayload, _ := assetCreatePayload.Bytes()

	assetSupplyPayload := common.AssetSupplyPayload{
		AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
		Amount:  big.NewInt(500),
	}
	rawAssetSupplyPayload, _ := assetSupplyPayload.Bytes()

	assetActionPayload := common.AssetActionPayload{
		Beneficiary: tests.RandomAddress(t),
		AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
		Amount:      big.NewInt(500),
	}
	rawAssetActionPayload, _ := assetActionPayload.Bytes()

	logicPayload := common.LogicPayload{
		Manifest: []byte{2, 1, 5, 9},
		Callsite: "hello",
		Calldata: []byte{0, 7, 8, 1},
		Interfaces: map[string]identifiers.LogicID{
			"hello": tests.GetLogicID(t, tests.RandomAddress(t)),
		},
	}
	rawLogicPayload, _ := logicPayload.Bytes()

	testcases := []struct {
		name        string
		ixData      common.IxData
		sign        []byte
		expectedIX  *common.Interaction
		expectedErr error
	}{
		{
			name: "asset transfer ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: rawAssetActionPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					Address: assetActionPayload.Beneficiary,
				})
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "missing beneficiary in participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: rawAssetActionPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingBeneficiary,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "asset create ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: rawAssetCreatePayload,
					},
				}
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "sender not found in participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: rawAssetCreatePayload,
					},
				}
				ixData.Participants = []common.IxParticipant{}
			}),
			expectedErr: common.ErrMissingSender,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "payer not found in participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: rawAssetCreatePayload,
					},
				}
				ixData.Participants = ixData.Participants[1:2]
			}),
			expectedErr: common.ErrMissingPayer,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "asset mint ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetMint,
						Payload: rawAssetSupplyPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					Address: assetSupplyPayload.AssetID.Address(),
				})
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "missing asset account in participants in asset mint ixn",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetMint,
						Payload: rawAssetSupplyPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingAssetAccount,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "asset burn ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetBurn,
						Payload: rawAssetSupplyPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					Address: assetSupplyPayload.AssetID.Address(),
				})
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "missing asset account in participants in asset burn ixn",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetBurn,
						Payload: rawAssetSupplyPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingAssetAccount,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "deploy logic ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxLogicDeploy,
						Payload: rawLogicPayload,
					},
				}
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "invoke logic ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxLogicInvoke,
						Payload: rawLogicPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, []common.IxParticipant{
					{
						Address: logicPayload.Logic.Address(),
					},
					{
						Address: logicPayload.Interfaces["hello"].Address(),
					},
				}...)
			}),
			sign: []byte{1, 2, 3},
		},
		{
			name: "missing foreign logic account in participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxLogicInvoke,
						Payload: rawLogicPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, []common.IxParticipant{
					{
						Address: logicPayload.Logic.Address(),
					},
				}...)
			}),
			expectedErr: common.ErrMissingForeignLogicAccount,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "missing logic account from participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxLogicInvoke,
						Payload: rawLogicPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingLogicAccount,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "missing foreign logic account from participants",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxLogicInvoke,
						Payload: rawLogicPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingLogicAccount,
			sign:        []byte{1, 2, 3},
		},
		{
			name: "invalid ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxInvalid,
						Payload: rawLogicPayload,
					},
				}
			}),
			sign:        []byte{1, 2, 3},
			expectedErr: common.ErrInvalidInteractionType,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, err := common.NewInteraction(test.ixData, test.sign)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)

			// check if ix data copied properly
			require.Equal(t, test.ixData.Nonce, ix.Nonce())
			require.Equal(t, test.ixData.Sender, ix.Sender())
			require.Equal(t, test.ixData.Payer, ix.Payer())
			require.Equal(t, test.ixData.FuelPrice, ix.FuelPrice())
			require.Equal(t, test.ixData.FuelLimit, ix.FuelLimit())
			require.Equal(t, test.ixData.Funds, ix.Funds())
			require.Equal(t, test.ixData.Participants, ix.IxParticipants())
			require.Equal(t, test.ixData.Perception, ix.Perception())
			require.Equal(t, test.ixData.Preferences, ix.Preferences())

			require.Equal(t, test.sign, ix.Signature())

			data, err := polo.Polorize(test.ixData)
			require.NoError(t, err)

			require.Equal(t, common.GetHash(data), ix.Hash())

			size, err := ix.Size()
			require.NoError(t, err)

			require.Equal(t, uint64(len(data)+len(ix.Signature())), size)

			// check for payload
			checkIxOperations(t, ix, assetCreatePayload, assetActionPayload, assetSupplyPayload, logicPayload)

			if test.ixData.IxOps[0].Type == common.IxAssetCreate ||
				test.ixData.IxOps[0].Type == common.IxLogicDeploy {
				addr := common.NewAccountAddress(ix.Nonce(), ix.Sender())
				info := ix.Participants()
				_, ok := info[addr]
				require.True(t, ok)

				_, ok = info[common.SargaAddress]
				require.True(t, ok)
			}
		})
	}
}

func TestUpdateLeaderCandidateAddr(t *testing.T) {
	regularAccount, err := identifiers.NewAddressFromHex(
		"0x0000000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9384")
	require.NoError(t, err)

	nonRegularAccount1, err := identifiers.NewAddressFromHex(
		"0xeeee000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9384")
	require.NoError(t, err)

	nonRegularAccount2, err := identifiers.NewAddressFromHex(
		"0xee00000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9384")
	require.NoError(t, err)

	testcases := []struct {
		name              string
		createIxParams    tests.CreateIxParams
		expectedLeaderAcc identifiers.Address
	}{
		{
			name: "sarga account is selected as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.IxOps = append(ix.IxOps, common.IxOpRaw{
						Type:    common.IxLogicInvoke,
						Payload: tests.CreateRawLogicPayload(t, nonRegularAccount1),
					})

					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							Address:  common.SargaAddress,
							LockType: common.MutateLock,
						},
						{
							Address:  regularAccount,
							LockType: common.MutateLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: common.SargaAddress,
		},
		{
			name: "first account from sorted regular accounts is chosen as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							Address:  regularAccount,
							LockType: common.MutateLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: regularAccount,
		},
		{
			name: "first account from sorted non-regular accounts is chosen as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.IxOps = append(ix.IxOps, common.IxOpRaw{
						Type:    common.IxLogicInvoke,
						Payload: tests.CreateRawLogicPayload(t, nonRegularAccount1),
					})
					ix.IxOps = append(ix.IxOps, common.IxOpRaw{
						Type:    common.IxLogicInvoke,
						Payload: tests.CreateRawLogicPayload(t, nonRegularAccount2),
					})
					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							Address:  regularAccount,
							LockType: common.MutateLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: nonRegularAccount2,
		},
		{
			name: "non-regular account with read lock is not chosen as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.IxOps = append(ix.IxOps, common.IxOpRaw{
						Type:    common.IxLogicInvoke,
						Payload: tests.CreateRawLogicPayload(t, nonRegularAccount1),
					})
					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							Address:  regularAccount,
							LockType: common.MutateLock,
						},
						{
							Address:  nonRegularAccount1,
							LockType: common.ReadLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: regularAccount,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ix := tests.CreateIX(t, &testcase.createIxParams)

			require.Equal(t, testcase.expectedLeaderAcc, ix.LeaderCandidateAcc())
		})
	}
}

func TestCopyIxFund(t *testing.T) {
	testcases := []struct {
		name string
		data common.IxFund
	}{
		{
			name: "copy ix fund with all populated fields",
			data: common.IxFund{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(5000),
			},
		},
		{
			name: "copy ix fund with empty fields",
			data: common.IxFund{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedData := test.data

			dataCopy := test.data.Copy()

			require.Equal(t, expectedData, dataCopy)

			if expectedData.Amount != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Amount).Pointer(),
					reflect.ValueOf(dataCopy.Amount).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxOperation(t *testing.T) {
	testcases := []struct {
		name string
		data common.IxOpRaw
	}{
		{
			name: "copy ix op with all populated fields",
			data: common.IxOpRaw{
				Type:    common.IxAssetCreate,
				Payload: []byte{1, 2, 3, 4},
			},
		},
		{
			name: "copy ix op with empty fields",
			data: common.IxOpRaw{
				Type: common.IxAssetCreate,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedData := test.data

			dataCopy := test.data.Copy()

			require.Equal(t, expectedData, dataCopy)

			if expectedData.Payload != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Payload).Pointer(),
					reflect.ValueOf(dataCopy.Payload).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxConsensusPreference(t *testing.T) {
	testcases := []struct {
		name string
		data *common.IxConsensusPreference
	}{
		{
			name: "copy ix consensus preference with all populated fields",
			data: &common.IxConsensusPreference{
				MTQ:        1,
				TrustNodes: tests.RandomKramaIDs(t, 3),
			},
		},
		{
			name: "copy ix consensus preference with empty fields",
			data: &common.IxConsensusPreference{
				MTQ: 1,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedData := test.data

			dataCopy := test.data.Copy()

			require.Equal(t, expectedData, dataCopy)

			if expectedData.TrustNodes != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.TrustNodes).Pointer(),
					reflect.ValueOf(dataCopy.TrustNodes).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxPreferences(t *testing.T) {
	testcases := []struct {
		name string
		data *common.IxPreferences
	}{
		{
			name: "copy ix preferences with all populated fields",
			data: &common.IxPreferences{
				Compute: []byte{1, 2, 3},
				Consensus: &common.IxConsensusPreference{
					MTQ: 1,
				},
			},
		},
		{
			name: "copy ix preferences with empty fields",
			data: &common.IxPreferences{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedData := test.data

			dataCopy := test.data.Copy()

			require.Equal(t, expectedData, dataCopy)

			if expectedData.Compute != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Compute).Pointer(),
					reflect.ValueOf(dataCopy.Compute).Pointer(),
				)
			}

			if expectedData.Consensus != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Consensus).Pointer(),
					reflect.ValueOf(dataCopy.Consensus).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxData(t *testing.T) {
	testcases := []struct {
		name string
		data common.IxData
	}{
		{
			name: "copy ix data with all populated fields",
			data: tests.CreateIXDataWithTestData(t, nil),
		},
		{
			name: "copy ix data with empty fields",
			data: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.FuelPrice = nil
				ixData.Funds = nil
				ixData.IxOps = nil
				ixData.Participants = nil
				ixData.Preferences = nil
				ixData.Perception = nil
			}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedData := test.data

			dataCopy := test.data.Copy()

			require.Equal(t, expectedData, dataCopy)

			if expectedData.FuelPrice != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.FuelPrice).Pointer(),
					reflect.ValueOf(dataCopy.FuelPrice).Pointer(),
				)
			}

			if expectedData.Funds != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Funds).Pointer(),
					reflect.ValueOf(dataCopy.Funds).Pointer(),
				)
			}

			if expectedData.IxOps != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.IxOps).Pointer(),
					reflect.ValueOf(dataCopy.IxOps).Pointer(),
				)
			}

			if expectedData.Participants != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Participants).Pointer(),
					reflect.ValueOf(dataCopy.Participants).Pointer(),
				)
			}

			if expectedData.Preferences != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Preferences).Pointer(),
					reflect.ValueOf(dataCopy.Preferences).Pointer(),
				)
			}

			if expectedData.Perception != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedData.Perception).Pointer(),
					reflect.ValueOf(dataCopy.Perception).Pointer(),
				)
			}
		})
	}
}

func TestAccountType(t *testing.T) {
	ixns := common.NewInteractionsWithLeaderCheck(false,
		tests.CreateIX(t, &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				}
			},
		}),
		tests.CreateIX(t, &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.Sender = tests.RandomAddress(t)
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, identifiers.NilAddress),
					},
					{
						Type:    common.IxLogicDeploy,
						Payload: tests.CreateRawLogicPayload(t, tests.RandomAddress(t)),
					},
				}
			},
		}),
	)

	testcases := []struct {
		name         string
		address      identifiers.Address
		expectedType common.AccountType
		expectedErr  error
	}{
		{
			name:         "sender should be a regular account",
			address:      ixns.IxList()[0].Sender(),
			expectedType: common.RegularAccount,
		},
		{
			name:         "payer should be a regular account",
			address:      ixns.IxList()[1].Payer(),
			expectedType: common.RegularAccount,
		},
		{
			name:         "target should be an asset account",
			address:      ixns.IxList()[0].GetIxOp(0).Target(),
			expectedType: common.AssetAccount,
		},
		{
			name:         "target should be a logic account",
			address:      ixns.IxList()[1].GetIxOp(1).Target(),
			expectedType: common.LogicAccount,
		},
		{
			name:         "target should be a regular account",
			address:      ixns.IxList()[1].GetIxOp(0).Target(),
			expectedType: common.RegularAccount,
		},
		{
			name:        "account not found",
			address:     tests.RandomAddress(t),
			expectedErr: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountType, err := ixns.AccountType(test.address)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Contains(t, test.expectedErr.Error(), err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedType, accountType)
		})
	}
}

func TestPolorizeInteractions(t *testing.T) {
	ix1 := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
			}
		},
	})

	ix2 := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, identifiers.NilAddress),
				},
			}
		},
	})

	interactions := common.NewInteractionsWithLeaderCheck(true, ix1, ix2)

	polorizer, err := interactions.Polorize()
	require.NoError(t, err)

	bytes := polorizer.Bytes()
	require.NotEmpty(t, bytes)

	var depolorizedInteractions common.Interactions
	// err = depolorizedInteractions.FromBytes(bytes)
	// require.NoError(t, err)

	err = polo.Depolorize(&depolorizedInteractions, bytes)
	require.NoError(t, err)

	require.Equal(t, interactions.Len(), depolorizedInteractions.Len())
	require.Equal(t, interactions.LeaderCandidateAddress(), depolorizedInteractions.LeaderCandidateAddress())
}

// helper functions
func checkIxOperations(
	t *testing.T, ix *common.Interaction,
	assetCreatePayload common.AssetCreatePayload,
	assetActionPayload common.AssetActionPayload,
	assetSupplyPayload common.AssetSupplyPayload,
	logicPayload common.LogicPayload,
) {
	t.Helper()

	for _, op := range ix.Ops() {
		switch op.Type() {
		case common.IxAssetTransfer:
			payload, err := op.GetAssetActionPayload()
			require.NoError(t, err)
			require.Equal(t, assetActionPayload, *payload)
		case common.IxAssetCreate:
			payload, err := op.GetAssetCreatePayload()
			require.NoError(t, err)
			require.Equal(t, assetCreatePayload, *payload)
		case common.IxAssetMint, common.IxAssetBurn:
			payload, err := op.GetAssetSupplyPayload()
			require.NoError(t, err)
			require.Equal(t, assetSupplyPayload, *payload)
		case common.IxLogicDeploy, common.IxLogicInvoke, common.IxLogicEnlist:
			payload, err := op.GetLogicPayload()
			require.NoError(t, err)
			require.Equal(t, logicPayload, *payload)
		default:
			t.Fatalf("unsupported ixOp type: %v", op.Type())
		}
	}
}
