package moiclient

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/bgclient"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/websocket"
	gtypes "github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
)

// Guidelines for creating MOIClient tests:
// This file should encompass MOIClient tests that do not require interactions to be finalized.
// If it's possible to structure a test in a manner that allows us to modify the genesis file to
// provide data for the MOIClient API, then make the necessary modifications to the genesis file.

type TestSingleNode struct {
	suite.Suite
	bgConfig       *bgclient.ClusterConfig
	moiClient      *Client
	bgClient       bgclient.Client
	genesis        *common.GenesisFile
	accounts       []tests.AccountWithMnemonic
	moiAssetInfo   *common.AssetCreationArgs
	logger         hclog.Logger
	instances      []common.Instance
	suiteSetupDone bool
}

func (tn *TestSingleNode) runCriticallyNecessaryTearDown() {
	err := tn.bgClient.DestroyNetwork(context.Background(), true)
	tn.Suite.NoError(err)
}

func (tn *TestSingleNode) initLogger() {
	tn.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString("ERROR"),
	})
}

func (tn *TestSingleNode) SetupSuite() {
	defer func() {
		// make sure to delete directories in case of setup suite failure
		if !tn.suiteSetupDone {
			tn.logger.Error("Setup suite failed")
			tn.runCriticallyNecessaryTearDown()
		}
	}()

	tn.initLogger()

	d := bgclient.DefaultClusterConfig()
	d.WithLogs = false
	d.WithStdout = false
	d.LogLevel = "TRACE"
	d.BootNodePort = 21000
	d.Libp2pPort = 22000
	d.JSONRPCPort = 23000
	d.ValidatorCount = 1
	// genesis asset count is 1 as we need to provide data for registry api
	d.GenesisAssetCount = 1

	tn.bgConfig = d
	tn.bgClient = bgclient.NewClient(&bgclient.Config{
		ClusterConfig: d,
		Network:       bgclient.LOCAL,
	})

	_, err := tn.bgClient.StartNetwork(context.Background())
	tn.Suite.NoError(err)

	// wait for node to start all modules
	time.Sleep(1 * time.Second)

	tn.moiClient, err = NewClient(fmt.Sprintf("http://localhost:%d", d.JSONRPCPort))
	tn.Suite.NoError(err)

	tn.genesis, err = common.ReadGenesisFile(filepath.Join(d.TempDir, "genesis.json"))
	tn.Suite.NoError(err)

	tn.instances, err = common.ReadInstancesFile(filepath.Join(d.TempDir, "instances.json"))
	tn.Suite.NoError(err)

	tn.accounts, err = tn.bgClient.Accounts(context.Background())
	tn.Suite.NoError(err)

	tn.moiAssetInfo = tn.genesis.AssetAccounts[0].AssetInfo

	// ixpool is filled with some interactions to provide data for ixpool related api's
	// fill ixpool in the following way
	// a0 - promoted -ix0,ix1,ix2
	//      enqueued - ix4
	// a1 - enqueued - ix0

	addr0 := tn.accounts[0].Addr
	addr1 := tn.accounts[1].Addr

	for i := 0; i < 3; i++ {
		// promoted queue
		createAssetWithNonce(tn.T(), tn.moiClient, addr0, tn.accounts[0].Mnemonic, uint64(i))
	}

	// enqueued queue
	createAssetWithNonce(tn.T(), tn.moiClient, addr0, tn.accounts[0].Mnemonic, uint64(4))
	// different account
	createAssetWithNonce(tn.T(), tn.moiClient, addr1, tn.accounts[1].Mnemonic, uint64(4))

	tn.suiteSetupDone = true
}

func (tn *TestSingleNode) TearDownSuite() {
	tn.logger.Debug("tear down suite called")

	tn.runCriticallyNecessaryTearDown()
}

func TestMoiClientApiSingleNode(t *testing.T) {
	suite.Run(t, new(TestSingleNode))
}

