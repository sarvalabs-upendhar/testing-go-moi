package api

import (
	"encoding/hex"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/state"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Core API Testcases

func TestPublicCoreAPI_GetAccMetaInfo(t *testing.T) {
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, stateManager, nil, nil, nil)

	accMetaInfo := tests.GetRandomAccMetaInfo(t, 3)
	randomHash := tests.RandomHash(t)
	num := int64(1)

	stateManager.setAccountMetaInfo(t, accMetaInfo.ID, accMetaInfo)

	testcases := []struct {
		name                string
		args                rpcargs.TesseractArgs
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name:          "failed to fetch acc meta info as id cannot be empty",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "failed to fetch acc meta info as both options provided",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash:   &randomHash,
					TesseractNumber: &num,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "fetch acc met info if height is latest",
			args: rpcargs.TesseractArgs{
				ID: accMetaInfo.ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedAccMetaInfo: accMetaInfo,
		},
		{
			name: "fetch acc meta info for non-latest height",
			args: rpcargs.TesseractArgs{
				ID: accMetaInfo.ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &num,
				},
			},
		},
		{
			name: "fetch acc meta info when tesseract hash is provided in option",
			args: rpcargs.TesseractArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accMetaInfo, err := coreAPI.getAccMetaInfo(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccMetaInfo, accMetaInfo)
		})
	}
}

func TestPublicCoreAPI_GetStateHash(t *testing.T) {
	id := tests.RandomIdentifier(t)
	stateHash := tests.RandomHash(t)
	height := int64(5)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{id},
		Participants: common.ParticipantsState{
			id: common.State{
				StateHash: stateHash,
				Height:    5,
			},
		},
	})

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	chainManager.SetTesseractHeightEntry(id, ts.Height(id), ts.Hash())

	accMetaInfo := tests.GetRandomAccMetaInfo(t, 3)
	stateManager.setAccountMetaInfo(t, accMetaInfo.ID, accMetaInfo)
	testcases := []struct {
		name              string
		args              rpcargs.TesseractArgs
		expectedStateHash common.Hash
		expectedError     error
	}{
		{
			name:          "failed to fetch state hash as acc meta info not found",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "fetch latest state hash successfully",
			args: rpcargs.TesseractArgs{
				ID: accMetaInfo.ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedStateHash: accMetaInfo.StateHash,
		},
		{
			name: "fetch non-latest state hash successfully",
			args: rpcargs.TesseractArgs{
				ID: ts.AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedStateHash: stateHash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateHash, err := coreAPI.getStateHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedStateHash, stateHash)
		})
	}
}

func TestPublicCoreAPI_GetContextHash(t *testing.T) {
	primaryAccount := tests.RandomIdentifierWithZeroVariant(t)
	subAccount := tests.RandomSubAccountIdentifier(t, 1)

	contextHashes := tests.GetHashes(t, 3)
	height := int64(5)

	ts1 := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{primaryAccount},
		Participants: common.ParticipantsState{
			primaryAccount: common.State{
				LockedContext: contextHashes[1],
				Height:        5,
			},
		},
	})

	ts2 := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{subAccount},
		Participants: common.ParticipantsState{
			subAccount: common.State{
				LockedContext: contextHashes[2],
				Height:        5,
			},
		},
	})

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts1)
	chainManager.SetTesseractHeightEntry(primaryAccount, ts1.Height(primaryAccount), ts1.Hash())

	chainManager.setTesseractByHash(t, ts2)
	chainManager.SetTesseractHeightEntry(subAccount, ts2.Height(subAccount), ts2.Hash())

	stateManager.setAccountMetaInfo(t, primaryAccount, &common.AccountMetaInfo{
		ID:          primaryAccount,
		ContextHash: contextHashes[0],
	})

	stateManager.setAccountMetaInfo(t, subAccount, &common.AccountMetaInfo{
		InheritedAccount: primaryAccount,
	})

	stateManager.setAccount(primaryAccount, common.Account{
		ContextHash: contextHashes[1],
	})

	testcases := []struct {
		name                string
		args                rpcargs.TesseractArgs
		expectedID          identifiers.Identifier
		expectedContextHash common.Hash
		expectedError       error
	}{
		{
			name:          "failed to fetch context hash as acc meta info not found",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "fetch latest context hash for primary account successfully",
			args: rpcargs.TesseractArgs{
				ID: primaryAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedID:          primaryAccount,
			expectedContextHash: contextHashes[0],
		},
		{
			name: "fetch context hash by height for primary account successfully",
			args: rpcargs.TesseractArgs{
				ID: primaryAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedID:          primaryAccount,
			expectedContextHash: contextHashes[1],
		},
		{
			name: "fetch latest context hash for sub account successfully",
			args: rpcargs.TesseractArgs{
				ID: subAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedID:          primaryAccount,
			expectedContextHash: contextHashes[0],
		},
		{
			name: "fetch context hash by height for sub account successfully",
			args: rpcargs.TesseractArgs{
				ID: subAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedID:          primaryAccount,
			expectedContextHash: contextHashes[2],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			id, contextHash, err := coreAPI.getContextHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedID, id)
			require.Equal(t, test.expectedContextHash, contextHash)
		})
	}
}

func TestPublicCoreAPI_GetRPCTesseract(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)
	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	hash := getTesseractHash(t, ts)
	num := int64(1)

	testcases := []struct {
		name          string
		args          rpcargs.TesseractArgs
		expectedError error
	}{
		{
			name: "failed to get rpc tesseract",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "get rpc tesseract by hash",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &hash,
				},
			},
		},
		{
			name: "failed to get tesseract as both options provided",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &num,
					TesseractHash:   &hash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.Tesseract(&test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			rpcargs.CheckForRPCTesseract(t, ts, fetchedTesseract)
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	ix := tests.CreateIX(t, nil)
	tsParams := &tests.CreateTesseractParams{
		Ixns: common.NewInteractionsWithLeaderCheck(false, ix),
		CommitInfo: &common.CommitInfo{
			Operator: tests.RandomKramaID(t, 0),
		},
	}

	ts := tests.CreateTesseract(t, tsParams)

	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	tsHash := tests.GetTesseractHash(t, ts)

	testcases := []struct {
		name             string
		id               identifiers.Identifier
		hash             common.Hash
		withInteractions bool
		withCommitInfo   bool
		expectedTS       *common.Tesseract
	}{
		{
			name:             "fetch tesseract with interactions and without commit info",
			hash:             tsHash,
			withInteractions: true,
			expectedTS:       ts,
		},
		{
			name:             "fetch tesseract with interactions and with commit info",
			hash:             tsHash,
			withInteractions: true,
			withCommitInfo:   true,
			expectedTS:       ts,
		},
		{
			name:             "fetch tesseract without interactions and without commit info",
			hash:             tsHash,
			withInteractions: false,
			expectedTS:       ts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.getTesseractByHash(test.hash, test.withInteractions, test.withCommitInfo)

			require.NoError(t, err)
			tests.ValidateTesseract(t, test.expectedTS, fetchedTesseract, test.withInteractions, test.withCommitInfo)
		})
	}
}

