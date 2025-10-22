package common_test

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/stretchr/testify/require"
)

func TestNewInteraction(t *testing.T) {
	assetCreatePayload := &common.AssetCreatePayload{Symbol: "MOI", MaxSupply: big.NewInt(500), Standard: common.MAS0}
	rawAssetCreatePayload, _ := assetCreatePayload.Bytes()

	assetActionPayload, err := common.GetAssetActionPayload(common.KMOITokenAssetID, common.TransferEndpoint,
		&common.TransferParams{
			Beneficiary: tests.RandomIdentifier(t),
			Amount:      big.NewInt(500),
		})

	require.NoError(t, err)

	rawParticipantCreate, err := tests.CreateParticipantCreatePayload(t, tests.RandomIdentifier(t)).Bytes()
	require.NoError(t, err)

	rawAssetActionPayload, _ := assetActionPayload.Bytes()

	logicPayload := &common.LogicPayload{
		Manifest: []byte{2, 1, 5, 9},
		Callsite: "hello",
		Calldata: []byte{0, 7, 8, 1},
		Interfaces: map[string]identifiers.Identifier{
			"hello": tests.GetLogicID(t, tests.RandomIdentifier(t)),
		},
	}
	rawLogicPayload, _ := logicPayload.Bytes()

	targetAccount := tests.RandomIdentifierWithZeroVariant(t)

	accountInheritPayload := &common.AccountInheritPayload{
		TargetAccount: targetAccount,
		Value: tests.AssetActionPayload(t, common.KMOITokenAssetID, common.TransferEndpoint, &common.TransferParams{
			Beneficiary: targetAccount, // this is just a placeholder
			Amount:      big.NewInt(500),
		}),
		SubAccountIndex: 3,
	}
	rawAccountInheritPayload, _ := accountInheritPayload.Bytes()

	signatures := common.Signatures{
		{
			ID:        tests.RandomIdentifier(t),
			KeyID:     2,
			Signature: []byte{1, 2, 3},
		},
	}

	testcases := []struct {
		name        string
		ixData      common.IxData
		expectedIX  *common.Interaction
		expectedErr error
	}{
		{
			name: "asset transfer ix",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetAction,
						Payload: rawAssetActionPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID: assetActionPayload.AssetID.AsIdentifier(),
				})
			}),
		},
		{
			name: "missing asset account in participant create",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxParticipantCreate,
						Payload: rawParticipantCreate,
					},
				}
			}),
			expectedErr: common.ErrMissingAssetAccount,
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
						ID: logicPayload.LogicID.AsIdentifier(),
					},
					{
						ID: logicPayload.Interfaces["hello"],
					},
				}...)
			}),
		},
		{
			name: "inherit account",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAccountInherit,
						Payload: rawAccountInheritPayload,
					},
				}
				ixData.Participants = append(ixData.Participants, []common.IxParticipant{
					{
						ID: accountInheritPayload.Value.AssetID.AsIdentifier(),
					},
				}...)
			}),
		},
		{
			name: "missing asset account in participants for account inherit",
			ixData: tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
				ixData.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAccountInherit,
						Payload: rawAccountInheritPayload,
					},
				}
			}),
			expectedErr: common.ErrMissingAssetAccount,
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
						ID: logicPayload.LogicID.AsIdentifier(),
					},
				}...)
			}),
			expectedErr: common.ErrMissingForeignLogicAccount,
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
			expectedErr: common.ErrInvalidInteractionType,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, err := common.NewInteraction(test.ixData, signatures)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)

			// check if ix data copied properly
			require.Equal(t, test.ixData.Sender.SequenceID, ix.SequenceID())
			require.Equal(t, test.ixData.Sender, ix.Sender())
			require.Equal(t, test.ixData.Payer, ix.Payer())
			require.Equal(t, test.ixData.FuelPrice, ix.FuelPrice())
			require.Equal(t, test.ixData.FuelLimit, ix.FuelLimit())
			require.Equal(t, test.ixData.Funds, ix.Funds())
			require.Equal(t, test.ixData.Participants, ix.IxParticipants())
			require.Equal(t, test.ixData.Perception, ix.Perception())
			require.Equal(t, test.ixData.Preferences, ix.Preferences())

			require.Equal(t, signatures, ix.Signatures())

			data, err := polo.Polorize(test.ixData)
			require.NoError(t, err)

			require.Equal(t, common.GetHash(data), ix.Hash())

			size, err := ix.Size()
			require.NoError(t, err)

			rawSig, err := ix.Signatures().Bytes()
			require.NoError(t, err)

			require.Equal(t, uint64(len(data)+len(rawSig)), size)

			// check for payload
			checkIxOperations(t, ix, assetCreatePayload, assetActionPayload,
				accountInheritPayload, logicPayload)

			checkForIxParticipants(t, test.ixData, test.ixData.IxOps[0], ix.Participants())
		})
	}
}