// TestTesseract includes an HTTP call to ensure the proper functioning of the moiclient framework.
func (tn *TestSingleNode) TestTesseract() {
	testcases := []struct {
		name          string
		tesseractArgs *rpcargs.TesseractArgs
		expectedError error
	}{
		{
			name: "should fetch genesis tesseract",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          common.SargaAddress,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &genesisHeight,
				},
			},
		},
		{
			name: "should return error as invalid address",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          tests.RandomAddress(tn.T()),
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			ts, err := tn.moiClient.Tesseract(context.Background(), test.tesseractArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			httpTS := httpTesseract(tn.T(), tn.moiClient.URL(), test.tesseractArgs)
			require.Equal(tn.T(), httpTS, ts)

			require.NoError(tn.T(), err)
			require.True(tn.T(), ts.HasParticipant(test.tesseractArgs.Address))
			require.Equal(tn.T(), 0, len(ts.Ixns))
		})
	}
}

func (tn *TestSingleNode) TestDBGet() {
	// key and value belongs to genesis tesseract account meta info
	key, _ := storage.BucketKeyAndID(common.SargaAddress)
	ctx := context.Background()
	testcases := []struct {
		name          string
		debugArgs     *rpcargs.DebugArgs
		expectedError error
	}{
		{
			name: "fetch value for existing key in db",
			debugArgs: &rpcargs.DebugArgs{
				Key: common.BytesToHex(key),
			},
		},
		{
			name: "fetch value for non-existing key in db",
			debugArgs: &rpcargs.DebugArgs{
				Key: "822c978f24933d17d4a6d8e40459c30ba9ba12d4d958ab2dc80d1720e39fa73ae5",
			},
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			value, err := tn.moiClient.DBGet(ctx, test.debugArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)

			accMetaInfo := new(common.AccountMetaInfo)
			require.NoError(tn.T(), accMetaInfo.FromBytes(common.Hex2Bytes(value)))
			require.Equal(tn.T(), common.SargaAddress, accMetaInfo.Address)
			require.Equal(tn.T(), common.SargaAccount, accMetaInfo.Type)
		})
	}
}

func (tn *TestSingleNode) TestSyncing() {
	ctx := context.Background()
	testcases := []struct {
		name          string
		StatusArgs    *rpcargs.SyncStatusRequest
		expectedError error
	}{
		{
			name: "should return error if failed to fetch account sync status",
			StatusArgs: &rpcargs.SyncStatusRequest{
				Address: tests.RandomAddress(tn.T()),
			},
			expectedError: common.ErrAccSyncStatusNotFound,
		},
		{
			name:       "node sync status fetched successfully",
			StatusArgs: &rpcargs.SyncStatusRequest{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			_, err := tn.moiClient.Syncing(ctx, test.StatusArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
		})
	}
}

func (tn *TestSingleNode) TestGetAssetInfoByAssetID() {
	a := tn.genesis.AssetAccounts[0].AssetInfo

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		expectedError error
	}{
		{
			name: "fetch asset info for existing assetID",
			assetID: identifiers.NewAssetIDv0(
				a.IsLogical,
				a.IsStateful,
				a.Dimension.ToInt(),
				a.Standard.ToInt(),
				common.CreateAddressFromString(a.Symbol),
			),
		},
		{
			name:          "fetch asset info for non-existing assetID",
			assetID:       tests.GetRandomAssetID(tn.T(), tests.RandomAddress(tn.T())),
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			args := &rpcargs.GetAssetInfoArgs{
				AssetID: test.assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			}

			assetDescriptor, err := tn.moiClient.AssetInfoByAssetID(context.Background(), args)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), assetDescriptor.Symbol, a.Symbol)
			require.Equal(tn.T(), assetDescriptor.Operator, a.Operator)
			require.Equal(tn.T(), assetDescriptor.Supply.ToInt().Uint64(),
				uint64(tn.bgConfig.GenesisAccountCount*tn.bgConfig.PremineAmount))
			require.Equal(tn.T(), assetDescriptor.Dimension, a.Dimension)
			require.Equal(tn.T(), assetDescriptor.Standard, a.Standard)
			require.Equal(tn.T(), assetDescriptor.IsLogical, a.IsLogical)
			require.Equal(tn.T(), assetDescriptor.IsStateFul, a.IsStateful)
		})
	}
}

