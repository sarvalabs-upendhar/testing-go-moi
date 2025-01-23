package moiclient

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/bgclient"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Guidelines for creating MOIClient tests:
// This file should encompass MOIClient tests that require interactions to be finalized.

type TestMultiNode struct {
	suite.Suite
	moiClient      *Client
	bgClient       bgclient.Client
	jsonRPCUrls    []string
	accounts       []tests.AccountWithMnemonic
	logger         hclog.Logger
	instances      []common.Instance
	ixHash         common.Hash
	suiteSetupDone bool
}

const filterTimeout = 5 * time.Second

var setupSuiteAssetID, setupSuiteSenderID identifiers.Identifier

func (tm *TestMultiNode) runCriticallyNecessaryTearDown() {
	err := tm.bgClient.DestroyNetwork(context.Background(), true)
	tm.Suite.NoError(err)
}

func (tm *TestMultiNode) initLogger() {
	tm.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString("TRACE"),
	})
}

func (tm *TestMultiNode) SetupSuite() {
	defer func() {
		// make sure to delete directories in case of setup suite failure
		if !tm.suiteSetupDone {
			tm.logger.Error("Setup suite failed")
			tm.runCriticallyNecessaryTearDown()
		}
	}()

	tm.initLogger()

	const multiNodeJSONRPCPort int64 = 26000

	d := bgclient.DefaultClusterConfig()

	d.WithLogs = false
	d.WithStdout = false
	d.LogLevel = "TRACE"
	d.BootNodePort = 24000
	d.Libp2pPort = 25000
	d.JSONRPCPort = multiNodeJSONRPCPort
	d.ValidatorCount = 20
	d.GenesisAssetCount = 0

	tm.bgClient = bgclient.NewClient(&bgclient.Config{
		ClusterConfig: d,
		Network:       bgclient.LOCAL,
	})

	_, err := tm.bgClient.StartNetwork(context.Background())
	tm.Suite.NoError(err)

	tm.jsonRPCUrls = GetJSONRPCUrls(tm.Suite.T(), tm.bgClient, d.ValidatorCount)

	tm.logger.Debug("multinode json urls ", "urls", tm.jsonRPCUrls)

	// wait for initial sync and all node modules to start
	CheckIfNodesInitialSyncDone(tm.Suite.T(), d.ValidatorCount, tm.jsonRPCUrls)

	tm.moiClient, err = NewClient(fmt.Sprintf("http://localhost:%d", multiNodeJSONRPCPort))
	tm.Suite.NoError(err)

	tm.instances, err = common.ReadInstancesFile(filepath.Join(d.TempDir, "instances.json"))
	tm.Suite.NoError(err)

	tm.accounts, err = tm.bgClient.Accounts(context.Background())
	tm.Suite.NoError(err)

	// a tesseract is generated to provide data for tesseract related api
	// fire and finalize ixn and store ix hash
	tm.ixHash, setupSuiteAssetID = createAsset(tm.T(), tm.moiClient, tm.accounts[0].ID, tm.accounts[0])
	setupSuiteSenderID = tm.accounts[0].ID

	tm.suiteSetupDone = true
}

func (tm *TestMultiNode) TearDownSuite() {
	tm.logger.Debug("tear down suite called")

	tm.runCriticallyNecessaryTearDown()
}

func TestMoiClientApiMultiNode(t *testing.T) {
	suite.Run(t, new(TestMultiNode))
}

func (tm *TestMultiNode) TestPeers() {
	testcases := []struct {
		name    string
		netArgs *rpcargs.NetArgs
	}{
		{
			name:    "fetch peers",
			netArgs: &rpcargs.NetArgs{},
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			clientPeers, err := tm.moiClient.Peers(context.Background(), test.netArgs)
			require.NoError(tm.T(), err)

			require.Greater(tm.T(), len(clientPeers), 0)

			// make sure fetched peers exists in instances file
			for _, peer := range clientPeers {
				found := 0

				for _, instance := range tm.instances {
					if string(peer) == instance.KramaID {
						found = 1

						break
					}
				}

				require.Equal(tm.T(), 1, found, "")
			}
		})
	}
}

func (tm *TestMultiNode) TestConnections() {
	testcases := []struct {
		name    string
		accArgs *rpcargs.ConnArgs
	}{
		{
			name:    "fetch connections",
			accArgs: &rpcargs.ConnArgs{},
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			connResp, err := tm.moiClient.Connections(context.Background())
			require.NoError(tm.T(), err)
			require.True(tm.T(), connResp.InboundConnCount > 0 || connResp.OutboundConnCount > 0)
			require.Greater(tm.T(), len(connResp.ActivePubSubTopics), 0)
		})
	}
}

