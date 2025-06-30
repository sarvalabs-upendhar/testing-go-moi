package moiclient

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/compute/exlogics/tokenledger"
	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/bgclient"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
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
	d.WithLogs = true
	d.WithStdout = false
	d.LogLevel = "TRACE"
	d.BootNodePort = 21000
	d.Libp2pPort = 22000
	d.JSONRPCPort = 23000
	d.ValidatorCount = 5
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

	id0 := tn.accounts[0].ID
	id1 := tn.accounts[1].ID

	for i := 0; i < 3; i++ {
		// promoted queue
		createAssetWithNonce(tn.T(), tn.moiClient, id0, uint64(i), tn.accounts[0])
	}

	// enqueued queue
	createAssetWithNonce(tn.T(), tn.moiClient, id0, uint64(4), tn.accounts[0])
	// different account
	createAssetWithNonce(tn.T(), tn.moiClient, id1, uint64(4), tn.accounts[1])

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
				ID:               common.SargaAccountID,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &genesisHeight,
				},
			},
		},
		{
			name: "should return error as invalid id",
			tesseractArgs: &rpcargs.TesseractArgs{
				ID:               tests.RandomIdentifier(tn.T()),
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
			require.True(tn.T(), ts.HasParticipant(test.tesseractArgs.ID))
			require.Equal(tn.T(), 0, len(ts.Ixns))
		})
	}
}