func TestPublicCoreAPI_GetTesseractHashByHeight(t *testing.T) {
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)
	acc := tests.GetRandomAccMetaInfo(t, 8)

	stateManager.setAccountMetaInfo(t, acc.ID, acc)

	id := tests.RandomIdentifier(t)
	height := uint64(66)
	hash := tests.RandomHash(t)

	chainManager.SetTesseractHeightEntry(id, height, hash)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		height        int64
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name:          "invalid id",
			id:            identifiers.Nil,
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name:          "failed fetch tesseract hash for latest tesseract height",
			id:            tests.RandomIdentifier(t),
			height:        -1,
			expectedError: common.ErrAccountNotFound,
		},
		{
			name:         "fetch tesseract hash for latest tesseract height",
			id:           acc.ID,
			height:       -1,
			expectedHash: acc.TesseractHash,
		},
		{
			name:         "fetch tesseract hash for given tesseract height",
			id:           id,
			height:       int64(height),
			expectedHash: hash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := coreAPI.getTesseractHashByHeight(test.id, test.height)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedHash, hash)
		})
	}
}

func TestPublicCoreAPI_FuelEstimate(t *testing.T) {
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	logicPayload := common.LogicPayload{
		LogicID:  identifiers.RandomLogicIDv0(),
		Callsite: "hello",
	}
	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(t, err)

	sm := NewMockStateManager(t)
	exec := NewMockExecutionManager(t)
	chainManager := NewMockChainManager(t)

	id := tests.RandomIdentifier(t)
	ts := tests.CreateTesseract(t,
		&tests.CreateTesseractParams{
			IDs: []identifiers.Identifier{id},
		},
	)
	chainManager.setTesseractByHash(t, ts)

	acc, _ := tests.GetTestAccount(t, nil)
	sm.setAccount(ts.AnyAccountID(), *acc)
	sm.setSequenceID(id, 2)

	tsHash := getTesseractHash(t, ts)

	IxWithoutFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: common.Sender{
			ID: id,
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicDeploy,
				Payload: rawLogicPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	})
	assert.NoError(t, err)

	IxWithFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: common.Sender{
			ID: id,
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 100,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: rawAssetPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	})
	assert.NoError(t, err)

	receipt := &common.Receipt{
		FuelUsed: 100,
	}

	exec.setInteractionCall(IxWithoutFuelParams, receipt)
	exec.setInteractionCall(IxWithFuelParams, receipt)

	testcases := []struct {
		name                 string
		callArgs             *rpcargs.CallArgs
		expectedFuelConsumed *hexutil.Big
		expectedErr          error
	}{
		{
			name: "failed to validate interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
				},
			},
			expectedErr: ErrEmptyIxOps,
		},
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxInvalid,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractNumber: nil,
					},
				},
			},
			expectedErr: common.ErrEmptyOptions,
		},
		{
			name: "should return error as rpc receipt fetch failed",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: tests.RandomIdentifier(t),
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrInvalidSequenceID,
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxLogicDeploy,
							Payload: (hexutil.Bytes)(rawLogicPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			coreAPI := NewPublicCoreAPI(nil, chainManager, sm, exec, nil, nil)

			fuelConsumed, err := coreAPI.FuelEstimate(test.callArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedFuelConsumed, fuelConsumed)
		})
	}
}

func TestPublicCoreAPI_Call(t *testing.T) {
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	logicPayload := common.LogicPayload{
		Callsite: "hello",
	}
	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(t, err)

	sm := NewMockStateManager(t)
	exec := NewMockExecutionManager(t)
	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, sm, exec, nil, nil)

	id := tests.RandomIdentifier(t)
	ts := tests.CreateTesseract(t,
		&tests.CreateTesseractParams{
			IDs: []identifiers.Identifier{id},
		},
	)
	chainManager.setTesseractByHash(t, ts)

	acc, _ := tests.GetTestAccount(t, nil)
	sm.setAccount(ts.AnyAccountID(), *acc)
	sm.setSequenceID(ts.AnyAccountID(), 2)

	tsHash := getTesseractHash(t, ts)

	IxWithoutFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: common.Sender{
			ID: id,
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: rawAssetPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	})
	assert.NoError(t, err)

	receiptWithoutFuelParams := &common.Receipt{
		IxHash:   IxWithoutFuelParams.Hash(),
		FuelUsed: 100,
		IxOps: []*common.IxOpResult{
			{
				IxType: common.IxAssetCreate,
				Data:   rawAssetPayload,
			},
		},
	}

	IxWithFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: common.Sender{
			ID: id,
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 100,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicDeploy,
				Payload: rawLogicPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	})
	assert.NoError(t, err)

	receiptWithFuelParams := &common.Receipt{
		IxHash:   IxWithFuelParams.Hash(),
		FuelUsed: 100,
		IxOps: []*common.IxOpResult{
			{
				IxType: common.IxLogicDeploy,
				Data:   rawLogicPayload,
			},
		},
	}

	exec.setInteractionCall(IxWithoutFuelParams, receiptWithoutFuelParams)
	exec.setInteractionCall(IxWithFuelParams, receiptWithFuelParams)

	testcases := []struct {
		name            string
		callArgs        *rpcargs.CallArgs
		expectedReceipt *rpcargs.RPCReceipt
		expectedErr     error
	}{
		{
			name: "failed to validate interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: tests.RandomIdentifier(t),
					},
				},
			},
			expectedErr: ErrEmptyIxOps,
		},
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxInvalid,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: tests.RandomIdentifier(t),
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractNumber: nil,
					},
				},
			},
			expectedErr: common.ErrEmptyOptions,
		},
		{
			name: "should return error as rpc receipt fetch failed",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: tests.RandomIdentifier(t),
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrInvalidSequenceID,
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxHash:   IxWithoutFuelParams.Hash(),
				FuelUsed: hexutil.Uint64(100),
				From:     IxWithoutFuelParams.SenderID(),
				IxOps: []*rpcargs.RPCIxOpResult{
					{
						TxType: hexutil.Uint64(common.IxAssetCreate),
						Data:   rawAssetPayload,
					},
				},
			},
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: common.Sender{
						ID: id,
					},
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxLogicDeploy,
							Payload: (hexutil.Bytes)(rawLogicPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							ID:       id,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxHash:   IxWithFuelParams.Hash(),
				FuelUsed: hexutil.Uint64(100),
				From:     IxWithFuelParams.SenderID(),
				IxOps: []*rpcargs.RPCIxOpResult{
					{
						TxType: hexutil.Uint64(common.IxLogicDeploy),
						Data:   rawLogicPayload,
					},
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixn, err := coreAPI.Call(test.callArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedReceipt, ixn)
		})
	}
}

