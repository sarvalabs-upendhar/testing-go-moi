package e2e

/*

func (te *TestEnvironment) createIxWithMultipleTxs(
	sender tests.AccountWithMnemonic,
	opTypes []common.IxOpType,
) *common.IxData {
	ixData := &common.IxData{
		Sender: common.Sender{
			ID: sender.ID,
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds:     make([]common.IxFund, 0),
		IxOps:     make([]common.IxOpRaw, 0),
		Participants: []common.IxParticipant{
			{
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
		},
	}

	for _, ixType := range opTypes {
		var (
			rawPayload []byte
			err        error
		)

		switch ixType {
		case common.IxInvalid:
			rawPayload, err = []byte{}, nil

		case common.IxParticipantCreate:
			id := tests.RandomIdentifierWithZeroVariant(te.T())
			participantRegisterPayload := &common.ParticipantCreatePayload{
				ID: id,
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          id.Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(300),
			}

			rawPayload, err = participantRegisterPayload.Bytes()

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       participantRegisterPayload.ID,
				LockType: common.MutateLock,
			})

		case common.IxAssetTransfer:
			assetID := createAsset(te, sender, &common.AssetCreatePayload{
				Symbol:    tests.GetRandomUpperCaseString(te.T(), 5),
				MaxSupply: big.NewInt(5000),
				Standard:  common.MAS0,
			})

			id := tests.RandomIdentifierWithZeroVariant(te.T())

			createParticipant(te, sender, &common.ParticipantCreatePayload{
				ID: id,
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          id.Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(1),
			})

			assetActionPayload := &common.AssetActionPayload{
				Beneficiary: id,
				AssetID:     assetID,
				Amount:      big.NewInt(1000),
			}

			rawPayload, err = assetActionPayload.Bytes()

			ixData.Funds = append(ixData.Funds, common.IxFund{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			})

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       assetActionPayload.Beneficiary,
				LockType: common.MutateLock,
			})

		case common.IxAssetCreate:
			assetCreatePayload := createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 5),
				big.NewInt(1000), common.MAS0,
				nil,
			)

			rawPayload, err = assetCreatePayload.Bytes()

		case common.IxAssetMint, common.IxAssetBurn:
			assetID := createAsset(te, sender, &common.AssetCreatePayload{
				Symbol:   tests.GetRandomUpperCaseString(te.T(), 5),
				Supply:   big.NewInt(5000),
				Standard: common.MAS0,
			})

			assetSupplyPayload := &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			}

			rawPayload, err = assetSupplyPayload.Bytes()

			ixData.Funds = append(ixData.Funds, common.IxFund{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			})

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       assetID.AsIdentifier(),
				LockType: common.MutateLock,
			})

		case common.IxLogicDeploy:
			logicDeployPayload := &common.LogicPayload{
				Manifest: common.Hex2Bytes(ledgerManifest),
				LogicID:  identifiers.Nil,
				Callsite: "Seed",
				Calldata: DeployCallData.Bytes(),
			}

			rawPayload, err = logicDeployPayload.Bytes()

		case common.IxLogicInvoke:
			logicID := deployLogic(te, sender, &common.LogicPayload{
				Manifest: common.Hex2Bytes(ledgerManifest),
				Callsite: "Seed",
				Calldata: DeployCallData.Bytes(),
			})

			invokeCalldata := DocGen(te.T(), map[string]any{
				"amount":   transferAmount,
				"receiver": DefaultBeneficiary,
			}).Bytes()

			logicInvokePayload := &common.LogicPayload{
				Manifest: []byte{},
				LogicID:  logicID,
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			}

			rawPayload, err = logicInvokePayload.Bytes()

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       logicID.AsIdentifier(),
				LockType: common.MutateLock,
			})
		default:
			continue
		}

		require.NoError(te.T(), err)

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		})
	}

	ixData.Sender.SequenceID = moiclient.GetLatestSequenceID(te.T(), te.moiClient, sender.ID, 0)

	return ixData
}

func validateOperations(
	te *TestEnvironment, sender identifiers.Identifier,
	ixHash common.Hash, txs []common.IxOpRaw,
) {
	te.T()

	for idx, op := range txs {
		switch op.Type {
		case common.IxParticipantCreate:
			payload := new(common.ParticipantCreatePayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateParticipantCreate(te, sender, payload, ixHash)

		case common.IxAssetTransfer:
			payload := new(common.AssetActionPayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateAssetTransfer(te, sender, payload, ixHash)

		case common.IxAssetCreate:
			payload := new(common.AssetCreatePayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateAssetCreation(te, sender, ixHash, idx, payload)

		case common.IxAssetMint:
			payload := new(common.AssetSupplyPayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateAssetMint(te, sender, *payload, ixHash)

		case common.IxAssetBurn:
			payload := new(common.AssetSupplyPayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateAssetBurn(te, sender, *payload, ixHash)

		case common.IxLogicDeploy:
			payload := new(common.LogicPayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateLogicDeploy(te, sender, payload, idx, ixHash)

		case common.IxLogicInvoke:
			payload := new(common.LogicPayload)

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateLogicInvoke(te, sender, DefaultBeneficiary, payload, ixHash)

		default:
			continue
		}
	}
}

func (te *TestEnvironment) TestOperations() {
	accounts, err := te.chooseRandomUniqueAccounts(4)
	require.NoError(te.T(), err)

	testcases := []struct {
		name          string
		account       tests.AccountWithMnemonic
		ixData        *common.IxData
		expectedError error
	}{
		{
			name:    "max ixOps limit exceeded",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxAssetMint,
				common.IxAssetTransfer,
				common.IxAssetTransfer,
			}),
			expectedError: api.ErrTooManyIxOps,
		},
		{
			name:    "interaction invalid operation type",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxAssetMint,
				common.IxInvalid,
			}),
			expectedError: common.ErrInvalidInteractionType,
		},
		{
			name:    "multiple asset create interactions",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxAssetCreate,
				common.IxLogicDeploy,
			}),
			expectedError: api.ErrAssetCreationLimit,
		},
		{
			name:    "multiple logic deploy interactions",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxLogicDeploy,
				common.IxLogicDeploy,
			}),
			expectedError: api.ErrLogicDeploymentLimit,
		},
		{
			name:    "valid interaction with asset operations",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxAssetMint,
				common.IxAssetTransfer,
			}),
		},
		{
			name:    "valid interaction with logic operations",
			account: accounts[1],
			ixData: te.createIxWithMultipleTxs(accounts[1], []common.IxOpType{
				common.IxLogicDeploy,
				common.IxLogicInvoke,
			}),
		},
		{
			name:    "valid interaction with participant create operations",
			account: accounts[3],
			ixData: te.createIxWithMultipleTxs(accounts[3], []common.IxOpType{
				common.IxParticipantCreate,
				common.IxAssetCreate,
			}),
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			sendIX := moiclient.CreateSendIXFromIxData(te.T(), test.ixData, []moiclient.AccountKeyWithMnemonic{
				{
					ID:       test.account.ID,
					KeyID:    0,
					Mnemonic: test.account.Mnemonic,
				},
			})

			ixHash, err := te.moiClient.SendInteractions(context.Background(), sendIX)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			validateOperations(te, test.ixData.Sender.ID, ixHash, test.ixData.IxOps)
		})
	}
}

*/