func (tn *TestSingleNode) TestGetBalance() {
	a := tn.genesis.AssetAccounts[0].AssetInfo

	testcases := []struct {
		name            string
		balanceArgs     *rpcargs.BalArgs
		expectedBalance *hexutil.Big
		expectedError   error
	}{
		{
			name: "fetch moi token balance at latest height",
			balanceArgs: &rpcargs.BalArgs{
				Address: a.Allocations[0].Address,
				AssetID: identifiers.NewAssetIDv0(
					a.IsLogical,
					a.IsStateful,
					a.Dimension.ToInt(),
					a.Standard.ToInt(),
					common.CreateAddressFromString(a.Symbol),
				),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedBalance: a.Allocations[0].Amount,
		},
		{
			name: "get balance returns error for unknown asset ID",
			balanceArgs: &rpcargs.BalArgs{
				Address: a.Allocations[0].Address,
				AssetID: tests.GetRandomAssetID(tn.T(), tests.RandomAddress(tn.T())),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("asset not found"),
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			balance, err := tn.moiClient.Balance(context.Background(), test.balanceArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), test.expectedBalance, balance)
		})
	}
}

func (tn *TestSingleNode) TestTDU() {
	a := tn.genesis.AssetAccounts[0].AssetInfo

	testcases := []struct {
		name          string
		queryArgs     *rpcargs.QueryArgs
		expectedError error
	}{
		{
			name: "fetch TDU for existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: a.Allocations[0].Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch TDU for non-existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			tdu, err := tn.moiClient.TDU(context.Background(), test.queryArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 2, len(tdu))
		})
	}
}

func (tn *TestSingleNode) TestRegistry() {
	a := tn.genesis.AssetAccounts[1].AssetInfo

	assetID := identifiers.NewAssetIDv0(
		a.IsLogical,
		a.IsStateful,
		a.Dimension.ToInt(),
		a.Standard.ToInt(),
		common.CreateAddressFromString(a.Symbol),
	)

	testcases := []struct {
		name          string
		queryArgs     *rpcargs.QueryArgs
		expectedError error
	}{
		{
			name: "fetch registry for existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: a.Allocations[0].Address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch registry for non-existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			registry, err := tn.moiClient.Registry(context.Background(), test.queryArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 1, len(registry))
			require.Equal(tn.T(), assetID.String(), registry[0].AssetID)
			require.Equal(tn.T(), a.Operator, registry[0].AssetInfo.Operator)
		})
	}
}

func (tn *TestSingleNode) TestGetContextInfo() {
	testcases := []struct {
		name            string
		contextInfoArgs *rpcargs.ContextInfoArgs
		expectedError   error
	}{
		{
			name: "fetch context info for existing address",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				Address: common.SargaAddress,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch context info for non-existing address",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			contextInfo, err := tn.moiClient.ContextInfo(context.Background(), test.contextInfoArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)

			for i := 0; i < len(tn.genesis.SargaAccount.BehaviouralContext); i++ {
				require.Equal(tn.T(), string(tn.genesis.SargaAccount.BehaviouralContext[i]),
					contextInfo.BehaviourNodes[i])
			}

			for i := 0; i < len(tn.genesis.SargaAccount.RandomContext); i++ {
				require.Equal(tn.T(), string(tn.genesis.SargaAccount.RandomContext[i]), contextInfo.RandomNodes[i])
			}
		})
	}
}

func (tn *TestSingleNode) TestInteractionCount() {
	testcases := []struct {
		name                 string
		interactionCountArgs *rpcargs.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch interaction count for existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: common.SargaAddress,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch interaction count for non-existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			interactionCount, err := tn.moiClient.InteractionCount(context.Background(), test.interactionCountArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), uint64(0), interactionCount.ToUint64())
		})
	}
}

func (tn *TestSingleNode) TestPendingInteractionCount() {
	testcases := []struct {
		name                 string
		interactionCountArgs *rpcargs.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch pending interaction count for non-existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(tn.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetch pending interaction count for existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: common.SargaAddress,
			},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			pendingInteractionCount, err := tn.moiClient.PendingInteractionCount(context.Background(), test.interactionCountArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), uint64(0), pendingInteractionCount.ToUint64())
		})
	}
}