func TestPublicCoreAPI_GetTesseract(t *testing.T) {
	var (
		id            = tests.RandomIdentifier(t)
		height        = int64(8)
		invalidHeight = int64(-2)
		ix            = tests.CreateIX(t, nil)
	)

	tsParams := map[int]*tests.CreateTesseractParams{
		0: {
			IDs: []identifiers.Identifier{id},
			Participants: common.ParticipantsState{
				id: {
					Height: uint64(height),
				},
			},
			Ixns: common.NewInteractionsWithLeaderCheck(false, ix),
			CommitInfo: &common.CommitInfo{
				Operator: tests.RandomKramaID(t, 0),
			},
		},
		1: {
			Ixns: common.NewInteractionsWithLeaderCheck(false, ix),
			CommitInfo: &common.CommitInfo{
				Operator: tests.RandomKramaID(t, 0),
			},
		},
	}

	ts := tests.CreateTesseracts(t, 2, tsParams)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(id, ts[0].Height(id), getTesseractHash(t, ts[0]))

	chainManager.setTesseractByHash(t, ts[1])
	chainManager.setTesseractByHash(t, ts[0])

	tsHash1 := tests.GetTesseractHash(t, ts[1])

	testcases := []struct {
		name          string
		args          rpcargs.TesseractArgs
		expectedTS    *common.Tesseract
		expectedError error
	}{
		{
			name: "should return error if options are empty",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "should return error if id is nil",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name: "should return error if height is invalid",
			args: rpcargs.TesseractArgs{
				ID: id,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "should return error if id is invalid",
			args: rpcargs.TesseractArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "get tesseract by height with interactions",
			args: rpcargs.TesseractArgs{
				ID:               id,
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by height without interactions",
			args: rpcargs.TesseractArgs{
				ID:               id,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by hash with interactions and without commit info",
			args: rpcargs.TesseractArgs{
				ID:               ts[1].AnyAccountID(),
				WithInteractions: true,
				WithCommitInfo:   false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash1,
				},
			},
			expectedTS: ts[1],
		},
		{
			name: "get tesseract by hash tesseract without interactions and with commit info",
			args: rpcargs.TesseractArgs{
				ID:               ts[1].AnyAccountID(),
				WithInteractions: false,
				WithCommitInfo:   true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash1,
				},
			},
			expectedTS: ts[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.getTesseract(&test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.ValidateTesseract(t, test.expectedTS, fetchedTesseract, test.args.WithInteractions, test.args.WithCommitInfo)
		})
	}
}

func TestPublicCoreAPI_GetBalance(t *testing.T) {
	id := tests.RandomIdentifier(t)
	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{id},
	})
	height := int64(ts.Height(id))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	chainManager.SetTesseractHeightEntry(id, ts.Height(id), ts.Hash())

	randomHash := tests.RandomHash(t)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomIdentifier(t))

	stateManager.setBalance(id, assetID, common.DefaultTokenID, big.NewInt(300))

	testcases := []struct {
		name            string
		args            rpcargs.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:          "id cannot be empty",
			args:          rpcargs.BalArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if failed to fetch balance",
			args: rpcargs.BalArgs{
				ID:      tests.RandomIdentifier(t),
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched balance successfully",
			args: rpcargs.BalArgs{
				ID:      id,
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedBalance: big.NewInt(300),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := coreAPI.Balance(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedBalance, balance.ToInt())
		})
	}
}

