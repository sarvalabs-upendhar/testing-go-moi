package moiclient

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	bg "github.com/sarvalabs/battleground"
	client "github.com/sarvalabs/battleground/client/types"
	bgcommon "github.com/sarvalabs/battleground/common"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/websocket"
)

// Guidelines for creating MOIClient tests:
// This file should encompass MOIClient tests that require interactions to be finalized.

type TestMultiNode struct {
	suite.Suite
	moiClient      *Client
	bgClient       bg.Client
	accounts       []bgcommon.AccountWithMnemonic
	logger         hclog.Logger
	instances      []common.Instance
	ixHash         common.Hash
	suiteSetupDone bool
}

func (tm *TestMultiNode) runCriticallyNecessaryTearDown() {
	err := tm.bgClient.DestroyNetwork(context.Background(), true)
	tm.Suite.NoError(err)
}

func (tm *TestMultiNode) initLogger() {
	tm.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString("ERROR"),
	})
}

func (tm *TestMultiNode) SetupSuite() {
	defer func() {
		// make sure to delete directories incase of setup suite failure
		if !tm.suiteSetupDone {
			tm.logger.Error("setup suite failed")
			tm.runCriticallyNecessaryTearDown()
		}
	}()

	tm.initLogger()

	d := client.DefaultClusterConfig()
	d.WithLogs = false
	d.WithStdout = false
	d.LogLevel = "TRACE"
	d.BootNodePort = 24000
	d.Libp2pPort = 25000
	d.JsonRPCPort = 26000
	d.ValidatorCount = 20
	d.GenesisAssetCount = 0

	tm.bgClient = bg.NewBGClient(&client.Config{
		ClusterConfig: d,
		Network:       client.Local,
	})

	_, err := tm.bgClient.StartNetwork(context.Background())
	tm.Suite.NoError(err)

	// wait for node to start all modules
	time.Sleep(tests.DefaultLocalWaitTime)

	tm.moiClient, err = NewClient(fmt.Sprintf("http://localhost:%d", d.JsonRPCPort))
	tm.Suite.NoError(err)

	tm.instances, err = common.ReadInstancesFile(filepath.Join(d.TempDir, "instances.json"))
	tm.Suite.NoError(err)

	tm.accounts, err = tm.bgClient.Accounts(context.Background())
	tm.Suite.NoError(err)

	// a tesseract is generated to provide data for tesseract related api
	// fire and finalize ixn and store ix hash
	tm.ixHash = createAsset(tm.T(), tm.moiClient, tm.accounts[0].Addr, tm.accounts[0].Mnemonic)

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
			require.Greater(tm.T(), connResp.InboundConnCount, int64(0))
			require.Greater(tm.T(), connResp.OutboundConnCount, int64(0))
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
				PeerID: tests.GetTestPeerID(tm.T()).String(),
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
			require.Equal(tm.T(), tm.accounts[0].Addr, rpcIxn.Sender)
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
				Address: tm.accounts[0].Addr,
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
			require.Equal(tm.T(), tm.accounts[0].Addr, rpcIxn.Sender)
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
			expectedError: common.ErrGridHashNotFound,
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
				Address:     tm.accounts[0].Addr,
			},
		},
		{
			name: "failed to get logs",
			filterQueryArgs: &rpcargs.FilterQueryArgs{
				Address: tests.RandomAddress(tm.T()),
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
	ctx := context.Background()
	acc := tm.accounts[1]

	tsFilter, err := tm.moiClient.NewTesseractFilter(ctx, &rpcargs.TesseractFilterArgs{})
	require.NoError(tm.T(), err)

	tsByAccFilter, err := tm.moiClient.NewTesseractsByAccountFilter(ctx, &rpcargs.TesseractByAccountFilterArgs{
		Addr: acc.Addr,
	})
	require.NoError(tm.T(), err)

	logFilter, err := tm.moiClient.NewLogFilter(ctx, &websocket.LogQuery{
		Address: acc.Addr,
	})
	require.NoError(tm.T(), err)

	ixnsFilter, err := tm.moiClient.PendingIxnsFilter(ctx, &rpcargs.PendingIxnsFilterArgs{})
	require.NoError(tm.T(), err)

	// send create asset interaction
	ixHash := createAsset(tm.T(), tm.moiClient, acc.Addr, acc.Mnemonic)

	testcases := []struct {
		name             string
		filterQueryArgs  *rpcargs.FilterArgs
		subscriptionType rpcargs.SubscriptionType
		expectedError    error
	}{
		{
			name: "fetch ts from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: tsFilter.FilterID,
			},
			subscriptionType: rpcargs.NewTesseract,
		},
		{
			name: "fetch ts by acc from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: tsByAccFilter.FilterID,
			},
			subscriptionType: rpcargs.NewTesseractsByAccount,
		},
		{
			name: "fetch logs from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: logFilter.FilterID,
			},
			subscriptionType: rpcargs.NewLogsByFilter,
		},
		{
			name: "fetch ixns from filter successfully",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: ixnsFilter.FilterID,
			},
			subscriptionType: rpcargs.PendingIxns,
		},
		{
			name: "failed to fetch data as filter does not exist",
			filterQueryArgs: &rpcargs.FilterArgs{
				FilterID: "hello",
			},
			expectedError: errors.New("filter not found"),
		},
	}

	for _, test := range testcases {
		tm.Run(test.name, func() {
			resp, err := tm.moiClient.GetFilterChanges(ctx, test.filterQueryArgs, test.subscriptionType)

			if test.expectedError != nil {
				require.ErrorContains(tm.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(tm.T(), err)

			switch test.subscriptionType {
			case rpcargs.NewTesseract:
				rpcTS, ok := resp.([]*rpcargs.RPCTesseract)
				require.True(tm.T(), ok)
				require.Equal(tm.T(), 3, len(rpcTS))

				found := 0

				// make sure sender and sarga tesseracts are there in generated tesseracts
				for i := 0; i < 3; i++ {
					if rpcTS[i].Address() == acc.Addr || rpcTS[i].Address() == common.SargaAddress {
						found++
					}
				}

				require.Equal(tm.T(), found, 2)

			case rpcargs.NewTesseractsByAccount:
				rpcTS, ok := resp.([]*rpcargs.RPCTesseract)
				require.True(tm.T(), ok)
				require.Equal(tm.T(), 1, len(rpcTS))
				require.Equal(tm.T(), acc.Addr, rpcTS[0].Address())

			case rpcargs.PendingIxns:
				ixHashes, ok := resp.([]*common.Hash)
				require.True(tm.T(), ok)
				require.Equal(tm.T(), 1, len(ixHashes))
				require.Equal(tm.T(), ixHash, *ixHashes[0])

			case rpcargs.NewLogsByFilter:
				logs, ok := resp.([]*rpcargs.RPCLog)
				require.True(tm.T(), ok)
				require.Equal(tm.T(), 0, len(logs))
			}
		})
	}
}