func (tn *TestSingleNode) TestAccountState() {
	testcases := []struct {
		name          string
		accountArgs   *rpcargs.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account state for existing address",
			accountArgs: &rpcargs.GetAccountArgs{
				Address: common.SargaAddress,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch account state for non-existing address",
			accountArgs: &rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			accountState, err := tn.moiClient.AccountState(context.Background(), test.accountArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), uint64(0), accountState.Nonce.ToUint64())
			require.NotNil(tn.T(), accountState.ContextHash)
		})
	}
}

func (tn *TestSingleNode) TestLogicStorage() {
	testcases := []struct {
		name             string
		logicStorageArgs *rpcargs.GetLogicStorageArgs
		expectedError    error
	}{
		{
			name: "fetch storage value for existing logic ID",
			logicStorageArgs: &rpcargs.GetLogicStorageArgs{
				LogicID:    common.GuardianLogicID,
				StorageKey: pisa.Slothash(gtypes.GuardianSLot),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch storage value for non-existing logic ID",
			logicStorageArgs: &rpcargs.GetLogicStorageArgs{
				LogicID:    "",
				StorageKey: common.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("invalid logic ID"),
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			logicStorageValue, err := tn.moiClient.LogicStorage(context.Background(), test.logicStorageArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Greater(tn.T(), len(logicStorageValue), 0)
		})
	}
}

func (tn *TestSingleNode) TestLogics() {
	testcases := []struct {
		name          string
		LogicIDArgs   *rpcargs.GetLogicIDArgs
		expectedError error
	}{
		{
			name: "fetch logicIDs for existing address",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				Address: common.GuardianLogicAddr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch logicIDs for non-existing address",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				Address: tests.RandomAddress(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			logicIDs, err := tn.moiClient.LogicIDs(context.Background(), test.LogicIDArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), common.GuardianLogicID, logicIDs[0])
		})
	}
}

func (tn *TestSingleNode) TestLogicManifest() {
	testcases := []struct {
		name              string
		logicManifestArgs *rpcargs.LogicManifestArgs
		expectedError     error
	}{
		{
			name: "fetch json logic manifest for existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  common.GuardianLogicID,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch logic manifest for non-existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  "0200000070c34ed6ec4384c75d469894052647a078b33ac0f08db0d3751c1fce29a49f",
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			manifest, err := tn.moiClient.LogicManifest(context.Background(), test.logicManifestArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)

			expectedManifest, err := GetLogicManifestByEncodingType(tn.T(), tn.genesis.Logics[0].Manifest,
				"JSON")
			require.NoError(tn.T(), err)

			require.Equal(tn.T(), expectedManifest, manifest)
		})
	}
}

func (tn *TestSingleNode) TestContent() {
	ctx := context.Background()
	testcases := []struct {
		name          string
		contentArgs   *rpcargs.ContentArgs
		expectedError error
	}{
		{
			name:        "fetch content from ixpool",
			contentArgs: &rpcargs.ContentArgs{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			contentResponse, err := tn.moiClient.Content(ctx, test.contentArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 3, len(contentResponse.Pending[tn.accounts[0].Addr]))
			require.Equal(tn.T(), 1, len(contentResponse.Queued[tn.accounts[0].Addr]))
			require.Equal(tn.T(), 1, len(contentResponse.Queued[tn.accounts[1].Addr]))
		})
	}
}

func (tn *TestSingleNode) TestContentFrom() {
	ctx := context.Background()
	testcases := []struct {
		name                 string
		ixPoolArgs           *rpcargs.IxPoolArgs
		expectedPendingCount int
		expectedQueuedCount  int
	}{
		{
			name: "fetch content from for existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tn.accounts[0].Addr,
			},
			expectedPendingCount: 3,
			expectedQueuedCount:  1,
		},
		{
			name: "fetch  content from for non-existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tests.RandomAddress(tn.T()),
			},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			contentFromResponse, err := tn.moiClient.ContentFrom(ctx, test.ixPoolArgs)
			require.NoError(tn.T(), err)

			require.Equal(tn.T(), test.expectedPendingCount, len(contentFromResponse.Pending))
			require.Equal(tn.T(), test.expectedQueuedCount, len(contentFromResponse.Queued))
		})
	}
}

