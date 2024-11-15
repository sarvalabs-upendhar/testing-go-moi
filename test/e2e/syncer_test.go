package e2e

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

func checkIfNodeSynced(t *testing.T, nodeToCheck *moiclient.Client) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), InitialSyncTime)
	defer cancel()

	_, err := tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)
		defer cancel()

		response, err := nodeToCheck.Syncing(ctx, &args.SyncStatusRequest{})
		require.NoError(t, err)

		if !response.NodeSyncResp.IsInitialSyncDone {
			return nil, true
		}

		if response.NodeSyncResp.TotalPendingAccounts.ToUint64() != uint64(0) {
			return nil, true
		}

		return nil, false
	})
	require.NoError(t, err, nodeToCheck.URL())

	t.Log("nodes synced ")
}

func checkIfNodesSynced(t *testing.T, syncNodeSet []*moiclient.Client) {
	t.Helper()

	for _, syncNode := range syncNodeSet {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)

		response, err := syncNode.Syncing(ctx, &args.SyncStatusRequest{})
		require.NoError(t, err)

		cancel()

		require.Equalf(t, true, response.NodeSyncResp.IsInitialSyncDone, syncNode.URL())
		require.Equalf(t,
			uint64(0),
			response.NodeSyncResp.TotalPendingAccounts.ToUint64(),
			syncNode.URL(),
			time.Now().String(),
		)
	}
}

func checkIfAccountsSyncedOnAllNodes(
	t *testing.T,
	nodeToCheck *moiclient.Client,
	syncNodeSet []*moiclient.Client,
	addresses ...identifiers.Address,
) {
	t.Helper()

	t.Log("total addresses ", len(addresses))
	t.Log("total nodes in network", len(syncNodeSet))

	// compare all account sync status among all other nodes with this node
	for _, address := range addresses {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)

		expectedAccMetaInfo, err := nodeToCheck.AccountMetaInfo(ctx, &args.GetAccountArgs{
			Address: address,
		})
		require.NoError(t, err, nodeToCheck.URL(), address, time.Now())

		cancel()

		var wg sync.WaitGroup

		wg.Add(len(syncNodeSet))

		// compare account sync status among all other nodes with this node
		for _, syncNode := range syncNodeSet {
			errMsg := fmt.Sprintln(address, nodeToCheck.URL(), syncNode.URL())

			syncNode := syncNode

			go func() {
				defer wg.Done()

				num := tests.GetRandomNumber(t, 1000)
				time.Sleep(time.Duration(num) * time.Millisecond)

				ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)

				actualAccMetaInfo, err := syncNode.AccountMetaInfo(ctx, &args.GetAccountArgs{
					Address: address,
				})
				require.NoErrorf(t, err, errMsg)

				cancel()

				require.Equalf(t, expectedAccMetaInfo.Height, actualAccMetaInfo.Height, errMsg, time.Now())
			}()
		}

		wg.Wait()
	}
}

func (te *TestEnvironment) chooseNonContextNodeURL(addrs []identifiers.Address) string {
	contextNodes := moiclient.GetContextNodes(te.T(), te.moiClient, addrs)

	hasNode := func(node string) bool {
		for _, n := range contextNodes {
			if n == node {
				return true
			}
		}

		return false
	}

	operatorPort := strconv.Itoa(DefaultJSONRPCPort)

	// avoid choosing operator and context nodes as node will be stopped
	for _, instance := range te.instances {
		if !hasNode(instance.KramaID) && !strings.HasSuffix(instance.RPCUrl, operatorPort) {
			parts := strings.Split(instance.RPCUrl, ":")

			return "http://0.0.0.0:" + parts[1]
		}
	}

	return ""
}

func (te *TestEnvironment) TestZFullSyncForOneNode() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	var (
		sender     = accs[0]
		chosenNode *moiclient.Client
	)

	// As cloud tests doesn't run in debug mode, we can choose any node other than operator
	if *networkType == cloud {
		chosenNode, err = getMoiClient(te.T(), te.moiClients[1].URL())
		require.NoError(te.T(), err)
	} else {
		// choose node that need to be stopped, it should be neither operator nor context node of ixn participants
		chosenNode, err = getMoiClient(
			te.T(),
			te.chooseNonContextNodeURL([]identifiers.Address{sender.Addr, common.SargaAddress}),
		)
		require.NoError(te.T(), err)
	}

	createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		big.NewInt(1),
		common.MAS0,
		nil,
	))

	testcases := []struct {
		name             string
		withCleanDB      bool
		intervalFunction func() // actions to perform after bringing down node
	}{
		{
			name:        "start the node with clean db",
			withCleanDB: true,
		},
		{
			name: "start the node without clean db",
			intervalFunction: func() {
				initialAmount := big.NewInt(1000)

				// send an ixn to create new account
				// check if the newly created asset account and existing account are synced
				createAsset(te, sender, createAssetCreatePayload(
					tests.GetRandomUpperCaseString(te.T(), 8),
					initialAmount,
					common.MAS0,
					nil,
				))
			},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultNodeStopTime)

			te.logger.Debug("Stop node", chosenNode.URL(), time.Now())

			err := te.bgClient.StopNode(ctx, chosenNode.URL())
			require.NoError(te.T(), err)

			cancel()

			if test.intervalFunction != nil {
				test.intervalFunction()
			}

			ctx, cancel = context.WithTimeout(context.Background(), DefaultNodeStartTime)

			te.logger.Debug("Start node", chosenNode.URL(), time.Now())

			err = te.bgClient.StartNode(ctx, chosenNode.URL(), test.withCleanDB)
			require.NoError(te.T(), err)

			cancel()

			// TODO Replace with wait for initial sync time code
			time.Sleep(10 * time.Second)

			checkIfNodeSynced(te.T(), chosenNode)
			checkIfNodesSynced(te.T(), te.moiClients)

			ctx, cancel = context.WithTimeout(context.Background(), DefaultQueryTime)
			defer cancel()

			addrs, err := te.moiClient.Accounts(ctx)
			require.NoError(te.T(), err)

			checkIfAccountsSyncedOnAllNodes(te.T(), te.moiClient, te.moiClients, addrs...)

			time.Sleep(2 * time.Second)
		})
	}
}