func (tm *TestMultiNode) TestNodeMetaInfo() {
	testCases := []struct {
		name          string
		nodeArgs      *rpcargs.NodeMetaInfoArgs
		expectedError error
	}{
		{
			name: "fetch node meta info for peer id",
			nodeArgs: &rpcargs.NodeMetaInfoArgs{
				PeerID: GetPeerID(tm.T(), tm.moiClient).String(),
			},
		},
		{
			name: "fetch node meta info for random peer id",
			nodeArgs: &rpcargs.NodeMetaInfoArgs{
				PeerID: tests.RandomPeerID(tm.T()).String(),
			},
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testCases {
		tm.Run(test.name, func() {
			nodeMetaInfoResponse, err := tm.moiClient.NodeMetaInfo(context.Background(), test.nodeArgs)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)

			_, ok := nodeMetaInfoResponse[test.nodeArgs.PeerID]
			require.True(tm.T(), ok)
		})
	}
}

func (tm *TestMultiNode) TestInteractionByHash() {
	testcases := []struct {
		name          string
		ixArgs        *rpcargs.InteractionByHashArgs
		expectedError error
	}{
		{
			name: "fetch interaction for existing ix hash",
			ixArgs: &rpcargs.InteractionByHashArgs{
				Hash: tm.ixHash,
			},
		},
		{
			name: "fetch interaction for non-existing ix hash",
			ixArgs: &rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(tm.T()),
			},
			expectedError: common.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			rpcIxn, err := tm.moiClient.InteractionByHash(context.Background(), test.ixArgs)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)
			require.Equal(tm.T(), tm.accounts[0].ID, rpcIxn.Sender.ID)
		})
	}
}

func (tm *TestMultiNode) TestInteractionByTesseract() {
	randomHash := tests.RandomHash(tm.T())
	ixIndex := uint64(0)

	testcases := []struct {
		name          string
		ixArgs        *rpcargs.InteractionByTesseract
		expectedError error
	}{
		{
			name: "fetch interaction for existing tesseract hash",
			ixArgs: &rpcargs.InteractionByTesseract{
				ID: tm.accounts[0].ID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
		},
		{
			name: "fetch interaction for non-existing tesseract hash",
			ixArgs: &rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("interaction not found"),
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			rpcIxn, err := tm.moiClient.InteractionByTesseract(context.Background(), test.ixArgs)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)
			require.Equal(tm.T(), tm.accounts[0].ID, rpcIxn.Sender.ID)
		})
	}
}

func (tm *TestMultiNode) TestInteractionReceipt() {
	testcases := []struct {
		name          string
		receiptArgs   *rpcargs.ReceiptArgs
		expectedError error
	}{
		{
			name: "fetch receipt for existing hash",
			receiptArgs: &rpcargs.ReceiptArgs{
				Hash: tm.ixHash,
			},
		},
		{
			name: "fetch receipt for non-existing hash",
			receiptArgs: &rpcargs.ReceiptArgs{
				Hash: tests.RandomHash(tm.T()),
			},
			expectedError: common.ErrTSHashNotFound,
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			receipt, err := tm.moiClient.InteractionReceipt(context.Background(), test.receiptArgs)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)
			require.Equal(tm.T(), tm.ixHash, receipt.IxHash)
		})
	}
}

// TODO: Revise the logic once logs are produced in the execution engine and incorporated it into the receipt.
func (tm *TestMultiNode) TestGetLogs() {
	testcases := []struct {
		name            string
		filterQueryArgs *rpcargs.FilterQueryArgs
		expectedError   error
	}{
		{
			name: "get logs successfully",
			filterQueryArgs: &rpcargs.FilterQueryArgs{
				StartHeight: NumPointer(0),
				EndHeight:   NumPointer(1),
				ID:          tm.accounts[0].ID,
			},
		},
		{
			name: "failed to get logs",
			filterQueryArgs: &rpcargs.FilterQueryArgs{
				ID: tests.RandomIdentifier(tm.T()),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			resp, err := tm.moiClient.GetLogs(context.Background(), test.filterQueryArgs)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)
			require.Equal(tm.T(), 0, len(resp))
		})
	}
}