func (tn *TestSingleNode) TestStatus() {
	ctx := context.Background()
	testcases := []struct {
		name       string
		ixPoolArgs *rpcargs.StatusArgs
	}{
		{
			name:       "fetch status of ixpool",
			ixPoolArgs: &rpcargs.StatusArgs{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			statusResponse, err := tn.moiClient.Status(ctx, test.ixPoolArgs)
			require.NoError(tn.T(), err)

			require.Equal(tn.T(), uint64(3), statusResponse.Pending.ToUint64())
			require.Equal(tn.T(), uint64(2), statusResponse.Queued.ToUint64())
		})
	}
}

func (tn *TestSingleNode) TestInspect() {
	ctx := context.Background()
	testcases := []struct {
		name          string
		inspectArgs   *rpcargs.InspectArgs
		expectedError error
	}{
		{
			name:        "inspect ixpool",
			inspectArgs: &rpcargs.InspectArgs{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			inspectResponse, err := tn.moiClient.Inspect(ctx, test.inspectArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 3, len(inspectResponse.Pending[tn.accounts[0].Addr.String()]))
			require.Equal(tn.T(), 1, len(inspectResponse.Queued[tn.accounts[0].Addr.String()]))
			require.Equal(tn.T(), 1, len(inspectResponse.Queued[tn.accounts[1].Addr.String()]))
			require.Equal(tn.T(), 2, len(inspectResponse.WaitTime))
		})
	}
}

func (tn *TestSingleNode) TestWaitTime() {
	testcases := []struct {
		name          string
		ixPoolArgs    *rpcargs.IxPoolArgs
		expectedError error
	}{
		{
			name: "fetch wait time for existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tn.accounts[0].Addr,
			},
		},
		{
			name: "fetch wait time for non-existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tests.RandomAddress(tn.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			waitTimeResp, err := tn.moiClient.WaitTime(context.Background(), test.ixPoolArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Greater(tn.T(), waitTimeResp.Time.ToInt().Uint64(), uint64(0))
		})
	}
}

func (tn *TestSingleNode) TestVersion() {
	testcases := []struct {
		name          string
		netArgs       *rpcargs.NetArgs
		expectedValue string
	}{
		{
			name:          "fetch version",
			netArgs:       &rpcargs.NetArgs{},
			expectedValue: config.ProtocolVersion,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			version, err := tn.moiClient.Version(context.Background(), test.netArgs)
			require.NoError(tn.T(), err)

			expectedVersion := config.ProtocolVersion

			require.Equal(tn.T(), expectedVersion, version)
		})
	}
}

func (tn *TestSingleNode) TestInfo() {
	testcases := []struct {
		name    string
		netArgs *rpcargs.NetArgs
	}{
		{
			name:    "fetch node's krama id",
			netArgs: &rpcargs.NetArgs{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			nodeInfo, err := tn.moiClient.Info(context.Background(), test.netArgs)
			require.NoError(tn.T(), err)

			require.Equal(tn.T(), tn.instances[0].KramaID, string(nodeInfo.KramaID))
		})
	}
}

func (tn *TestSingleNode) TestSendInteraction() {
	testcases := []struct {
		name          string
		ixPoolArgs    *common.SendIXArgs
		expectedError error
	}{
		{
			name: "invalid account",
			ixPoolArgs: &common.SendIXArgs{
				Type:      common.IxValueTransfer,
				Sender:    tests.RandomAddress(tn.T()),
				FuelPrice: big.NewInt(1),
				FuelLimit: 200,
			},
			expectedError: common.ErrInsufficientFunds,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			bz, err := polo.Polorize(test.ixPoolArgs)
			require.NoError(tn.T(), err)

			sendIx := &rpcargs.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: "",
			}

			_, err = tn.moiClient.SendInteractions(context.Background(), sendIx)
			require.ErrorContains(tn.T(), err, test.expectedError.Error())
		})
	}
}

