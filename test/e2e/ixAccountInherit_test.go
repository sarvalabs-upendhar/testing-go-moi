package e2e

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func validateKMOIAssetTransfer(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.AssetActionPayload,
	fuelKMOI uint64,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)
	receiverHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, payload.Beneficiary)

	senderPrevBal := getBalance(te, sender, payload.AssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.AssetID, args.LatestTesseractHeight)

	receiverPrevBal := getBalance(te, payload.Beneficiary, payload.AssetID, int64(receiverHeight-1))
	receiverCurBal := getBalance(te, payload.Beneficiary, payload.AssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64()+fuelKMOI, senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal-receiverPrevBal)
}

// 1. ensure sub account count increased
// 2. make sure account keys of sender and sub account are same
// 3. make sure consensus nodes of sender and logic are same
// 4. ensure amount transferred from sender to sub account
func validateAccountInherit(
	te *TestEnvironment,
	sender identifiers.Identifier,
	ixHash common.Hash,
	txnID int,
	payload *common.AccountInheritPayload,
) {
	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var accountInheritResult common.AccountInheritResult

	err := json.Unmarshal(receipt.IxOps[txnID].Data, &accountInheritResult)
	require.NoError(te.T(), err)

	subAccount := accountInheritResult.SubAccount

	subAccountCount, err := te.moiClient.SubAccountCount(context.Background(), &args.SubAccountCountArgs{
		ID: sender,
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), payload.SubAccountIndex, uint32(subAccountCount.ToUint64()))

	senderKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
		ID: sender,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	subAccountKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
		ID: subAccount,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), len(senderKeys), len(subAccountKeys))

	for i := 0; i < len(senderKeys); i++ {
		require.Equal(te.T(), senderKeys[i].ID, subAccountKeys[i].ID)
		require.Equal(te.T(), senderKeys[i].PublicKey, subAccountKeys[i].PublicKey)
		require.Equal(te.T(), senderKeys[i].Weight, subAccountKeys[i].Weight)
		require.Equal(te.T(), senderKeys[i].SignatureAlgorithm, subAccountKeys[i].ID)
		require.Equal(te.T(), senderKeys[i].Revoked, subAccountKeys[i].Revoked)
		// sequence id of sub account should start from zero
		require.Equal(te.T(), uint64(0), subAccountKeys[i].SequenceID.ToUint64())
	}

	targetConsensusNodes, err := te.moiClient.ContextInfo(context.Background(), &args.ContextInfoArgs{
		ID: payload.TargetAccount,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	subAccountConsensusNodes, err := te.moiClient.ContextInfo(context.Background(), &args.ContextInfoArgs{
		ID: subAccount,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), targetConsensusNodes.ConsensusNodes, subAccountConsensusNodes.ConsensusNodes)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, common.KMOITokenAssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, common.KMOITokenAssetID, args.LatestTesseractHeight)

	receiverCurBal := getBalance(te, subAccount, common.KMOITokenAssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64()+compute.FuelAccountInherit, senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal)
}

//nolint:dupl
func (te *TestEnvironment) inheritAccount(
	senderID identifiers.Identifier,
	senderKeyID uint64,
	accountInheritPayload *common.AccountInheritPayload,
	accKeys []moiclient.AccountKeyWithMnemonic,
) (common.Hash, error) {
	te.logger.Debug("inherit account ", "sender", senderID)

	payload, err := accountInheritPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         senderID,
			KeyID:      senderKeyID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, senderID, senderKeyID),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IXAccountInherit,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       senderID,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, accKeys)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func (te *TestEnvironment) TestIXAccountInherit() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	invalidTarget := accs[1]

	ixHash, err := te.deployLogic(
		sender,
		&common.LogicPayload{
			Callsite: "Seed",
			Calldata: DeployCallData.Bytes(),
			Manifest: common.Hex2Bytes(ledgerManifest),
		},
	)
	require.NoError(te.T(), err)

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	ledgerLogicID := moiclient.GetLogicID(te.T(), te.moiClient, 0, sender.ID, args.LatestTesseractHeight)

	senderSubAccountCount, err := te.moiClient.SubAccountCount(context.Background(), &args.SubAccountCountArgs{
		ID: sender.ID,
	})
	require.NoError(te.T(), err)

	testcases := []struct {
		name                  string
		senderAddr            identifiers.Identifier
		senderKeyID           uint64
		accountInheritPayload *common.AccountInheritPayload
		signers               []moiclient.AccountKeyWithMnemonic
		expectedError         error
	}{
		{
			name:        "inherit context of logic to sub account",
			senderAddr:  sender.ID,
			senderKeyID: 0,
			accountInheritPayload: &common.AccountInheritPayload{
				TargetAccount:   ledgerLogicID.AsIdentifier(),
				Amount:          big.NewInt(1000),
				SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 1,
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       sender.ID,
					KeyID:    0,
					Mnemonic: sender.Mnemonic,
				},
			},
		},
		{
			name:        "invalid target account",
			senderAddr:  sender.ID,
			senderKeyID: 0,
			accountInheritPayload: &common.AccountInheritPayload{
				TargetAccount:   invalidTarget.ID,
				Amount:          big.NewInt(1000),
				SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 1,
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       sender.ID,
					KeyID:    0,
					Mnemonic: sender.Mnemonic,
				},
			},
			expectedError: common.ErrInvalidTargetAccount,
		},
		{
			name:        "invalid amount",
			senderAddr:  sender.ID,
			senderKeyID: 0,
			accountInheritPayload: &common.AccountInheritPayload{
				TargetAccount:   ledgerLogicID.AsIdentifier(),
				Amount:          big.NewInt(0),
				SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 2,
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       sender.ID,
					KeyID:    0,
					Mnemonic: sender.Mnemonic,
				},
			},
			expectedError: common.ErrInvalidValue,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.inheritAccount(test.senderAddr, test.senderKeyID,
				test.accountInheritPayload, test.signers)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)
			validateAccountInherit(te, sender.ID, ixHash, 0, test.accountInheritPayload)
		})
	}
}

