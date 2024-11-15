package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/jsonrpc/api"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) createIxWithMultipleTxs(
	sender tests.AccountWithMnemonic,
	opTypes []common.IxOpType,
) *common.IxData {
	ixData := &common.IxData{
		Sender:    sender.Addr,
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds:     make([]common.IxFund, 0),
		IxOps:     make([]common.IxOpRaw, 0),
		Participants: []common.IxParticipant{
			{
				Address:  sender.Addr,
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
			participantRegisterPayload := &common.ParticipantCreatePayload{
				Address: tests.RandomAddress(te.T()),
				Amount:  big.NewInt(300),
			}

			rawPayload, err = participantRegisterPayload.Bytes()

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				Address:  participantRegisterPayload.Address,
				LockType: common.MutateLock,
			})

		case common.IxAssetTransfer:
			assetID := createAsset(te, sender, &common.AssetCreatePayload{
				Symbol:   tests.GetRandomUpperCaseString(te.T(), 5),
				Supply:   big.NewInt(5000),
				Standard: common.MAS0,
			})

			addr := tests.RandomAddress(te.T())

			createParticipant(te, sender, &common.ParticipantCreatePayload{
				Address: addr,
				Amount:  big.NewInt(1),
			})

			assetActionPayload := &common.AssetActionPayload{
				Beneficiary: addr,
				AssetID:     assetID,
				Amount:      big.NewInt(1000),
			}

			rawPayload, err = assetActionPayload.Bytes()

			ixData.Funds = append(ixData.Funds, common.IxFund{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			})

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				Address:  assetActionPayload.Beneficiary,
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
				Address:  assetID.Address(),
				LockType: common.MutateLock,
			})

		case common.IxLogicDeploy:
			logicDeployPayload := &common.LogicPayload{
				Manifest: common.Hex2Bytes(ledgerManifest),
				Logic:    "",
				Callsite: "Seed",
				Calldata: common.Hex2Bytes(deployCalldata),
			}

			rawPayload, err = logicDeployPayload.Bytes()

		case common.IxLogicInvoke:
			logicID := deployLogic(te, sender, &common.LogicPayload{
				Manifest: common.Hex2Bytes(ledgerManifest),
				Callsite: "Seed",
				Calldata: common.Hex2Bytes(deployCalldata),
			})

			invokeCalldata := "0x0d6f0665a601a502616d6f756e74030f42407265636569766572060fafe52ec42a85db6" +
				"44d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"

			logicInvokePayload := &common.LogicPayload{
				Manifest: []byte{},
				Logic:    logicID,
				Callsite: "Transfer",
				Calldata: common.Hex2Bytes(invokeCalldata),
			}

			rawPayload, err = logicInvokePayload.Bytes()

			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				Address:  logicID.Address(),
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

	ixData.Nonce = moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr)

	return ixData
}

func validateTransactions(
	te *TestEnvironment, sender identifiers.Address,
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
			receiver, _ := identifiers.NewAddressFromHex("0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c")

			err := payload.FromBytes(op.Payload)
			require.NoError(te.T(), err)

			validateLogicInvoke(te, sender, receiver, payload, ixHash)

		default:
			continue
		}
	}
}

func (te *TestEnvironment) TestTransactions() {
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
			name:    "valid interaction with asset transactions",
			account: accounts[0],
			ixData: te.createIxWithMultipleTxs(accounts[0], []common.IxOpType{
				common.IxAssetCreate,
				common.IxAssetMint,
				common.IxAssetTransfer,
			}),
		},
		{
			name:    "valid interaction with logic transactions",
			account: accounts[1],
			ixData: te.createIxWithMultipleTxs(accounts[1], []common.IxOpType{
				common.IxLogicDeploy,
				common.IxLogicInvoke,
			}),
		},
		{
			name:    "valid interaction with both asset and logic transactions",
			account: accounts[2],
			ixData: te.createIxWithMultipleTxs(accounts[2], []common.IxOpType{
				common.IxLogicDeploy,
				common.IxAssetCreate,
				common.IxLogicInvoke,
			}),
		},
		{
			name:    "valid interaction with participant create transactions",
			account: accounts[3],
			ixData: te.createIxWithMultipleTxs(accounts[3], []common.IxOpType{
				common.IxParticipantCreate,
				common.IxAssetCreate,
			}),
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			sendIX := moiclient.CreateSendIXFromIxData(te.T(), test.ixData, test.account.Mnemonic)

			ixHash, err := te.moiClient.SendInteractions(context.Background(), sendIX)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			validateTransactions(te, test.ixData.Sender, ixHash, test.ixData.IxOps)
		})
	}
}