func (tn *TestSingleNode) TestAccounts() {
	testcases := []struct {
		name    string
		accArgs *rpcargs.AccountArgs
	}{
		{
			name:    "fetch accounts",
			accArgs: &rpcargs.AccountArgs{},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			accountsResponse, err := tn.moiClient.Accounts(context.Background())
			require.NoError(tn.T(), err)

			// make sure genesis accounts exists in fetched accounts
			for _, expectedAccount := range tn.genesis.Accounts {
				found := 0

				for _, address := range accountsResponse {
					if expectedAccount.Address == address {
						found = 1

						break
					}
				}

				require.Equal(tn.T(), found, 1, "address not found")
			}
		})
	}
}

func (tn *TestSingleNode) TestAccountMetaInfo() {
	testcases := []struct {
		name          string
		accArgs       *rpcargs.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account meta info for sarga address",
			accArgs: &rpcargs.GetAccountArgs{
				Address: common.SargaAddress,
			},
		},
		{
			name: "fetch account meta info for random address",
			accArgs: &rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(tn.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			accountMetaInfoResponse, err := tn.moiClient.AccountMetaInfo(context.Background(), test.accArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)

			require.Equal(tn.T(), common.SargaAddress, accountMetaInfoResponse.Address)
			require.Equal(tn.T(), common.SargaAccount, accountMetaInfoResponse.Type)
		})
	}
}

func (tn *TestSingleNode) TestNewTesseractFilter() {
	testcases := []struct {
		name string
		args *rpcargs.TesseractFilterArgs
	}{
		{
			name: "add tesseract filter successfully",
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			resp, err := tn.moiClient.NewTesseractFilter(context.Background(), test.args)
			require.NoError(tn.T(), err)
			require.NotEqual(tn.T(), "", resp.FilterID)
		})
	}
}

func (tn *TestSingleNode) TestNewTesseractsByAccountFilter() {
	testcases := []struct {
		name string
		args *rpcargs.TesseractByAccountFilterArgs
	}{
		{
			name: "add tesseract by account filter successfully",
			args: &rpcargs.TesseractByAccountFilterArgs{Addr: tests.RandomAddress(tn.T())},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			resp, err := tn.moiClient.NewTesseractsByAccountFilter(context.Background(), test.args)
			require.NoError(tn.T(), err)
			require.NotEqual(tn.T(), "", resp.FilterID)
		})
	}
}

func (tn *TestSingleNode) TestNewLogFilter() {
	testcases := []struct {
		name            string
		filterQueryArgs *websocket.LogQuery
		expectedError   error
	}{
		{
			name: "add log filter successfully",
			filterQueryArgs: &websocket.LogQuery{
				Address: tests.RandomAddress(tn.T()),
			},
		},
		{
			name:            "failed to add log filter",
			filterQueryArgs: &websocket.LogQuery{},
			expectedError:   common.ErrInvalidAddress,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			resp, err := tn.moiClient.NewLogFilter(context.Background(), test.filterQueryArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.NotEqual(tn.T(), "", resp.FilterID)
		})
	}
}

func (tn *TestSingleNode) TestPendingIxnsFilter() {
	testcases := []struct {
		name string
		args *rpcargs.PendingIxnsFilterArgs
	}{
		{
			name: "add pending ixns filter successfully",
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			resp, err := tn.moiClient.PendingIxnsFilter(context.Background(), test.args)
			require.NoError(tn.T(), err)
			require.NotEqual(tn.T(), "", resp.FilterID)
		})
	}
}

func (tn *TestSingleNode) TestRemoveFilter() {
	testcases := []struct {
		name string
	}{
		{
			name: "remove filter successfully",
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			filterResp, err := tn.moiClient.NewTesseractFilter(context.Background(), &rpcargs.TesseractFilterArgs{})
			require.NoError(tn.T(), err)

			resp, err := tn.moiClient.RemoveFilter(context.Background(), &rpcargs.FilterArgs{
				FilterID: filterResp.FilterID,
			})
			require.NoError(tn.T(), err)
			require.True(tn.T(), resp.Status)
		})
	}
}