func TestUpdateLeaderCandidateAddr(t *testing.T) {
	// we create a max sender id to test the leader candidate id
	senderID, err := identifiers.GenerateParticipantIDv0(tests.Max24Byte(t), 0)
	require.NoError(t, err)

	// we create a min sender id to test the leader candidate id
	psID, err := identifiers.GenerateParticipantIDv0(tests.Min24Byte(t, 1), 0)
	require.NoError(t, err)

	regularAccount := psID.AsIdentifier()

	logicID, err := identifiers.GenerateLogicIDv0(identifiers.RandomFingerprint(), 0)
	require.NoError(t, err)

	nonRegularAccount1, _ := logicID.AsIdentifier().DeriveVariant(1, nil, nil)

	nonRegularAccount2, err := logicID.AsIdentifier().DeriveVariant(2, nil, nil)
	require.NoError(t, err)

	testcases := []struct {
		name              string
		createIxParams    tests.CreateIxParams
		expectedLeaderAcc identifiers.Identifier
	}{
		{
			name: "sarga account is selected as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					tests.AddIxOp(
						t,
						ix,
						common.IxLogicInvoke, common.KMOITokenAssetID,
						tests.CreateLogicPayload(t, identifiers.MustLogicID(nonRegularAccount1)),
					)

					tests.AddParticipants(t, ix, []common.IxParticipant{
						{
							ID:       common.SargaAccountID,
							LockType: common.MutateLock,
						},
						{
							ID:       regularAccount,
							LockType: common.MutateLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: common.SargaAccountID,
		},
		{
			name: "first account from sorted regular accounts is chosen as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Sender.ID = senderID.AsIdentifier()
					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							ID:       regularAccount,
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
					tests.AddIxOp(
						t, ix,
						common.IxLogicInvoke,
						common.KMOITokenAssetID,
						tests.CreateLogicPayload(t, identifiers.MustLogicID(nonRegularAccount1)),
					)
					tests.AddIxOp(
						t,
						ix,
						common.IxLogicInvoke,
						common.KMOITokenAssetID,
						tests.CreateLogicPayload(t, identifiers.MustLogicID(nonRegularAccount2)),
					)
					tests.AddParticipants(t, ix, []common.IxParticipant{
						{
							ID:       regularAccount,
							LockType: common.MutateLock,
						},
					}...)
				},
			},
			expectedLeaderAcc: nonRegularAccount1,
		},
		{
			name: "non-regular account with read lock is not chosen as leader",
			createIxParams: tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.IxOps = append(ix.IxOps, common.IxOpRaw{
						Type:    common.IxLogicInvoke,
						Payload: tests.CreateRawLogicPayload(t, identifiers.MustLogicID(nonRegularAccount1)),
					})
					ix.Participants = append(ix.Participants, []common.IxParticipant{
						{
							ID:       regularAccount,
							LockType: common.MutateLock,
						},
						{
							ID:       nonRegularAccount1,
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
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Amount:  big.NewInt(5000),
			},
		},
		{
			name: "copy ix fund with empty fields",
			data: common.IxFund{
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
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

func TestPolorizeInteractions(t *testing.T) {
	ix1 := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			tests.AddIxOp(t, ix, common.IxAssetCreate, common.KMOITokenAssetID, tests.CreateAssetCreatePayload(t))
		},
	})

	ix2 := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			tests.AddIxOp(
				t,
				ix,
				common.IxAssetAction,
				common.KMOITokenAssetID,
				tests.CreateAssetTransferPayload(t, identifiers.Nil),
			)
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
	require.Equal(t, interactions.LeaderCandidateID(), depolorizedInteractions.LeaderCandidateID())
}

// helper functions
func checkIxOperations(
	t *testing.T, ix *common.Interaction,
	assetCreatePayload *common.AssetCreatePayload,
	assetActionPayload *common.AssetActionPayload,
	accountInheritPayload *common.AccountInheritPayload,
	logicPayload *common.LogicPayload,
) {
	t.Helper()

	for _, op := range ix.Ops() {
		switch op.Type() {
		// TODO: Add more interaction types
		case common.IxAssetAction:
			payload, err := op.GetAssetActionPayload()
			require.NoError(t, err)
			require.Equal(t, assetActionPayload, payload)
		case common.IxAssetCreate:
			payload, err := op.GetAssetCreatePayload()
			require.NoError(t, err)
			require.Equal(t, assetCreatePayload, payload)
		case common.IxLogicDeploy, common.IxLogicInvoke, common.IxLogicEnlist:
			payload, err := op.GetLogicPayload()
			require.NoError(t, err)
			require.Equal(t, logicPayload, payload)
		case common.IxAccountInherit:
			payload, err := op.GetAccountInheritPayload()
			require.NoError(t, err)

			require.Equal(t, accountInheritPayload, payload)

		default:
			t.Fatalf("unsupported ixOp type: %v", op.Type())
		}
	}
}

func checkForIxParticipants(
	t *testing.T,
	ixData common.IxData,
	op common.IxOpRaw,
	ps map[identifiers.Identifier]*common.ParticipantInfo,
) {
	t.Helper()

	var id identifiers.Identifier

	if op.Type == common.IxAssetCreate {
		assetCreatePayload := new(common.AssetCreatePayload)
		if err := assetCreatePayload.FromBytes(op.Payload); err != nil {
			require.NoError(t, err)
		}

		assetID, err := identifiers.GenerateAssetIDv0(
			common.NewAccountID(ixData.Sender),
			0,
			uint16(assetCreatePayload.Standard),
			assetCreatePayload.Flags()...,
		)

		require.NoError(t, err)

		id = assetID.AsIdentifier()
	}

	if op.Type == common.IxLogicDeploy {
		logicPayload := new(common.LogicPayload)
		if err := logicPayload.FromBytes(op.Payload); err != nil {
			require.NoError(t, err)
		}

		logicID, _ := identifiers.GenerateLogicIDv0(
			common.NewAccountID(ixData.Sender),
			0,
			logicPayload.Flags()...,
		)

		id = logicID.AsIdentifier()
	}

	if op.Type == common.IxAccountInherit {
		accInheritPayload := new(common.AccountInheritPayload)
		if err := accInheritPayload.FromBytes(op.Payload); err != nil {
			require.NoError(t, err)
		}

		subAccount, err := ixData.Sender.ID.DeriveVariant(accInheritPayload.SubAccountIndex, nil, nil)
		require.NoError(t, err)

		id = subAccount
	}

	if !id.IsNil() {
		_, ok := ps[id]
		require.True(t, ok)

		_, ok = ps[common.SargaAccountID]
		require.True(t, ok)
	}
}