func (tn *TestSingleNode) TestDBGet() {
	// key and value belongs to genesis tesseract account meta info
	key, _ := storage.BucketKeyAndID(storage.NewIdentifierKey(common.SargaAccountID))
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
			require.Equal(tn.T(), common.SargaAccountID, accMetaInfo.ID)
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
				ID: tests.RandomIdentifier(tn.T()),
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
			name:    "fetch asset info for existing assetID",
			assetID: common.CreateAssetIDFromString(a.Symbol, 0, uint16(a.Standard), a.AssetDescriptor().Flags()...),
		},
		{
			name:          "fetch asset info for non-existing assetID",
			assetID:       tests.GetRandomAssetID(tn.T(), tests.RandomIdentifier(tn.T())),
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
				ID:      a.Allocations[0].ID,
				AssetID: common.KMOITokenAssetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedBalance: a.Allocations[0].Amount,
		},
		{
			name: "get balance returns error for unknown asset ID",
			balanceArgs: &rpcargs.BalArgs{
				ID:      a.Allocations[0].ID,
				AssetID: tests.GetRandomAssetID(tn.T(), tests.RandomIdentifier(tn.T())),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAssetNotFound,
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
			name: "fetch TDU for existing id",
			queryArgs: &rpcargs.QueryArgs{
				ID: a.Allocations[0].ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch TDU for non-existing id",
			queryArgs: &rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(tn.T()),
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

func (tn *TestSingleNode) TestDeeds() {
	a := tn.genesis.AssetAccounts[1].AssetInfo

	assetID := common.CreateAssetIDFromString(a.Symbol, 0, uint16(a.Standard), a.AssetDescriptor().Flags()...)

	testcases := []struct {
		name          string
		queryArgs     *rpcargs.QueryArgs
		expectedError error
	}{
		{
			name: "fetch registry for existing id",
			queryArgs: &rpcargs.QueryArgs{
				ID: a.Allocations[0].ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch registry for non-existing id",
			queryArgs: &rpcargs.QueryArgs{
				ID: tests.RandomIdentifier(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			deeds, err := tn.moiClient.Deeds(context.Background(), test.queryArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 1, len(deeds))
			require.Equal(tn.T(), assetID.String(), deeds[0].AssetID)
			require.Equal(tn.T(), a.Operator, deeds[0].AssetInfo.Operator)
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
			name: "fetch context info for existing id",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				ID: common.SargaAccountID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch context info for non-existing id",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				ID: tests.RandomIdentifier(tn.T()),
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

			for i := 0; i < len(tn.genesis.SargaAccount.ConsensusNodes); i++ {
				require.Equal(tn.T(), string(tn.genesis.SargaAccount.ConsensusNodes[i]),
					contextInfo.ConsensusNodes[i])
			}

			require.Equal(tn.T(), 0, len(contextInfo.SubAccounts))
			require.True(tn.T(), contextInfo.InheritedAccount.IsNil())
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
			name: "fetch interaction count for existing id",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				ID: tn.accounts[0].ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch interaction count for non-existing id",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				ID: tests.RandomIdentifier(tn.T()),
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
		expectedSequenceID   uint64
		expectedError        error
	}{
		{
			name: "fetch pending interaction count for non-existing id",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				ID: tests.RandomIdentifier(tn.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetch pending interaction count for existing id",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				ID: tn.accounts[0].ID,
			},
			expectedSequenceID: 3,
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
			require.Equal(tn.T(), test.expectedSequenceID, pendingInteractionCount.ToUint64())
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
			name: "fetch account state for existing id",
			accountArgs: &rpcargs.GetAccountArgs{
				ID: common.SargaAccountID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch account state for non-existing id",
			accountArgs: &rpcargs.GetAccountArgs{
				ID: tests.RandomIdentifier(tn.T()),
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
			require.NotNil(tn.T(), accountState.ContextHash)
			require.Equal(tn.T(), common.SargaAccount, accountState.AccType)
		})
	}
}

func (tn *TestSingleNode) TestAccountKeys() {
	testcases := []struct {
		name          string
		accKeysArgs   *rpcargs.GetAccountKeysArgs
		expectedError error
	}{
		{
			name: "fetch account state for existing id",
			accKeysArgs: &rpcargs.GetAccountKeysArgs{
				ID: tn.accounts[0].ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch account state for non-existing id",
			accKeysArgs: &rpcargs.GetAccountKeysArgs{
				ID: tests.RandomIdentifier(tn.T()),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			accKeys, err := tn.moiClient.AccountKeys(context.Background(), test.accKeysArgs)

			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), 1, len(accKeys))
			require.Equal(tn.T(), uint64(1000), accKeys[0].Weight.ToUint64())
			require.Equal(tn.T(), uint64(0), accKeys[0].ID.ToUint64())
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
				StorageKey: pisa.GenerateStorageKey(guardianregistry.SlotGuardians),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch storage value for non-existing logic ID",
			logicStorageArgs: &rpcargs.GetLogicStorageArgs{
				LogicID:    identifiers.RandomLogicIDv0(),
				StorageKey: common.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: common.ErrAccountNotFound,
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
			name: "fetch logicIDs for existing id",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				ID: common.GuardianAccountID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch logicIDs for non-existing id",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				ID: tests.RandomIdentifier(tn.T()),
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
				LogicID:  identifiers.RandomLogicIDv0(),
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
			require.Equal(tn.T(), 3, len(contentResponse.Pending[tn.accounts[0].ID]))
			require.Equal(tn.T(), 1, len(contentResponse.Queued[tn.accounts[0].ID]))
			require.Equal(tn.T(), 1, len(contentResponse.Queued[tn.accounts[1].ID]))
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
			name: "fetch content from for existing id",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				ID: tn.accounts[0].ID,
			},
			expectedPendingCount: 3,
			expectedQueuedCount:  1,
		},
		{
			name: "fetch  content from for non-existing id",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				ID: tests.RandomIdentifier(tn.T()),
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
			require.Equal(tn.T(), 3, len(inspectResponse.Pending[tn.accounts[0].ID.String()]))
			require.Equal(tn.T(), 1, len(inspectResponse.Queued[tn.accounts[0].ID.String()]))
			require.Equal(tn.T(), 1, len(inspectResponse.Queued[tn.accounts[1].ID.String()]))
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
			name: "fetch wait time for existing id",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				ID: tn.accounts[0].ID,
			},
		},
		{
			name: "fetch wait time for non-existing id",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				ID: tests.RandomIdentifier(tn.T()),
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

			found := false

			for _, ins := range tn.instances {
				if ins.KramaID == string(nodeInfo.KramaID) {
					found = true

					break
				}
			}

			require.True(tn.T(), found)
		})
	}
}

func (tn *TestSingleNode) TestSendInteraction() {
	testcases := []struct {
		name          string
		ixPoolArgs    *common.IxData
		expectedError error
	}{
		{
			name: "invalid account",
			ixPoolArgs: &common.IxData{
				Sender: common.Sender{
					ID: tests.RandomIdentifier(tn.T()),
				},
				FuelPrice: big.NewInt(1),
				FuelLimit: 200,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(tn.T(), identifiers.Nil),
					},
				},
			},
			expectedError: common.ErrMissingSender,
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			bz, err := polo.Polorize(test.ixPoolArgs)
			require.NoError(tn.T(), err)

			signatures := make(common.Signatures, 0)
			data, err := signatures.Bytes()
			require.NoError(tn.T(), err)

			sendIx := &rpcargs.SendIX{
				IXArgs:     hex.EncodeToString(bz),
				Signatures: hex.EncodeToString(data),
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

				for _, id := range accountsResponse {
					if expectedAccount.ID == id {
						found = 1

						break
					}
				}

				require.Equal(tn.T(), found, 1, "id not found")
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
			name: "fetch account meta info for sarga id",
			accArgs: &rpcargs.GetAccountArgs{
				ID: common.SargaAccountID,
			},
		},
		{
			name: "fetch account meta info for random id",
			accArgs: &rpcargs.GetAccountArgs{
				ID: tests.RandomIdentifier(tn.T()),
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

			require.Equal(tn.T(), common.SargaAccountID, accountMetaInfoResponse.ID)
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
			args: &rpcargs.TesseractByAccountFilterArgs{ID: tests.RandomIdentifier(tn.T())},
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
		filterQueryArgs *jsonrpc.LogQuery
		expectedError   error
	}{
		{
			name: "add log filter successfully",
			filterQueryArgs: &jsonrpc.LogQuery{
				ID: tests.RandomIdentifier(tn.T()),
			},
		},
		{
			name:            "failed to add log filter",
			filterQueryArgs: &jsonrpc.LogQuery{},
			expectedError:   errors.New("Invalid Params"),
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
	id := tn.accounts[0].ID

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "BTC",
		Supply: big.NewInt(22100),
	}

	rawAssetCreatePayload, err := assetCreationPayload.Bytes()
	require.NoError(tn.T(), err)

	logicPayload := common.LogicPayload{
		Manifest: common.Hex2Bytes(manifest),
		Callsite: "Seed",
		Calldata: common.Hex2Bytes("0x0d6f0665b6019502737570706c790305f5e10073796d626f6c064d4f49"),
	}

	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(tn.T(), err)

	ixArgsWithFuelParams := &rpcargs.IxArgs{
		Sender: common.Sender{
			ID: id,
		},
		FuelPrice: (*hexutil.Big)(big.NewInt(1)),
		FuelLimit: hexutil.Uint64(200),
		IxOps: []rpcargs.IxOp{
			{
				Type:    common.IxAssetCreate,
				Payload: (hexutil.Bytes)(rawAssetCreatePayload),
			},
		},
		Participants: []rpcargs.IxParticipant{{
			ID:       id,
			LockType: common.MutateLock,
		}},
	}

	ixArgsWithoutFuelParams := &rpcargs.IxArgs{
		Sender: common.Sender{
			ID: id,
		},
		IxOps: []rpcargs.IxOp{
			{
				Type:    common.IxLogicDeploy,
				Payload: (hexutil.Bytes)(rawLogicPayload),
			},
		},
		Participants: []rpcargs.IxParticipant{{
			ID:       id,
			LockType: common.MutateLock,
		}},
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
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
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
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(3473)),
		},
		{
			name: "failed to fetch fuel estimate as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
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
	id := tn.accounts[0].ID
	invalidHeight := int64(-2)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: big.NewInt(999),
	}

	rawAssetPayload, err := assetCreationPayload.Bytes()
	require.NoError(tn.T(), err)

	inputs := tokenledger.InputSeed{
		Symbol: "MOI",
		Supply: 100000000,
	}

	DeployCallData, _ := polo.PolorizeDocument(inputs, polo.DocStructs())

	logicPayload := common.LogicPayload{
		Manifest: common.Hex2Bytes(manifest),
		Callsite: "Seed",
		Calldata: DeployCallData.Bytes(),
	}

	assetID, _ := identifiers.GenerateAssetIDv0(
		common.NewAccountID(common.Sender{
			ID:         id,
			KeyID:      0,
			SequenceID: 0,
		},
		),
		0,
		uint16(assetCreationPayload.Standard),
		assetCreationPayload.Flags()...,
	)

	logicID, _ := identifiers.GenerateLogicIDv0(
		common.NewAccountID(common.Sender{
			ID:         id,
			KeyID:      0,
			SequenceID: 0,
		}),
		0,
		logicPayload.Flags()...,
	)

	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(tn.T(), err)

	ixArgsWithFuelParams := &rpcargs.IxArgs{
		Sender: common.Sender{
			ID: id,
		},
		FuelPrice: (*hexutil.Big)(big.NewInt(1)),
		FuelLimit: hexutil.Uint64(200),
		IxOps: []rpcargs.IxOp{
			{
				Type:    common.IxLogicDeploy,
				Payload: (hexutil.Bytes)(rawLogicPayload),
			},
		},
		Participants: []rpcargs.IxParticipant{{
			ID:       id,
			LockType: common.MutateLock,
		}},
	}

	ixArgsWithoutFuelParams := &rpcargs.IxArgs{
		Sender: common.Sender{
			ID: id,
		},
		IxOps: []rpcargs.IxOp{
			{
				Type:    common.IxAssetCreate,
				Payload: (hexutil.Bytes)(rawAssetPayload),
			},
		},
		Participants: []rpcargs.IxParticipant{{
			ID:       id,
			LockType: common.MutateLock,
		}},
	}

	receiptWithFuelParams := &common.IxOpResult{
		IxType: common.IxLogicDeploy,
	}

	receiptWithoutFuelParams := &common.IxOpResult{
		IxType: common.IxAssetCreate,
	}

	common.SetResultPayload(receiptWithoutFuelParams, common.AssetCreationResult{
		AssetID: assetID,
	})

	common.SetResultPayload(receiptWithFuelParams, common.LogicDeployResult{
		LogicID: logicID,
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
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				FuelUsed: hexutil.Uint64(3473),
				From:     id,
				IxOps: []*rpcargs.RPCIxOpResult{
					{
						TxType: hexutil.Uint64(ixArgsWithFuelParams.IxOps[0].Type),
						Data:   receiptWithFuelParams.Data,
					},
				},
			},
		},
		{
			name: "fetched rpc receipt successfully when fuel limit and price are not given",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithoutFuelParams,
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
						TesseractNumber: &LatestTesseractNumber,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				FuelUsed: hexutil.Uint64(100),
				From:     id,
				IxOps: []*rpcargs.RPCIxOpResult{
					{
						TxType: hexutil.Uint64(ixArgsWithoutFuelParams.IxOps[0].Type),
						Data:   receiptWithoutFuelParams.Data,
					},
				},
			},
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: ixArgsWithFuelParams,
				Options: map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash{
					id: {
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

func (tn *TestSingleNode) TestSubAccountCount() {
	ctx := context.Background()
	testcases := []struct {
		name          string
		StatusArgs    *rpcargs.SubAccountCountArgs
		expectedError error
	}{
		{
			name: "should return error if failed to fetch sub account count",
			StatusArgs: &rpcargs.SubAccountCountArgs{
				ID: tests.RandomIdentifierWithZeroVariant(tn.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "sub account count fetched successfully",
			StatusArgs: &rpcargs.SubAccountCountArgs{
				ID: tn.accounts[0].ID,
			},
		},
	}

	for _, test := range testcases {
		tn.Run(test.name, func() {
			count, err := tn.moiClient.SubAccountCount(ctx, test.StatusArgs)
			if test.expectedError != nil {
				require.ErrorContains(tn.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tn.T(), err)
			require.Equal(tn.T(), uint64(0), count.ToUint64())
		})
	}
}
