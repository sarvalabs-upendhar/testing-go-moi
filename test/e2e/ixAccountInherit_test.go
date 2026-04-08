package e2e

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func validateKMOIAssetTransfer(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.PayoutDetails,
	ixHash common.Hash,
) {
	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)
	receiverHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, payload.Beneficiary)

	senderPrevBal := getBalance(te, sender, payload.AssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.AssetID, args.LatestTesseractHeight)

	receiverPrevBal := getBalance(te, payload.Beneficiary, payload.AssetID, int64(receiverHeight-1))
	receiverCurBal := getBalance(te, payload.Beneficiary, payload.AssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64()+receipt.FuelUsed.ToUint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal-receiverPrevBal)
}

// 1. ensure sub account count increased
// 2. make sure account keys of sender and sub account are same
// 3. make sure consensus nodes of sender and logic are same
// 4. ensure amount transferred from sender to sub account
func validateAccountInherit(
	te *TestEnvironment,
	isFailure bool,
	sender identifiers.Identifier,
	ixHash common.Hash,
	txnID int,
	payload *common.AccountInheritPayload,
	amount *big.Int,
) {
	if isFailure {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

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

	require.Equal(te.T(), amount.Uint64()+receipt.FuelUsed.ToUint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), amount.Uint64(), receiverCurBal)
}

//nolint:dup
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
				Type:    common.IxAccountInherit,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       senderID,
				LockType: common.MutateLock,
			},
			{
				ID:       common.KMOITokenAccountID,
				LockType: common.NoLock,
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

	ixHash, err := te.logicDeploy(
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
		name            string
		senderAddr      identifiers.Identifier
		senderKeyID     uint64
		TargetAccount   identifiers.Identifier
		SubAccountIndex uint32
		Amount          *big.Int
		AssetID         identifiers.AssetID
		TokenID         common.TokenID
		signers         []moiclient.AccountKeyWithMnemonic
		expectedError   error
		isFailure       bool
	}{
		{
			name:            "inherit context of logic to sub account",
			senderAddr:      sender.ID,
			senderKeyID:     0,
			TargetAccount:   ledgerLogicID.AsIdentifier(),
			SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 1,
			AssetID:         common.KMOITokenAssetID,
			TokenID:         common.DefaultTokenID,
			Amount:          big.NewInt(1000),

			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       sender.ID,
					KeyID:    0,
					Mnemonic: sender.Mnemonic,
				},
			},
		},
		{
			name:            "invalid target account",
			senderAddr:      sender.ID,
			senderKeyID:     0,
			TargetAccount:   invalidTarget.ID,
			SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 1,
			AssetID:         common.KMOITokenAssetID,
			TokenID:         common.DefaultTokenID,
			Amount:          big.NewInt(1000),
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
			name:            "invalid amount",
			senderAddr:      sender.ID,
			senderKeyID:     0,
			TargetAccount:   ledgerLogicID.AsIdentifier(),
			SubAccountIndex: uint32(senderSubAccountCount.ToUint64()) + 2,
			AssetID:         common.KMOITokenAssetID,
			TokenID:         common.DefaultTokenID,
			Amount:          big.NewInt(0),
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       sender.ID,
					KeyID:    0,
					Mnemonic: sender.Mnemonic,
				},
			},
			isFailure: true,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			subAccountID, err := test.senderAddr.DeriveVariant(test.SubAccountIndex, nil, nil)
			require.NoError(te.T(), err)

			action, err := common.GetAssetActionPayload(test.AssetID, common.TransferEndpoint, &common.TransferParams{
				Beneficiary: subAccountID,
				Amount:      test.Amount,
			})
			require.NoError(te.T(), err)

			payload := &common.AccountInheritPayload{
				TargetAccount:   test.TargetAccount,
				SubAccountIndex: test.SubAccountIndex,
				Value:           action,
			}

			ixHash, err = te.inheritAccount(test.senderAddr, test.senderKeyID,
				payload, test.signers)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)
			validateAccountInherit(te, test.isFailure, sender.ID, ixHash, 0, payload, test.Amount)
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

	subAccountIndex := uint32(senderSubAccountCount.ToUint64()) + 1

	subAccountID, err := sender.ID.DeriveVariant(subAccountIndex, nil, nil)
	require.NoError(te.T(), err)

	action, err := common.GetAssetActionPayload(common.KMOITokenAssetID, common.TransferEndpoint, &common.TransferParams{
		Beneficiary: subAccountID,
		Amount:      big.NewInt(100000),
	})
	require.NoError(te.T(), err)

	payload := &common.AccountInheritPayload{
		TargetAccount:   logicID.AsIdentifier(),
		SubAccountIndex: subAccountIndex,
		Value:           action,
	}

	ixHash, err := te.inheritAccount(sender.ID, 0, payload, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       sender.ID,
			KeyID:    0,
			Mnemonic: sender.Mnemonic,
		},
	})
	require.NoError(te.T(), err)

	validateAccountInherit(te, false, sender.ID, ixHash, 0, payload, big.NewInt(100000))

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

		payload := &common.PayoutDetails{
			Beneficiary: receiver.ID,
			AssetID:     common.KMOITokenAssetID,
			TokenID:     common.DefaultTokenID,
			Amount:      big.NewInt(2),
		}

		ixHash, err := te.transferAsset(sender, payload)
		require.NoError(t, err)

		validateKMOIAssetTransfer(te, sender.ID, payload, ixHash)
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
				LogicID:  logicID,
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
	ixHash, err := te.logicDeploy(
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
		"logicID", logicPayload.LogicID,
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
				ID:       logicPayload.LogicID.AsIdentifier(),
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
