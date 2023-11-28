package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

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
		require.Equalf(t, uint64(0), response.NodeSyncResp.TotalPendingAccounts.ToUint64(), syncNode.URL())
	}
}

func checkIfAccountsSyncedOnAllNodes(
	t *testing.T,
	nodeToCheck *moiclient.Client,
	syncNodeSet []*moiclient.Client,
	addresses ...common.Address,
) {
	t.Helper()

	t.Log("total addresses ", len(addresses))
	t.Log("total nodes ", len(syncNodeSet))

	// compare all account sync status among all other nodes with this node
	for _, address := range addresses {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTime)

		expectedAccMetaInfo, err := nodeToCheck.AccountMetaInfo(ctx, &args.GetAccountArgs{
			Address: address,
		})
		require.NoError(t, err, nodeToCheck.URL(), address)

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

				require.Equalf(t, expectedAccMetaInfo.Height, actualAccMetaInfo.Height, errMsg)
			}()
		}

		wg.Wait()
	}
}

func (te *TestEnvironment) TestFullSyncForOneNode() {
	// as first node is operator, avoid stopping operator
	te.moiClient, te.moiClients[50] = te.moiClients[50], te.moiClient

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
				accs, err := te.chooseRandomUniqueAccounts(2)
				require.NoError(te.T(), err)

				sender := accs[0]
				initialAmount := big.NewInt(1000)

				// send ixn to new node as moiclient is down
				temp := te.moiClient
				te.moiClient = te.moiClients[1]

				// send an ixn to create new account
				// check if the newly created asset account and existing account are synced
				createAsset(te, sender, createAssetCreatePayload(
					tests.GetRandomUpperCaseString(te.T(), 8),
					initialAmount,
					common.MAS0,
					nil,
				))

				te.moiClient = temp
			},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultNodeStopTime)

			te.logger.Info("stop node", te.moiClient.URL())

			err := te.bgClient.StopNode(ctx, te.moiClient.URL())
			require.NoError(te.T(), err)

			cancel()

			if test.intervalFunction != nil {
				test.intervalFunction()
			}

			ctx, cancel = context.WithTimeout(context.Background(), DefaultNodeStartTime)

			te.logger.Info("start node", te.moiClient.URL())

			err = te.bgClient.StartNode(ctx, te.moiClient.URL(), test.withCleanDB)
			require.NoError(te.T(), err)

			cancel()

			time.Sleep(5 * time.Second)

			checkIfNodeSynced(te.T(), te.moiClient)
			checkIfNodesSynced(te.T(), te.moiClients)

			ctx, cancel = context.WithTimeout(context.Background(), DefaultQueryTime)
			defer cancel()

			addrs, err := te.moiClients[1].Accounts(ctx)
			require.NoError(te.T(), err)

			checkIfAccountsSyncedOnAllNodes(te.T(), te.moiClient, te.moiClients, addrs...)

			time.Sleep(2 * time.Second)
		})
	}
}