func TestPublicCoreAPI_GetContextInfo(t *testing.T) {
	primaryAccount := tests.RandomIdentifierWithZeroVariant(t)
	subAccount := tests.RandomSubAccountIdentifier(t, 1)

	contextHashes := tests.GetHashes(t, 3)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	stateManager.setAccountMetaInfo(t, primaryAccount, &common.AccountMetaInfo{
		ID:          primaryAccount,
		ContextHash: contextHashes[0],
	})

	stateManager.setAccountMetaInfo(t, subAccount, &common.AccountMetaInfo{
		InheritedAccount: primaryAccount,
	})

	consensusNodes := tests.RandomKramaIDs(t, 2)

	subAccounts := map[identifiers.Identifier]identifiers.Identifier{
		subAccount: tests.RandomIdentifierWithZeroVariant(t),
	}

	stateManager.setMetaContextObject(contextHashes[0], &state.MetaContextObject{
		ConsensusNodes: consensusNodes,
		SubAccounts:    subAccounts,
	})

	testcases := []struct {
		name               string
		args               rpcargs.ContextInfoArgs
		expectedMctxObject *state.MetaContextObject
		expectedError      error
	}{
		{
			name: "id cannot be empty",
			args: rpcargs.ContextInfoArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "fetched context info for primary account successfully",
			args: rpcargs.ContextInfoArgs{
				ID: primaryAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedMctxObject: &state.MetaContextObject{
				ConsensusNodes: consensusNodes,
				SubAccounts:    subAccounts,
			},
		},
		{
			name: "fetched context info for sub account account successfully",
			args: rpcargs.ContextInfoArgs{
				ID: subAccount,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedMctxObject: &state.MetaContextObject{
				ConsensusNodes:   consensusNodes,
				InheritedAccount: primaryAccount,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctxResponse, err := coreAPI.ContextInfo(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, len(test.expectedMctxObject.ConsensusNodes), len(ctxResponse.ConsensusNodes))

			for i := 0; i < len(ctxResponse.ConsensusNodes); i++ {
				require.Equal(t, string(test.expectedMctxObject.ConsensusNodes[i]), ctxResponse.ConsensusNodes[i])
			}

			if !test.args.ID.IsParticipantVariant() {
				require.Equal(t, subAccount, ctxResponse.SubAccounts[0].SubAccounts[0])
				require.Equal(t, test.expectedMctxObject.SubAccounts[subAccount],
					ctxResponse.SubAccounts[0].InheritedAccount)

				return
			}

			require.Equal(t, test.expectedMctxObject.InheritedAccount, ctxResponse.InheritedAccount)
		})
	}
}

func TestPublicCoreAPI_GetTDU(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAccountID(), ts[0].Height(ts[0].AnyAccountID()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomIdentifier(t))

	stateManager.setBalance(ts[0].AnyAccountID(), assetID, common.DefaultTokenID, big.NewInt(300))

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedTDU   common.AssetMap
		expectedError error
	}{
		{
			name:          "id cannot be empty",
			args:          rpcargs.QueryArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: rpcargs.QueryArgs{
				ID: ts[0].AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTDU: stateManager.getTDU(ts[0].AnyAccountID(), common.NilHash),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tdu, err := coreAPI.TDU(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for _, element := range tdu {
				tokenMap, ok := test.expectedTDU[element.AssetID]
				require.True(t, ok)

				val, ok := tokenMap[common.TokenID(element.TokenID.ToUint64())]
				require.True(t, ok)

				require.Equal(t, element.Amount.ToInt(), val)
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCount(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)
	height := int64(ts.Height(ts.AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts.AnyAccountID(), ts.Height(ts.AnyAccountID()), ts.Hash())
	chainManager.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)

	latestNonce := uint64(5)
	acc, _ := tests.GetTestAccount(t, nil)

	stateManager.setAccount(ts.AnyAccountID(), *acc)
	stateManager.setSequenceID(ts.AnyAccountID(), latestNonce)

	testcases := []struct {
		name          string
		args          *rpcargs.InteractionCountArgs
		expectedNonce uint64
		expectedError error
	}{
		{
			name:          "id cannot be empty",
			args:          &rpcargs.InteractionCountArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "interaction count fetched successfully",
			args: &rpcargs.InteractionCountArgs{
				ID: ts.AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedNonce: latestNonce,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.InteractionCountArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedNonce, err := coreAPI.InteractionCount(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedNonce, fetchedNonce.ToUint64())
		})
	}
}

func TestPublicCoreAPI_GetPendingInteractionCount(t *testing.T) {
	id := tests.RandomIdentifier(t)

	ixpool := NewMockIxPool(t)
	ixpool.setNonce(id, 5)

	coreAPI := NewPublicCoreAPI(ixpool, nil, nil, nil, nil, nil)

	testcases := []struct {
		name            string
		args            *rpcargs.InteractionCountArgs
		expectedIxCount uint64
		expectedErr     error
	}{
		{
			name: "valid id with state",
			args: &rpcargs.InteractionCountArgs{
				ID: id,
			},
			expectedIxCount: 5,
		},
		{
			name: "valid id without state",
			args: &rpcargs.InteractionCountArgs{
				ID: tests.RandomIdentifier(t),
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrAccountNotFound,
		},
		{
			name: "id can not be empty",
			args: &rpcargs.InteractionCountArgs{
				ID: identifiers.Nil,
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrEmptyID,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixCount, err := coreAPI.PendingInteractionCount(testcase.args)

			if testcase.expectedErr != nil {
				require.EqualError(t, testcase.expectedErr, err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedIxCount, ixCount.ToUint64())
		})
	}
}

func TestPublicCoreAPI_GetAccountState(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)
	height := int64(ts.Height(ts.AnyAccountID()))
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)
	chainManager.SetTesseractHeightEntry(ts.AnyAccountID(), ts.Height(ts.AnyAccountID()), ts.Hash())
	chainManager.setTesseractByHash(t, ts)
	randomHash := tests.RandomHash(t)
	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.AccType = common.RegularAccount
		acc.AssetDeeds = tests.RandomHash(t)
		acc.ContextHash = tests.RandomHash(t)
		acc.StorageRoot = tests.RandomHash(t)
		acc.AssetRoot = tests.RandomHash(t)
		acc.LogicRoot = tests.RandomHash(t)
		acc.FileRoot = tests.RandomHash(t)
	})
	stateManager.setAccount(ts.AnyAccountID(), *acc)
	testcases := []struct {
		name          string
		args          *rpcargs.GetAccountArgs
		expectedAcc   *common.Account
		expectedError error
	}{
		{
			name:          "id can not be empty",
			args:          &rpcargs.GetAccountArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "account state fetched successfully",
			args: &rpcargs.GetAccountArgs{
				ID: ts.AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedAcc: acc,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.GetAccountArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAcc, err := coreAPI.AccountState(test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAcc.AccType, fetchedAcc.AccType)
			require.Equal(t, test.expectedAcc.AssetDeeds, fetchedAcc.AssetDeeds)
			require.Equal(t, test.expectedAcc.ContextHash, fetchedAcc.ContextHash)
			require.Equal(t, test.expectedAcc.StorageRoot, fetchedAcc.StorageRoot)
			require.Equal(t, test.expectedAcc.AssetRoot, fetchedAcc.AssetRoot)
			require.Equal(t, test.expectedAcc.LogicRoot, fetchedAcc.LogicRoot)
			require.Equal(t, test.expectedAcc.FileRoot, fetchedAcc.FileRoot)
		})
	}
}

//nolint:dupl
func TestPublicCoreAPI_Mandates(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAccountID(), ts[0].Height(ts[0].AnyAccountID()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	mandates, rpcMandates := createMandatesOrLockups(t)
	stateManager.setMandates(ts[0].AnyAccountID(), mandates)

	testcases := []struct {
		name          string
		args          rpcargs.GetAssetMandateOrLockupArgs
		expected      []rpcargs.RPCMandateOrLockup
		expectedError error
	}{
		{
			name:          "id cannot be empty",
			args:          rpcargs.GetAssetMandateOrLockupArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return empty mandates",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expected: []rpcargs.RPCMandateOrLockup{},
		},
		{
			name: "mandates retrieved successfully",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: ts[0].AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expected: rpcMandates,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := coreAPI.Mandates(&test.args)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, test.expected, result)
		})
	}
}

//nolint:dupl
func TestPublicCoreAPI_Lockups(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAccountID(), ts[0].Height(ts[0].AnyAccountID()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	lockups, rpcLockups := createMandatesOrLockups(t)
	stateManager.setLockups(ts[0].AnyAccountID(), lockups)

	testcases := []struct {
		name          string
		args          rpcargs.GetAssetMandateOrLockupArgs
		expected      []rpcargs.RPCMandateOrLockup
		expectedError error
	}{
		{
			name:          "id cannot be empty",
			args:          rpcargs.GetAssetMandateOrLockupArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return empty lockups",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expected: []rpcargs.RPCMandateOrLockup{},
		},
		{
			name: "lockups retrieved successfully",
			args: rpcargs.GetAssetMandateOrLockupArgs{
				ID: ts[0].AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expected: rpcLockups,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := coreAPI.Lockups(&test.args)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			require.ElementsMatch(t, test.expected, result)
		})
	}
}

func TestPublicCoreAPI_GetLogicIDs(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAccountID(), ts[0].Height(ts[0].AnyAccountID()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	logicIDs := make([]identifiers.Identifier, 0, 3)

	for i := 0; i < 3; i++ {
		logicID := identifiers.RandomLogicIDv0()

		logicIDs = append(logicIDs, logicID.AsIdentifier())
	}

	stateManager.setLogicIDs(t, ts[0].AnyAccountID(), logicIDs)

	testcases := []struct {
		name             string
		args             rpcargs.GetAccountArgs
		expectedLogicIDs []identifiers.Identifier
		expectedError    error
	}{
		{
			name:          "id can not be empty",
			args:          rpcargs.GetAccountArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAccountArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if logicIDs not found",
			args: rpcargs.GetAccountArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("logic IDs not found"),
		},
		{
			name: "fetched logicIDs successfully",
			args: rpcargs.GetAccountArgs{
				ID: ts[0].AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedLogicIDs: logicIDs,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicIDs, err := coreAPI.LogicIDs(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLogicIDs, logicIDs)
		})
	}
}

func TestPublicCoreAPI_GetValidators(t *testing.T) {
	stateManager := NewMockStateManager(t)
	validators := tests.CreateTestValidators(t, 2)
	stateManager.setValidators(validators)

	coreAPI := NewPublicCoreAPI(nil, nil, stateManager, nil, nil, nil)

	testcases := []struct {
		name               string
		args               rpcargs.GetValidatorsArgs
		expectedError      error
		expectedValidators []*rpcargs.RPCValidator
	}{
		{
			name: "should return error if krama id is invalid",
			args: rpcargs.GetValidatorsArgs{
				KramaID: "invalid",
			},
			expectedError: common.ErrInvalidKramaID,
		},
		{
			name: "should return error if krama not found",
			args: rpcargs.GetValidatorsArgs{
				KramaID: tests.RandomKramaID(t, 0),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name: "should return the queried validator",
			args: rpcargs.GetValidatorsArgs{
				KramaID: validators[1].KramaID,
			},
			expectedValidators: rpcargs.CreateRPCValidators([]*common.Validator{validators[1]}),
		},
		{
			name:               "should return all the validators",
			expectedValidators: rpcargs.CreateRPCValidators(validators),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcValidators, err := coreAPI.Validators(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, test.expectedValidators, rpcValidators)
		})
	}
}

func TestPublicCoreAPI_GetDeeds(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAccountID()))

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil, nil, nil)
	c.SetTesseractHeightEntry(ts[0].AnyAccountID(), ts[0].Height(ts[0].AnyAccountID()), ts[0].Hash())
	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])
	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	assetIDs, assetDescriptors := tests.CreateTestAssets(t, 3)

	deeds, deedEntries := getDeeds(t, assetIDs, assetDescriptors)
	sortDeeds(deedEntries)

	s.setDeeds(t, ts[0].AnyAccountID(), deeds)

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedDeeds []rpcargs.RPCDeeds
		expectedError error
	}{
		{
			name:          "id can not be empty",
			args:          rpcargs.QueryArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if deeds not found",
			args: rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("deeds not found"),
		},
		{
			name: "should fetch deeds successfully",
			args: rpcargs.QueryArgs{
				ID: ts[0].AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedDeeds: deedEntries,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			deeds, err := coreAPI.Deeds(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			sortDeeds(deeds)
			require.Equal(t, test.expectedDeeds, deeds)
		})
	}
}

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := tests.GetLogicID(t, ts.AnyAccountID())
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomIdentifier(t))

	poloM, jsonM, yamlM := getManifestInAllEncoding(t, "./../../compute/exlogics/tokenledger/tokenledger.yaml")

	stateManager.setLogicManifest(hex.EncodeToString(logicID.Bytes()), poloM)
	chainManager.setTesseractByHash(t, ts)

	testcases := []struct {
		name                  string
		args                  *rpcargs.LogicManifestArgs
		expectedLogicManifest []byte
		expectedError         error
	}{
		{
			name: "returns error if failed to fetch logic manifest",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicIDWithoutState,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched json encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: jsonM,
		},
		{
			name: "fetched polo encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "POLO",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: poloM,
		},
		{
			name: "fetched yaml encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "YAML",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: yamlM,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := coreAPI.LogicManifest(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLogicManifest, manifest.Bytes())
		})
	}
}

