package api

import (
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

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

	stateManager.setAccountMetaInfo(t, accMetaInfo.Address, accMetaInfo)

	testcases := []struct {
		name                string
		args                rpcargs.TesseractArgs
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name:          "failed to fetch acc meta info as address cannot be empty",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyAddress,
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
				Address: accMetaInfo.Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedAccMetaInfo: accMetaInfo,
		},
		{
			name: "fetch acc meta info for non-latest height",
			args: rpcargs.TesseractArgs{
				Address: accMetaInfo.Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &num,
				},
			},
		},
		{
			name: "fetch acc meta info when tesseract hash is provided in option",
			args: rpcargs.TesseractArgs{
				Address: tests.RandomAddress(t),
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

//nolint:dupl
func TestPublicCoreAPI_GetStateHash(t *testing.T) {
	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	height := int64(5)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantsState{
			address: common.State{
				StateHash: stateHash,
				Height:    5,
			},
		},
	})

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	chainManager.SetTesseractHeightEntry(address, ts.Height(address), ts.Hash())

	accMetaInfo := tests.GetRandomAccMetaInfo(t, 3)
	stateManager.setAccountMetaInfo(t, accMetaInfo.Address, accMetaInfo)
	testcases := []struct {
		name              string
		args              rpcargs.TesseractArgs
		expectedStateHash common.Hash
		expectedError     error
	}{
		{
			name:          "failed to fetch state hash as acc meta info not found",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "fetch latest state hash successfully",
			args: rpcargs.TesseractArgs{
				Address: accMetaInfo.Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedStateHash: accMetaInfo.StateHash,
		},
		{
			name: "fetch non-latest state hash successfully",
			args: rpcargs.TesseractArgs{
				Address: ts.AnyAddress(),
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

//nolint:dupl
func TestPublicCoreAPI_GetContextHash(t *testing.T) {
	address := tests.RandomAddress(t)
	contextHash := tests.RandomHash(t)
	height := int64(5)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantsState{
			address: common.State{
				LatestContext: contextHash,
				Height:        5,
			},
		},
	})

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	chainManager.SetTesseractHeightEntry(address, ts.Height(address), ts.Hash())

	accMetaInfo := tests.GetRandomAccMetaInfo(t, 3)
	stateManager.setAccountMetaInfo(t, accMetaInfo.Address, accMetaInfo)
	testcases := []struct {
		name                string
		args                rpcargs.TesseractArgs
		expectedContextHash common.Hash
		expectedError       error
	}{
		{
			name:          "failed to fetch context hash as acc meta info not found",
			args:          rpcargs.TesseractArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "fetch latest context hash successfully",
			args: rpcargs.TesseractArgs{
				Address: accMetaInfo.Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedContextHash: accMetaInfo.ContextHash,
		},
		{
			name: "fetch non-latest context hash successfully",
			args: rpcargs.TesseractArgs{
				Address: ts.AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedContextHash: contextHash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateHash, err := coreAPI.getContextHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedContextHash, stateHash)
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
		address          identifiers.Address
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

	stateManager.setAccountMetaInfo(t, acc.Address, acc)

	address := tests.RandomAddress(t)
	height := uint64(66)
	hash := tests.RandomHash(t)

	chainManager.SetTesseractHeightEntry(address, height, hash)

	testcases := []struct {
		name          string
		address       identifiers.Address
		height        int64
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name:          "invalid address",
			address:       identifiers.NilAddress,
			expectedError: common.ErrInvalidAddress,
		},
		{
			name:          "failed fetch tesseract hash for latest tesseract height",
			address:       tests.RandomAddress(t),
			height:        -1,
			expectedError: common.ErrAccountNotFound,
		},
		{
			name:         "fetch tesseract hash for latest tesseract height",
			address:      acc.Address,
			height:       -1,
			expectedHash: acc.TesseractHash,
		},
		{
			name:         "fetch tesseract hash for given tesseract height",
			address:      address,
			height:       int64(height),
			expectedHash: hash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := coreAPI.getTesseractHashByHeight(test.address, test.height)

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
		Callsite: "hello",
	}
	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(t, err)

	sm := NewMockStateManager(t)
	exec := NewMockExecutionManager(t)
	chainManager := NewMockChainManager(t)

	address := tests.RandomAddress(t)
	ts := tests.CreateTesseract(t,
		&tests.CreateTesseractParams{
			Addresses: []identifiers.Address{address},
		},
	)
	chainManager.setTesseractByHash(t, ts)

	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(2)
	})
	sm.setAccount(ts.AnyAddress(), *acc)

	tsHash := getTesseractHash(t, ts)

	IxWithoutFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: address,
		Nonce:  0,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicDeploy,
				Payload: rawLogicPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  address,
				LockType: common.MutateLock,
			},
		},
	}, nil)
	assert.NoError(t, err)

	IxWithFuelParams, err := constructIxn(sm, &common.IxData{
		Sender:    address,
		Nonce:     0,
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
				Address:  address,
				LockType: common.MutateLock,
			},
		},
	}, nil)
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
					Sender: address,
				},
			},
			expectedErr: ErrEmptyIxOps,
		},
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender:    address,
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
							Address:  address,
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
					Sender:    address,
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
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
					Sender:    tests.RandomAddress(t),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: address,
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxLogicDeploy,
							Payload: (hexutil.Bytes)(rawLogicPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							Address:  address,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
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
					Sender:    address,
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
							Address:  address,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
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

	address := tests.RandomAddress(t)
	ts := tests.CreateTesseract(t,
		&tests.CreateTesseractParams{
			Addresses: []identifiers.Address{address},
		},
	)
	chainManager.setTesseractByHash(t, ts)

	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(2)
	})
	sm.setAccount(ts.AnyAddress(), *acc)

	tsHash := getTesseractHash(t, ts)

	IxWithoutFuelParams, err := constructIxn(sm, &common.IxData{
		Sender: address,
		Nonce:  0,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: rawAssetPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  address,
				LockType: common.MutateLock,
			},
		},
	}, nil)
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
		Sender:    address,
		Nonce:     0,
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
				Address:  address,
				LockType: common.MutateLock,
			},
		},
	}, nil)
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
					Sender: tests.RandomAddress(t),
				},
			},
			expectedErr: ErrEmptyIxOps,
		},
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender:    address,
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
							Address:  address,
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
					Sender:    tests.RandomAddress(t),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
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
					Sender:    tests.RandomAddress(t),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "rpc receipt fetched successfully when fuel price and limit are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Sender: address,
					IxOps: []rpcargs.IxOp{
						{
							Type:    common.IxAssetCreate,
							Payload: (hexutil.Bytes)(rawAssetPayload),
						},
					},
					Participants: []rpcargs.IxParticipant{
						{
							Address:  address,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxHash:   IxWithoutFuelParams.Hash(),
				FuelUsed: hexutil.Uint64(100),
				From:     IxWithoutFuelParams.Sender(),
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
					Sender:    address,
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
							Address:  address,
							LockType: common.MutateLock,
						},
					},
				},
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					address: {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxHash:   IxWithFuelParams.Hash(),
				FuelUsed: hexutil.Uint64(100),
				From:     IxWithFuelParams.Sender(),
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
		address       = tests.RandomAddress(t)
		height        = int64(8)
		invalidHeight = int64(-2)
		ix            = tests.CreateIX(t, nil)
	)

	tsParams := map[int]*tests.CreateTesseractParams{
		0: {
			Addresses: []identifiers.Address{address},
			Participants: common.ParticipantsState{
				address: {
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

	chainManager.SetTesseractHeightEntry(address, ts[0].Height(address), getTesseractHash(t, ts[0]))

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
			name: "should return error if address is nil",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedError: common.ErrInvalidAddress,
		},
		{
			name: "should return error if height is invalid",
			args: rpcargs.TesseractArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "should return error if address is invalid",
			args: rpcargs.TesseractArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "get tesseract by height with interactions",
			args: rpcargs.TesseractArgs{
				Address:          address,
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
				Address:          address,
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
				Address:          ts[1].AnyAddress(),
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
				Address:          ts[1].AnyAddress(),
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
	address := tests.RandomAddress(t)
	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
	})
	height := int64(ts.Height(address))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	chainManager.SetTesseractHeightEntry(address, ts.Height(address), ts.Hash())

	randomHash := tests.RandomHash(t)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomAddress(t))

	stateManager.setBalance(address, assetID, big.NewInt(300))

	testcases := []struct {
		name            string
		args            rpcargs.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:          "address cannot be empty",
			args:          rpcargs.BalArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "should return error if failed to fetch balance",
			args: rpcargs.BalArgs{
				Address: tests.RandomAddress(t),
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
				Address: address,
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
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	context := getContext(t, 2)
	stateManager.setContext(t, ts[0].AnyAddress(), context)

	tsHash := getTesseractsHashes(t, ts)
	randomHash := tests.RandomHash(t)

	testcases := []struct {
		name            string
		args            rpcargs.ContextInfoArgs
		expectedContext *Context
		expectedError   error
	}{
		{
			name: "address cannot be empty",
			args: rpcargs.ContextInfoArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "fetched context info successfully",
			args: rpcargs.ContextInfoArgs{
				Address: ts[0].AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedContext: context,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.ContextInfoArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if context not found",
			args: rpcargs.ContextInfoArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contextResponse, err := coreAPI.ContextInfo(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForContext(t, test.expectedContext, contextResponse.BehaviourNodes, contextResponse.RandomNodes)
		})
	}
}

func TestPublicCoreAPI_GetTDU(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomAddress(t))

	stateManager.setBalance(ts[0].AnyAddress(), assetID, big.NewInt(300))

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedTDU   common.AssetMap
		expectedError error
	}{
		{
			name:          "address cannot be empty",
			args:          rpcargs.QueryArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: rpcargs.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: rpcargs.QueryArgs{
				Address: ts[0].AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTDU: stateManager.getTDU(ts[0].AnyAddress(), common.NilHash),
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

			for _, expectedAmount := range test.expectedTDU {
				amount := tdu[0].Amount
				require.Equal(t, expectedAmount, amount.ToInt())
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCount(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)
	height := int64(ts.Height(ts.AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts.AnyAddress(), ts.Height(ts.AnyAddress()), ts.Hash())
	chainManager.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)

	latestNonce := uint64(5)
	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(5)
	})

	stateManager.setAccount(ts.AnyAddress(), *acc)

	testcases := []struct {
		name          string
		args          *rpcargs.InteractionCountArgs
		expectedNonce uint64
		expectedError error
	}{
		{
			name:          "address cannot be empty",
			args:          &rpcargs.InteractionCountArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "interaction count fetched successfully",
			args: &rpcargs.InteractionCountArgs{
				Address: ts.AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedNonce: latestNonce,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(t),
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
	address := tests.RandomAddress(t)

	ixpool := NewMockIxPool(t)
	ixpool.setNonce(address, 5)

	coreAPI := NewPublicCoreAPI(ixpool, nil, nil, nil, nil, nil)

	testcases := []struct {
		name            string
		args            *rpcargs.InteractionCountArgs
		expectedIxCount uint64
		expectedErr     error
	}{
		{
			name: "valid address with state",
			args: &rpcargs.InteractionCountArgs{
				Address: address,
			},
			expectedIxCount: 5,
		},
		{
			name: "valid address without state",
			args: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(t),
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrAccountNotFound,
		},
		{
			name: "address can not be empty",
			args: &rpcargs.InteractionCountArgs{
				Address: identifiers.NilAddress,
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrEmptyAddress,
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
	height := int64(ts.Height(ts.AnyAddress()))
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)
	chainManager.SetTesseractHeightEntry(ts.AnyAddress(), ts.Height(ts.AnyAddress()), ts.Hash())
	chainManager.setTesseractByHash(t, ts)
	randomHash := tests.RandomHash(t)
	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(5)
		acc.AccType = common.RegularAccount
		acc.AssetDeeds = tests.RandomHash(t)
		acc.ContextHash = tests.RandomHash(t)
		acc.StorageRoot = tests.RandomHash(t)
		acc.AssetRoot = tests.RandomHash(t)
		acc.LogicRoot = tests.RandomHash(t)
		acc.FileRoot = tests.RandomHash(t)
	})
	stateManager.setAccount(ts.AnyAddress(), *acc)
	testcases := []struct {
		name          string
		args          *rpcargs.GetAccountArgs
		expectedAcc   *common.Account
		expectedError error
	}{
		{
			name:          "address can not be empty",
			args:          &rpcargs.GetAccountArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "account state fetched successfully",
			args: &rpcargs.GetAccountArgs{
				Address: ts.AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedAcc: acc,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(t),
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
			require.Equal(t, hexutil.Uint64(test.expectedAcc.Nonce), fetchedAcc.Nonce)
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

func TestPublicCoreAPI_Mandates(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	mandates, rpcMandates := createMandates(t)
	stateManager.setMandates(ts[0].AnyAddress(), mandates)

	testcases := []struct {
		name          string
		args          rpcargs.GetAssetMandateArgs
		expected      []rpcargs.RPCMandate
		expectedError error
	}{
		{
			name:          "address cannot be empty",
			args:          rpcargs.GetAssetMandateArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAssetMandateArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return empty mandates",
			args: rpcargs.GetAssetMandateArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expected: []rpcargs.RPCMandate{},
		},
		{
			name: "mandates retrieved successfully",
			args: rpcargs.GetAssetMandateArgs{
				Address: ts[0].AnyAddress(),
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

func TestPublicCoreAPI_GetLogicIDs(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), ts[0].Hash())
	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	logicIDs := make([]identifiers.LogicID, 0, 3)

	for i := 0; i < 3; i++ {
		logicID := identifiers.NewLogicIDv0(true, false, false, false, uint16(i), tests.RandomAddress(t))

		logicIDs = append(logicIDs, logicID)
	}

	stateManager.setLogicIDs(t, ts[0].AnyAddress(), logicIDs)

	testcases := []struct {
		name             string
		args             rpcargs.GetAccountArgs
		expectedLogicIDs []identifiers.LogicID
		expectedError    error
	}{
		{
			name:          "address can not be empty",
			args:          rpcargs.GetAccountArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if logicIDs not found",
			args: rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("logic IDs not found"),
		},
		{
			name: "fetched logicIDs successfully",
			args: rpcargs.GetAccountArgs{
				Address: ts[0].AnyAddress(),
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

func TestPublicCoreAPI_GetDeeds(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)
	height := int64(ts[0].Height(ts[0].AnyAddress()))

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil, nil, nil)
	c.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), ts[0].Hash())
	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])
	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	assetIDs, assetDescriptors := tests.CreateTestAssets(t, 3)

	deeds, deedEntries := getDeeds(t, assetIDs, assetDescriptors)
	sortDeeds(deedEntries)

	s.setDeeds(t, ts[0].AnyAddress(), deeds)

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedDeeds []rpcargs.RPCDeeds
		expectedError error
	}{
		{
			name:          "address can not be empty",
			args:          rpcargs.QueryArgs{},
			expectedError: common.ErrEmptyAddress,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if deeds not found",
			args: rpcargs.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("deeds not found"),
		},
		{
			name: "should fetch deeds successfully",
			args: rpcargs.QueryArgs{
				Address: ts[0].AnyAddress(),
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

	logicID := tests.GetLogicID(t, ts.AnyAddress())
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomAddress(t))

	poloM, jsonM, yamlM := getManifestInAllEncoding(t, "./../../compute/exlogics/tokenledger/tokenledger.yaml")

	stateManager.setLogicManifest(string(logicID), poloM)
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

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomAddress(t))

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
			expectedError: common.ErrEmptyLogicID,
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

	ixParams := tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), tests.RandomAddress(t))
	ix := tests.CreateIX(t, ixParams)
	tsHash := tests.RandomHash(t)
	participants := tests.CreateParticipantWithTestData(t, 1)
	chainManager.SetInteractionDataByTSHash(tsHash, ix, participants)

	address := tests.RandomAddress(t)
	height := int64(88)
	chainManager.SetTesseractHeightEntry(address, uint64(height), tsHash)

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
			name: "hash not available for given tesseract number and address",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrInvalidAddress,
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
				Address: address,
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
		0: tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), tests.RandomAddress(t)),
		1: tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), tests.RandomAddress(t)),
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
	ixParams := tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), tests.RandomAddress(t))
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
	address := tests.RandomAddress(t)
	stateManager := NewMockStateManager(t)
	chainManager := NewMockChainManager(t)

	ts := tests.CreateTesseract(t, nil)
	tsHash := tests.GetTesseractHash(t, ts)
	chainManager.setTesseractByHash(t, ts)

	assetID, assetInfo := tests.CreateTestAsset(t, address)
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
			expectedError: common.ErrEmptyAssetID,
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
				AssetID: tests.GetRandomAssetID(t, identifiers.NilAddress),
			},
			expectedError: common.ErrEmptyAddress,
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
			require.Equal(t, test.expectedAssetDescriptor.Symbol, fetchedAssetInfo.Symbol)
			require.Equal(t, test.expectedAssetDescriptor.Operator, fetchedAssetInfo.Operator)
			require.Equal(t, *(*hexutil.Big)(test.expectedAssetDescriptor.Supply), fetchedAssetInfo.Supply)
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Dimension), fetchedAssetInfo.Dimension)
			require.Equal(t, hexutil.Uint16(test.expectedAssetDescriptor.Standard), fetchedAssetInfo.Standard)
			require.Equal(t, test.expectedAssetDescriptor.IsLogical, fetchedAssetInfo.IsLogical)
			require.Equal(t, test.expectedAssetDescriptor.IsStateFul, fetchedAssetInfo.IsStateFul)
			require.Equal(t, test.expectedAssetDescriptor.LogicID, fetchedAssetInfo.LogicID)
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

	stateManager.setAccountMetaInfo(t, ts.AnyAddress(), acc)

	testcases := []struct {
		name                string
		args                *rpcargs.GetAccountArgs
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name: "account meta info fetched successfully",
			args: &rpcargs.GetAccountArgs{
				Address: ts.AnyAddress(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAccMetaInfo: acc,
		},
		{
			name: "should return error if failed to fetch account meta info",
			args: &rpcargs.GetAccountArgs{
				Address: identifiers.NilAddress,
			},
			expectedError: common.ErrInvalidAddress,
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
			require.Equal(t, test.expectedAccMetaInfo.Address, fetchedAccMetaInfo.Address)
			require.Equal(t, hexutil.Uint64(test.expectedAccMetaInfo.Height), fetchedAccMetaInfo.Height)
			require.Equal(t, test.expectedAccMetaInfo.TesseractHash, fetchedAccMetaInfo.TesseractHash)
		})
	}
}