func (tn *TestSingleNode) TestFuelEstimate() {
	addr := tn.accounts[0].Addr

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "BTC",
		Supply: big.NewInt(22100),
	}

	rawAssetCreatePayload, err := assetCreationPayload.Bytes()
	require.NoError(tn.T(), err)

	ixArgsWithFuelParams := &rpcargs.IxArgs{
		Type:      common.IxAssetCreate,
		Sender:    addr,
		FuelPrice: (*hexutil.Big)(big.NewInt(1)),
		FuelLimit: hexutil.Uint64(200),
		Payload:   (hexutil.Bytes)(rawAssetCreatePayload),
	}

	ixArgsWithoutFuelParams := &rpcargs.IxArgs{
		Type:    common.IxAssetCreate,
		Sender:  addr,
		Payload: (hexutil.Bytes)(rawAssetCreatePayload),
	}

	testcases := []struct {
		name                 string
		callArgs             *rpcargs.CallArgs
		expectedFuelConsumed *hexutil.Big
		expectedError        error
	}{
		{
			name: "retrieved fuel used in asset create ixn when fuel limit and price are given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
		{
			name: "retrieved fuel used in asset create ixn when fuel limit and price are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithoutFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
		{
			name: "failed to fetch fuel estimate as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: nil,
					},
				},
			},
			expectedError: common.ErrEmptyOptions,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			fuelConsumed, err := tn.moiClient.FuelEstimate(context.Background(), test.callArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), test.expectedFuelConsumed, fuelConsumed)
		})
	}
}

func (tn *TestSingleNode) TestCall() {
	addr := tn.accounts[0].Addr
	invalidHeight := int64(-2)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: big.NewInt(999),
	}

	rawAssetPayload, err := assetCreationPayload.Bytes()
	require.NoError(tn.T(), err)

	ixArgsWithFuelParams := &rpcargs.IxArgs{
		Type:      common.IxAssetCreate,
		Sender:    addr,
		FuelPrice: (*hexutil.Big)(big.NewInt(1)),
		FuelLimit: hexutil.Uint64(200),
		Payload:   (hexutil.Bytes)(rawAssetPayload),
	}

	ixArgsWithoutFuelParams := &rpcargs.IxArgs{
		Type:    common.IxAssetCreate,
		Sender:  addr,
		Payload: (hexutil.Bytes)(rawAssetPayload),
	}

	expectedReceipt := &common.Receipt{
		IxType:   common.IxAssetCreate,
		FuelUsed: 100,
	}

	expectedAssetAddr := common.NewAccountAddress(0, addr)
	expectedAssetID := identifiers.NewAssetIDv0(false, false, 0, 0, expectedAssetAddr)

	common.SetReceiptExtraData(expectedReceipt, common.AssetCreationReceipt{
		AssetID:      expectedAssetID,
		AssetAccount: expectedAssetAddr,
	})

	testcases := []struct {
		name            string
		callArgs        *rpcargs.CallArgs
		expectedReceipt *rpcargs.RPCReceipt
		expectedError   error
	}{
		{
			name: "fetched rpc receipt successfully when fuel limit and price are given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxType:    hexutil.Uint64(ixArgsWithFuelParams.Type),
				FuelUsed:  hexutil.Uint64(expectedReceipt.FuelUsed),
				ExtraData: expectedReceipt.ExtraData,
				From:      addr,
				To:        expectedAssetAddr,
			},
		},
		{
			name: "fetched rpc receipt successfully when fuel limit and price are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithoutFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxType:    hexutil.Uint64(ixArgsWithoutFuelParams.Type),
				FuelUsed:  hexutil.Uint64(expectedReceipt.FuelUsed),
				ExtraData: expectedReceipt.ExtraData,
				From:      addr,
				To:        expectedAssetAddr,
			},
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Address]*rpcargs.TesseractNumberOrHash{
					addr: {
						TesseractNumber: &invalidHeight,
					},
				},
			},
			expectedError: errors.New("invalid options"),
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			receipt, err := tn.moiClient.InteractionCall(context.Background(), test.callArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			checkForCallReceipt(tn.T(), test.expectedReceipt, receipt)
		})
	}
}