func (tm *TestMultiNode) TestGetFilterChanges() {
	var (
		ctx              = context.Background()
		acc1             = tm.accounts[1]
		acc2             = tm.accounts[2]
		expectedIXHashes = make([]common.Hash, 2)
		assetAddresses   = make([]identifiers.Identifier, 2)
	)

	tsFilter := createTesseractFilter(tm.T(), ctx, tm.moiClient)
	tsByAccFilter := createTesseractsByAccountFilter(tm.T(), ctx, tm.moiClient, acc2.ID)
	ixnsFilter := createPendingIxnsFilter(tm.T(), ctx, tm.moiClient)
	logFilter := createLogFilter(tm.T(), ctx, tm.moiClient, acc1.ID)

	// send create asset interactions
	expectedIXHashes[0], assetAddresses[0] = createAsset(tm.T(), tm.moiClient, acc1.ID, acc1)
	expectedIXHashes[1], assetAddresses[1] = createAsset(tm.T(), tm.moiClient, acc2.ID, acc2)

	testcases := []struct {
		name             string
		filterQueryArgs  *rpcargs.FilterArgs
		subscriptionType rpcargs.SubscriptionType
		msgCount         int
		expectedError    error
	}{
		{
			name: "fetch ts from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: tsFilter.FilterID,
			},
			msgCount:         2,
			subscriptionType: rpcargs.NewTesseract,
		},
		{
			name: "fetch ts by acc from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: tsByAccFilter.FilterID,
			},
			msgCount:         1,
			subscriptionType: rpcargs.NewTesseractsByAccount,
		},
		{
			name: "fetch ixns from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: ixnsFilter.FilterID,
			},
			msgCount:         2,
			subscriptionType: rpcargs.PendingIxns,
		},
		{
			name: "fetch logs from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: logFilter.FilterID,
			},
			// TODO: add msgCount once logs is supported
			subscriptionType: rpcargs.NewLogsByFilter,
		},
		{
			name: "failed to fetch data as filter does not exist",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: "hello",
			},
			expectedError: errors.New("unknown subscription type"),
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			ctx, cancel := context.WithTimeout(context.Background(), filterTimeout)
			defer cancel()

			if test.expectedError != nil {
				_, err := tm.moiClient.GetFilterChanges(ctx, test.filterQueryArgs, test.subscriptionType)
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			switch test.subscriptionType {
			case rpcargs.NewTesseract:
				rpcTS := getRPCTesseractUntilTimeout(
					tm.T(),
					ctx,
					tm.moiClient,
					test.filterQueryArgs,
					test.subscriptionType,
					test.msgCount,
				)

				// TODO: Remove code from here, after issue #756 is resolved
				var rpcTSValues []rpcargs.RPCTesseract
				for _, ptr := range rpcTS {
					rpcTSValues = append(rpcTSValues, *ptr)
				}

				var addresses []identifiers.Identifier
				for _, account := range tm.accounts {
					addresses = append(addresses, account.ID)
				}

				// TODO: tesseract addresses
				require.Equal(
					tm.T(),
					2,
					len(rpcTSValues),
					fmt.Sprintf("Expected length 2, but got %d. rpcTS: %+v", len(rpcTSValues), rpcTSValues),
					fmt.Sprint(
						"Sarga ID", common.SargaAccountID,
						"Sender IDs", acc1.ID, acc2.ID,
						"Asset ID", assetAddresses[0], assetAddresses[1],
						"Setup Suite Sender ID", setupSuiteSenderID,
						"Setup Suite Asset ID", setupSuiteAssetID,
					),
					fmt.Sprint("List of all account addresses", addresses),
				)
				// till here

				// make sure sender and sarga tesseracts are there in generated tesseracts
				require.True(tm.T(), rpcTS[0].HasParticipant(acc1.ID))
				require.True(tm.T(), rpcTS[0].HasParticipant(common.SargaAccountID))
				require.True(tm.T(), rpcTS[0].HasParticipant(assetAddresses[0]))
				require.True(tm.T(), rpcTS[1].HasParticipant(acc2.ID))
				require.True(tm.T(), rpcTS[1].HasParticipant(common.SargaAccountID))
				require.True(tm.T(), rpcTS[1].HasParticipant(assetAddresses[1]))

			case rpcargs.NewTesseractsByAccount:
				rpcTS := getRPCTesseractUntilTimeout(
					tm.T(),
					ctx,
					tm.moiClient,
					test.filterQueryArgs,
					test.subscriptionType,
					test.msgCount,
				)

				require.Equal(tm.T(), 1, len(rpcTS))
				require.True(tm.T(), rpcTS[0].HasParticipant(acc2.ID))
				require.True(tm.T(), rpcTS[0].HasParticipant(common.SargaAccountID))

			case rpcargs.PendingIxns:
				ixHashes := getIxHashesUntilTimeout(
					tm.T(),
					ctx,
					tm.moiClient,
					test.filterQueryArgs,
					test.subscriptionType,
					test.msgCount,
				)

				require.Equal(tm.T(), len(expectedIXHashes), len(ixHashes))

				for i, h := range expectedIXHashes {
					require.Equal(tm.T(), h, *ixHashes[i])
				}

			case rpcargs.NewLogsByFilter:
				resp, err := tm.moiClient.GetFilterChanges(ctx, test.filterQueryArgs, test.subscriptionType)
				require.NoError(tm.T(), err)

				logs, ok := resp.([]*rpcargs.RPCLog)
				require.True(tm.T(), ok)
				require.Equal(tm.T(), 0, len(logs))
			}
		})
	}
}

func (tm *TestMultiNode) TestGetPeersScore() {
	_, err := tm.moiClient.PeersScore(context.Background(), &rpcargs.PeerScoreRequest{})
	require.NoError(tm.T(), err)
}