func TestPublicCoreAPI_Syncing(t *testing.T) {
	addrs := tests.GetAddresses(t, 5)

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
		PendingAccounts:       addrs,
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
				Address: addrs[0],
			},
			preTestFn: func() {
				syncer.setAccountSyncStatus(addrs[0], accSyncStatus)
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
				Address: tests.RandomAddress(t),
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

	validHeight := int64(ts[0].Height(ts[0].AnyAddress()))

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].AnyAddress(), ts[0].Height(ts[0].AnyAddress()), getTesseractHash(t, ts[0]))

	chainManager.setTesseractByHash(t, ts[1])
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[1])

	testcases := []struct {
		name                string
		options             map[identifiers.Address]*rpcargs.TesseractNumberOrHash
		expectedStateHashes map[identifiers.Address]common.Hash
		expectedError       error
	}{
		{
			name: "state hashes fetched successfully from tesseract hashes",
			options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
				ts[0].AnyAddress(): {
					TesseractNumber: &validHeight,
				},
				ts[1].AnyAddress(): {
					TesseractHash: &tsHash,
				},
			},
			expectedStateHashes: map[identifiers.Address]common.Hash{
				ts[0].AnyAddress(): ts[0].StateHash(ts[0].AnyAddress()),
				ts[1].AnyAddress(): ts[1].StateHash(ts[0].AnyAddress()),
			},
		},
		{
			name: "should return error as tesseract height is invalid",
			options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
				ts[0].AnyAddress(): {
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
				Addr: tests.RandomAddress(t),
			},
			expectedResult: true,
		},
		{
			name: "setup ts by account filter with nil address",
			args: &rpcargs.TesseractByAccountFilterArgs{
				Addr: identifiers.NilAddress,
			},
			expectedError: common.ErrInvalidAddress,
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
		require.Equal(t, testcase.args.Addr, res.tsByAccFilterParams)
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
		Addr: tests.RandomAddress(t),
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
		Addr: tests.RandomAddress(t),
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

	addr := tests.RandomAddress(t)
	args := &jsonrpc.LogQuery{
		StartHeight: 1,
		EndHeight:   10,
		Address:     addr,
		Topics:      nil,
	}

	filterResponse, _ := coreAPI.NewLogFilter(args)
	require.NotEmpty(t, filterResponse.FilterID)

	dummyLogs := []*rpcargs.RPCLog{
		{
			Address: addr,
		},
	}

	filterManager.setLogs(filterResponse.FilterID, dummyLogs)

	query := &rpcargs.FilterQueryArgs{
		StartHeight: NumPointer(t, 1),
		EndHeight:   NumPointer(t, 10),
		Address:     addr,
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
		sender  identifiers.Address
		receipt *common.Receipt
	}{
		{
			name:    "receipt with ixOps",
			sender:  tests.RandomAddress(t),
			receipt: tests.CreateReceiptWithTestData(t),
		},
		{
			name:    "receipt without ixOps",
			sender:  tests.RandomAddress(t),
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