func (te *TestEnvironment) createSubAccount(
	sender tests.AccountWithMnemonic,
	logicID identifiers.LogicID,
) tests.AccountWithMnemonic {
	senderSubAccountCount, err := te.moiClient.SubAccountCount(context.Background(), &args.SubAccountCountArgs{
		ID: sender.ID,
	})
	require.NoError(te.T(), err)

	accInheritPayload := &common.AccountInheritPayload{
		TargetAccount:   logicID.AsIdentifier(),
		Amount:          big.NewInt(100000),
		SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 1,
	}

	ixHash, err := te.inheritAccount(sender.ID, 0, accInheritPayload, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       sender.ID,
			KeyID:    0,
			Mnemonic: sender.Mnemonic,
		},
	})
	require.NoError(te.T(), err)

	validateAccountInherit(te, sender.ID, ixHash, 0, accInheritPayload)

	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var accountInheritResult common.AccountInheritResult

	err = json.Unmarshal(receipt.IxOps[0].Data, &accountInheritResult)
	require.NoError(te.T(), err)

	return tests.AccountWithMnemonic{
		ID:       accountInheritResult.SubAccount,
		Mnemonic: sender.Mnemonic,
	}
}

func (te *TestEnvironment) TestSubAccountInteractions() {
	transferAsset := func(t *testing.T, sender, receiver tests.AccountWithMnemonic) {
		t.Helper()

		assetActionPayload := &common.AssetActionPayload{
			Beneficiary: receiver.ID,
			AssetID:     common.KMOITokenAssetID,
			Amount:      big.NewInt(2),
		}

		ixHash, err := te.transferAsset(sender, assetActionPayload)
		require.NoError(t, err)

		validateKMOIAssetTransfer(te, sender.ID, assetActionPayload, compute.FuelSimpleAssetTransfer, ixHash)
	}

	// generates one  logic invoke ixns for each sub account
	invokeLogic := func(subAccounts []tests.AccountWithMnemonic, logicID identifiers.LogicID) []*args.SendIX {
		sendIXArgs := make([]*args.SendIX, len(subAccounts))

		receiver := identifiers.RandomParticipantIDv0().AsIdentifier()

		for i, sender := range subAccounts {
			// tries to fetch balance of receiver
			invokeCalldata := DocGen(te.T(), map[string]any{
				"addr": receiver,
			}).Bytes()

			logicPayload := &common.LogicPayload{
				Logic:    logicID,
				Callsite: "BalanceOf",
				Calldata: invokeCalldata,
			}

			arg, err := te.getLogicInvoke(sender, logicPayload)
			if err != nil {
				panic(err)
			}

			sendIXArgs[i] = arg
		}

		return sendIXArgs
	}

	sender := te.chooseRandomAccount()

	// create the logic
	ixHash, err := te.deployLogic(
		sender,
		&common.LogicPayload{
			Callsite: "Seed",
			Calldata: DeployCallData.Bytes(),
			Manifest: common.Hex2Bytes(ledgerManifest),
		},
	)
	require.NoError(te.T(), err)

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	// create sub accounts
	ledgerLogicID := moiclient.GetLogicID(te.T(), te.moiClient, 0, sender.ID, args.LatestTesseractHeight)
	num := 5
	subAccounts := make([]tests.AccountWithMnemonic, num)

	for i := 0; i < num; i++ {
		subAccounts[i] = te.createSubAccount(sender, ledgerLogicID)
	}

	te.T().Run("logic invoke by sub accounts", func(t *testing.T) {
		sendIXArgs := invokeLogic(subAccounts, ledgerLogicID)
		ixHashes := make([]common.Hash, 0, num)

		for _, sendIX := range sendIXArgs {
			ixHash, err = te.moiClient.SendInteractions(context.Background(), sendIX)
			require.NoError(t, err)

			ixHashes = append(ixHashes, ixHash)
		}

		// ensure ixns are batched into a single tesseract
		ixReceipt := checkForReceiptSuccess(t, te.moiClient, ixHashes[0])

		for _, ixHash := range ixHashes[1:] {
			receipt := checkForReceiptSuccess(t, te.moiClient, ixHash)
			require.Equal(t, ixReceipt.TSHash, receipt.TSHash)
		}
	})

	te.T().Run("asset transfer between different types of accounts", func(t *testing.T) {
		transferAsset(t, subAccounts[0], sender)
		transferAsset(t, subAccounts[0], subAccounts[1])
		transferAsset(t, subAccounts[1], subAccounts[0])
		transferAsset(t, sender, subAccounts[0])
	})
}

func (te *TestEnvironment) getLogicInvoke(
	sender tests.AccountWithMnemonic,
	logicPayload *common.LogicPayload,
) (*args.SendIX, error) {
	te.logger.Debug("invoke logic ",
		"sender", sender.ID,
		"logicID", logicPayload.Logic,
		"callsite", logicPayload.Callsite,
		"calldata", logicPayload.Calldata,
	)

	payload, err := logicPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         sender.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, sender.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicInvoke,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       logicPayload.Logic.AsIdentifier(),
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       sender.ID,
			KeyID:    0,
			Mnemonic: sender.Mnemonic,
		},
	})

	return sendIX, nil
}