func TestPublicCoreAPI_GetLogicStorage(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomIdentifier(t))

	chainManager.setTesseractByHash(t, ts)

	keys := tests.GetHexEntries(t, 1)
	values := tests.GetHexEntries(t, 1)

	stateManager.setStorageEntry(t, logicID, tests.GetStorageMap(keys, values))

	testcases := []struct {
		name          string
		args          *rpcargs.GetLogicStorageArgs
		expectedValue []byte
		expectedError error
	}{
		{
			name:          "logic id can not be empty",
			args:          &rpcargs.GetLogicStorageArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "returns error if failed to fetch logic manifest",
			args: &rpcargs.GetLogicStorageArgs{
				LogicID: logicIDWithoutState,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched logic manifest successfully",
			args: &rpcargs.GetLogicStorageArgs{
				LogicID:    logicID,
				StorageKey: common.Hex2Bytes(keys[0]),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedValue: []byte(values[0]),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := coreAPI.LogicStorage(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, value.Bytes())
		})
	}
}

func TestPublicCoreAPI_GetInteractionByTSHash(t *testing.T) {
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	ixParams := tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t))
	ix := tests.CreateIX(t, ixParams)
	tsHash := tests.RandomHash(t)
	participants := tests.CreateParticipantWithTestData(t, 1)
	chainManager.SetInteractionDataByTSHash(tsHash, ix, participants)

	id := tests.RandomIdentifier(t)
	height := int64(88)
	chainManager.SetTesseractHeightEntry(id, uint64(height), tsHash)

	randomHeight := int64(6)
	negativeHeight := int64(-99)
	randomHash := tests.RandomHash(t)
	ixIndex := uint64(8)

	testcases := []struct {
		name                 string
		args                 rpcargs.InteractionByTesseract
		expectedIX           *common.Interaction
		expectedParticipants common.ParticipantsState
		expectedError        error
	}{
		{
			name: "can not use both tesseract number and tesseract hash",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
					TesseractHash:   &randomHash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "empty options",
			args: rpcargs.InteractionByTesseract{
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "interaction not found",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "hash not available for given tesseract number and id",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name: "ix index is nil",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
			},
			expectedError: common.ErrIXIndex,
		},
		{
			name: "invalid options",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &negativeHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "fetch interaction by tesseract hash successfully",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedIX:           ix,
			expectedParticipants: participants,
		},
		{
			name: "fetch interaction by tesseract number successfully",
			args: rpcargs.InteractionByTesseract{
				ID: id,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedIX:           ix,
			expectedParticipants: participants,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIX, err := coreAPI.InteractionByTesseract(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			rpcargs.CheckForRPCIxn(t, test.expectedIX, tsHash, test.expectedParticipants, rpcIX)
			require.Equal(t, *test.args.IxIndex, rpcIX.IxIndex)
		})
	}
}

func TestPublicCoreAPI_GetInteractionByHash(t *testing.T) {
	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t)),
		1: tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t)),
	}
	ixns := tests.CreateIxns(t, 2, ixParams)
	participants := tests.CreateParticipantWithTestData(t, 1)
	ixIndex := 8

	ixpool := NewMockIxPool(t)
	ixpool.SetPendingIx(ixns[1])

	testcases := []struct {
		name           string
		args           rpcargs.InteractionByHashArgs
		setHook        func(c *MockChainManager)
		expectedIXData ixData
		expectedError  error
	}{
		{
			name: "invalid tesseract hash",
			args: rpcargs.InteractionByHashArgs{
				Hash: common.NilHash,
			},
			expectedError: common.ErrInvalidHash,
		},
		{
			name: "fetch pending interaction successfully",
			args: rpcargs.InteractionByHashArgs{
				Hash: ixns[1].Hash(),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return common.ErrTSHashNotFound
				}
			},
			expectedIXData: ixData{
				ix:           ixns[1],
				tsHash:       tests.RandomHash(t),
				participants: participants,
				ixIndex:      0,
			},
		},
		{
			name: "fetch finalized interaction successfully",
			args: rpcargs.InteractionByHashArgs{
				Hash: ixns[0].Hash(),
			},
			expectedIXData: ixData{
				ix:           ixns[0],
				tsHash:       tests.RandomHash(t),
				participants: participants,
				ixIndex:      ixIndex,
			},
		},
		{
			name: "should return error if failed to fetch finalized interaction",
			args: rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "should return error if failed to fetch pending interaction",
			args: rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return common.ErrTSHashNotFound
				}
			},
			expectedError: common.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			chainManager := NewMockChainManager(t)
			if test.setHook != nil {
				test.setHook(chainManager)
			}

			chainManager.SetInteractionDataByIxHash(ixns[0], test.expectedIXData.tsHash, participants, ixIndex)

			coreAPI := NewPublicCoreAPI(ixpool, chainManager, nil, nil, nil, nil)

			rpcIX, err := coreAPI.InteractionByHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if len(test.expectedIXData.participants) == 0 {
				rpcargs.CheckForRPCIxn(t,
					test.expectedIXData.ix,
					test.expectedIXData.tsHash,
					test.expectedIXData.participants,
					rpcIX,
				)
			}

			require.Equal(t, uint64(test.expectedIXData.ixIndex), rpcIX.IxIndex.ToUint64())
		})
	}
}

func TestPublicCoreAPI_GetInteractionReceipt(t *testing.T) {
	chainManager := NewMockChainManager(t)

	ixHashWithoutParticipants := tests.RandomHash(t)
	ixParams := tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t))
	ix := tests.CreateIX(t, ixParams)
	participants := tests.CreateParticipantWithTestData(t, 1)
	ixIndex := 8
	tsHash := tests.RandomHash(t)

	receipt := tests.CreateReceiptWithTestData(t)
	receipt.IxHash = ix.Hash()

	chainManager.SetInteractionDataByIxHash(ix, tsHash, participants, ixIndex)
	chainManager.setReceiptByIXHash(receipt.IxHash, receipt)
	chainManager.setReceiptByIXHash(ixHashWithoutParticipants, receipt)

	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	testcases := []struct {
		name            string
		args            rpcargs.ReceiptArgs
		expectedReceipt *common.Receipt
		ix              *common.Interaction
		participants    common.ParticipantsState
		ixIndex         int
		expectedError   error
	}{
		{
			name: "nil hash",
			args: rpcargs.ReceiptArgs{
				Hash: common.NilHash,
			},
			expectedError: common.ErrInvalidHash,
		},
		{
			name: "failed to fetch receipt",
			args: rpcargs.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrReceiptNotFound,
		},
		{
			name: "failed to fetch interaction",
			args: rpcargs.ReceiptArgs{
				Hash: ixHashWithoutParticipants,
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "fetched receipt successfully",
			args: rpcargs.ReceiptArgs{
				Hash: ix.Hash(),
			},
			expectedReceipt: receipt,
			ix:              ix,
			participants:    participants,
			ixIndex:         ixIndex,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := coreAPI.InteractionReceipt(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			rpcargs.CheckForRPCReceipt(t, tsHash, test.participants, test.ix, test.expectedReceipt, receipt, test.ixIndex)
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	id := tests.RandomIdentifier(t)
	stateManager := NewMockStateManager(t)
	chainManager := NewMockChainManager(t)

	ts := tests.CreateTesseract(t, nil)
	tsHash := tests.GetTesseractHash(t, ts)
	chainManager.setTesseractByHash(t, ts)

	assetID, assetInfo := tests.CreateTestAsset(t, id)
	stateManager.addAsset(assetID, assetInfo)

	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	testcases := []struct {
		name                    string
		args                    *rpcargs.GetAssetInfoArgs
		expectedAssetDescriptor *common.AssetDescriptor
		expectedError           error
	}{
		{
			name:          "empty asset id",
			args:          &rpcargs.GetAssetInfoArgs{},
			expectedError: common.ErrEmptyID,
		},
		{
			name: "Valid asset id",
			args: &rpcargs.GetAssetInfoArgs{
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAssetDescriptor: assetInfo,
		},
		{
			name: "failed to fetch state hash",
			args: &rpcargs.GetAssetInfoArgs{
				AssetID: tests.GetRandomAssetID(t, identifiers.Nil),
			},
			expectedError: common.ErrEmptyOptions,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAssetInfo, err := coreAPI.AssetInfoByAssetID(test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAssetDescriptor.AssetID.Standard(), fetchedAssetInfo.AssetID.Standard())
			require.Equal(t, test.expectedAssetDescriptor.Symbol, fetchedAssetInfo.Symbol)
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Dimension), fetchedAssetInfo.Dimension)
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Decimals), fetchedAssetInfo.Decimals)
			require.Equal(t, test.expectedAssetDescriptor.Creator, fetchedAssetInfo.Creator)
			require.Equal(t, test.expectedAssetDescriptor.Manager, fetchedAssetInfo.Manager)
			require.Equal(t, (*hexutil.Big)(test.expectedAssetDescriptor.MaxSupply), fetchedAssetInfo.MaxSupply)
			require.Equal(t, (*hexutil.Big)(test.expectedAssetDescriptor.CirculatingSupply), fetchedAssetInfo.CirculatingSupply)
			require.Equal(t, test.expectedAssetDescriptor.EnableEvents, fetchedAssetInfo.EnableEvents)
			require.Equal(t, len(test.expectedAssetDescriptor.StaticMetaData), len(fetchedAssetInfo.StaticMetadata))
			require.Equal(t, len(test.expectedAssetDescriptor.DynamicMetaData), len(fetchedAssetInfo.DynamicMetadata))

			for k, v := range test.expectedAssetDescriptor.StaticMetaData {
				require.Equal(t, v, fetchedAssetInfo.StaticMetadata[k].Bytes())
			}

			for k, v := range test.expectedAssetDescriptor.DynamicMetaData {
				require.Equal(t, v, fetchedAssetInfo.DynamicMetadata[k].Bytes())
			}
		})
	}
}

func TestPublicCoreAPI_GetAccountMetaInfo(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	tsHash := tests.GetTesseractHash(t, ts)
	acc := tests.GetRandomAccMetaInfo(t, 1)

	stateManager.setAccountMetaInfo(t, ts.AnyAccountID(), acc)

	testcases := []struct {
		name                string
		args                *rpcargs.GetAccountArgs
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name: "account meta info fetched successfully",
			args: &rpcargs.GetAccountArgs{
				ID: ts.AnyAccountID(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAccMetaInfo: acc,
		},
		{
			name: "should return error if failed to fetch account meta info",
			args: &rpcargs.GetAccountArgs{
				ID: identifiers.Nil,
			},
			expectedError: common.ErrInvalidIdentifier,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAccMetaInfo, err := coreAPI.AccountMetaInfo(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccMetaInfo.Type, fetchedAccMetaInfo.Type)
			require.Equal(t, test.expectedAccMetaInfo.ID, fetchedAccMetaInfo.ID)
			require.Equal(t, hexutil.Uint64(test.expectedAccMetaInfo.Height), fetchedAccMetaInfo.Height)
			require.Equal(t, test.expectedAccMetaInfo.TesseractHash, fetchedAccMetaInfo.TesseractHash)
		})
	}
}

func TestPublicCoreAPI_Syncing(t *testing.T) {
	ids := tests.GetIdentifiers(t, 5)

	syncer := NewMockSyncer(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, syncer, nil)

	accSyncStatus := &rpcargs.AccSyncStatus{
		CurrentHeight:     2,
		ExpectedHeight:    4,
		IsPrimarySyncDone: true,
	}

	nodeSyncStatus := &rpcargs.NodeSyncStatus{
		TotalPendingAccounts:  2,
		PrincipalSyncDoneTime: hexutil.Uint64(time.Now().UnixNano()),
		IsPrincipalSyncDone:   true,
		IsInitialSyncDone:     false,
	}

	pendingNodeSyncStatus := &rpcargs.NodeSyncStatus{
		TotalPendingAccounts:  5,
		PendingAccounts:       ids,
		PrincipalSyncDoneTime: hexutil.Uint64(time.Now().UnixNano()),
		IsPrincipalSyncDone:   true,
		IsInitialSyncDone:     false,
	}

	testcases := []struct {
		name                       string
		args                       *rpcargs.SyncStatusRequest
		preTestFn                  func()
		expectedSyncStatusResponse *rpcargs.SyncStatusResponse
		expectedError              error
	}{
		{
			name: "account sync status fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				ID: ids[0],
			},
			preTestFn: func() {
				syncer.setAccountSyncStatus(ids[0], accSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				AccSyncResp: accSyncStatus,
			},
		},
		{
			name: "node sync status with pending accounts fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				PendingAccounts: true,
			},
			preTestFn: func() {
				syncer.setPendingNodeSyncStatus(pendingNodeSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				NodeSyncResp: pendingNodeSyncStatus,
			},
		},
		{
			name: "node sync status without pending accounts fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				PendingAccounts: false,
			},
			preTestFn: func() {
				syncer.setNodeSyncStatus(nodeSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				NodeSyncResp: nodeSyncStatus,
			},
		},
		{
			name: "should return error if failed to fetch account sync status",
			args: &rpcargs.SyncStatusRequest{
				ID: tests.RandomIdentifier(t),
			},
			expectedError: common.ErrAccSyncStatusNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn()
			}

			syncStatus, err := coreAPI.Syncing(test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSyncStatusResponse, syncStatus)
		})
	}
}

func TestPublicCoreAPI_normalizeOptions(t *testing.T) {
	invalidHeight := int64(-2)

	ts := tests.CreateTesseracts(t, 2, nil)

	validHeight := int64(ts[0].Height(ts[0].AnyAccountID()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(
		ts[0].AnyAccountID(),
		ts[0].Height(ts[0].AnyAccountID()),
		getTesseractHash(t, ts[0]),
	)

	chainManager.setTesseractByHash(t, ts[1])
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[1])

	testcases := []struct {
		name                string
		options             map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash
		expectedStateHashes map[identifiers.Identifier]common.Hash
		expectedError       error
	}{
		{
			name: "state hashes fetched successfully from tesseract hashes",
			options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
				ts[0].AnyAccountID(): {
					TesseractNumber: &validHeight,
				},
				ts[1].AnyAccountID(): {
					TesseractHash: &tsHash,
				},
			},
			expectedStateHashes: map[identifiers.Identifier]common.Hash{
				ts[0].AnyAccountID(): ts[0].StateHash(ts[0].AnyAccountID()),
				ts[1].AnyAccountID(): ts[1].StateHash(ts[0].AnyAccountID()),
			},
		},
		{
			name: "should return error as tesseract height is invalid",
			options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
				ts[0].AnyAccountID(): {
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateHashes, err := coreAPI.normalizeOptions(test.options)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedStateHashes, stateHashes)
		})
	}
}

func TestPublicCoreAPI_NewTesseractFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	filterResponse, _ := coreAPI.NewTesseractFilter()
	require.NotEmpty(t, filterResponse.FilterID)

	res := filterManager.getTSFilter(filterResponse.FilterID)
	require.True(t, res)
}

func TestPublicCoreAPI_NewTesseractsByAccountFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	testcases := []struct {
		name           string
		args           *rpcargs.TesseractByAccountFilterArgs
		expectedResult bool
		expectedError  error
	}{
		{
			name: "setup ts by account filter successfully",
			args: &rpcargs.TesseractByAccountFilterArgs{
				ID: tests.RandomIdentifier(t),
			},
			expectedResult: true,
		},
		{
			name: "setup ts by account filter with nil id",
			args: &rpcargs.TesseractByAccountFilterArgs{
				ID: identifiers.Nil,
			},
			expectedError: common.ErrInvalidIdentifier,
		},
	}

	for _, testcase := range testcases {
		filterResponse, err := coreAPI.NewTesseractsByAccountFilter(testcase.args)
		if !testcase.expectedResult {
			require.Equal(t, testcase.expectedError, err)

			return
		}

		require.NotEmpty(t, filterResponse.FilterID)
		res, ok := filterManager.getTSByAccFilter(filterResponse.FilterID)
		require.True(t, ok)
		require.Equal(t, testcase.args.ID, res.tsByAccFilterParams)
	}
}

func TestPublicCoreAPI_NewLogFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	args := &jsonrpc.LogQuery{}

	hash, err := common.PoloHash(args)
	if err != nil {
		log.Fatal(err)
	}

	filterResponse, _ := coreAPI.NewLogFilter(args)
	require.NotEmpty(t, filterResponse.FilterID)

	res, ok := filterManager.getLogFilter(filterResponse.FilterID)
	require.True(t, ok)
	require.Equal(t, hash, res)
}

func TestPublicCoreAPI_PendingIxnsFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	filterResponse, _ := coreAPI.PendingIxnsFilter()
	require.NotEmpty(t, filterResponse.FilterID)

	res := filterManager.getIxnsFilter(filterResponse.FilterID)
	require.True(t, res)
}

func TestPublicCoreAPI_RemoveFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	reqArgs := &rpcargs.TesseractByAccountFilterArgs{
		ID: tests.RandomIdentifier(t),
	}

	// add new tesseract by account filter
	filterResponse, err := coreAPI.NewTesseractsByAccountFilter(reqArgs)
	require.NoError(t, err)

	respArgs := &rpcargs.FilterArgs{
		FilterID: filterResponse.FilterID,
	}

	// remove new tesseract by account filter
	response, _ := coreAPI.RemoveFilter(respArgs)
	require.True(t, response.Status)

	// removing already removed filter
	response, _ = coreAPI.RemoveFilter(respArgs)
	require.False(t, response.Status)
}

func TestPublicCoreAPI_GetFilterChanges(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	reqArgs := &rpcargs.TesseractByAccountFilterArgs{
		ID: tests.RandomIdentifier(t),
	}

	// add new tesseract by account filter
	filterResponse, err := coreAPI.NewTesseractsByAccountFilter(reqArgs)
	require.NoError(t, err)

	ts := tests.CreateTesseract(t, nil)
	rpcTS, err := rpcargs.CreateRPCTesseract(ts)
	require.NoError(t, err)

	filterManager.setTSByAccFilterChanges(filterResponse.FilterID, []*rpcargs.RPCTesseract{rpcTS})

	args := &rpcargs.FilterArgs{
		FilterID: filterResponse.FilterID,
	}

	// get filter changes for new tesseract by account filter
	respArgs, err := coreAPI.GetFilterChanges(args)
	require.NoError(t, err)

	resp, ok := respArgs.([]*rpcargs.RPCTesseract)
	require.True(t, ok)
	require.Equal(t, []*rpcargs.RPCTesseract{rpcTS}, resp)
}

func TestPublicCoreAPI_GetLogs(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	id := tests.RandomIdentifier(t)
	args := &jsonrpc.LogQuery{
		StartHeight: 1,
		EndHeight:   10,
		ID:          id,
		Topics:      nil,
	}

	filterResponse, _ := coreAPI.NewLogFilter(args)
	require.NotEmpty(t, filterResponse.FilterID)

	dummyLogs := []*rpcargs.RPCLog{
		{
			ID: id,
		},
	}

	filterManager.setLogs(filterResponse.FilterID, dummyLogs)

	query := &rpcargs.FilterQueryArgs{
		StartHeight: NumPointer(t, 1),
		EndHeight:   NumPointer(t, 10),
		ID:          id,
		Topics:      nil,
	}
	logs, err := coreAPI.GetLogs(query)
	require.NoError(t, err)
	require.Equal(t, dummyLogs, logs)
}

func TestPublicCoreAPI_createCallReceipt(t *testing.T) {
	receiptWithNilData := tests.CreateReceiptWithTestData(t)
	receiptWithNilData.IxOps = nil

	testcases := []struct {
		name    string
		sender  identifiers.Identifier
		receipt *common.Receipt
	}{
		{
			name:    "receipt with ixOps",
			sender:  tests.RandomIdentifier(t),
			receipt: tests.CreateReceiptWithTestData(t),
		},
		{
			name:    "receipt without ixOps",
			sender:  tests.RandomIdentifier(t),
			receipt: receiptWithNilData,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipt := test.receipt

			callReceipt := createCallReceipt(test.sender, test.receipt)

			require.Equal(t, expectedReceipt.Status, callReceipt.Status)
			require.Equal(t, expectedReceipt.IxHash, callReceipt.IxHash)
			require.Equal(t, expectedReceipt.FuelUsed, callReceipt.FuelUsed.ToUint64())
			require.Len(t, callReceipt.IxOps, len(expectedReceipt.IxOps))

			for idx, expectedTx := range expectedReceipt.IxOps {
				require.Equal(t, expectedTx.Status, callReceipt.IxOps[idx].Status)
				require.Equal(t, expectedTx.IxType, common.IxOpType(callReceipt.IxOps[idx].TxType.ToUint64()))
				require.Equal(t, expectedTx.Data, callReceipt.IxOps[idx].Data)
			}
		})
	}
}

func getManifestInAllEncoding(t *testing.T, filepath string) (poloEncoded, jsonEncoded, yamlEncoded []byte) {
	t.Helper()

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngine(pisa.NewEngine())

	// Read manifest at file path
	manifest, err := engineio.NewManifestFromFile(filepath)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	poloEncoded, err = manifest.Encode(common.POLO)
	require.NoError(t, err)

	// Encode the Manifest into JSON data
	jsonEncoded, err = manifest.Encode(common.JSON)
	require.NoError(t, err)

	// Encode the Manifest into YAML data
	yamlEncoded, err = manifest.Encode(common.YAML)
	require.NoError(t, err)

	return
}
