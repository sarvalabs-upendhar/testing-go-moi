package decision

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/polo/go-polo"
	"testing"
	"time"
)

func TestGetAssociatedPeers_FetchFromCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMockDB()
	ledger := NewTestLedger(t, ctx, store)

	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ids := tests.GetTestKramaIDs(t, 1)

	pList := types.NewPeerList()
	pList.AddPeer(ids[0])

	ledger.cache.Add(GetAgoraKey(stateHash), pList)

	peers, err := ledger.GetAssociatedPeers(address, stateHash)
	require.NoError(t, err, err)
	require.Contains(t, peers, ids[0])
}

func TestGetAssociatedPeers_FetchFromDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMockDB()
	ledger := NewTestLedger(t, ctx, store)

	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ids := tests.GetTestKramaIDs(t, 1)

	pList := types.NewPeerList()
	pList.AddPeer(ids[0])

	// Write the list to db
	err := ledger.db.GetBatchWriter().Set(GetAgoraDBKey(address, stateHash), polo.Polorize(pList.CanonicalPeerList()))
	require.NoError(t, err)

	peers, err := ledger.GetAssociatedPeers(address, stateHash)
	require.NoError(t, err, err)
	require.Contains(t, peers, ids[0])
}

func TestUpdateAssociatedPeers_EntryAlreadyExists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMockDB()
	ledger := NewTestLedger(t, ctx, store)
	ledger.Start()

	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ids := tests.GetTestKramaIDs(t, 2)

	// Create an entry and add it to db
	pList := types.NewPeerList()
	pList.AddPeer(ids[0])

	// Write the list to db
	err := ledger.db.GetBatchWriter().Set(GetAgoraDBKey(address, stateHash), polo.Polorize(pList.CanonicalPeerList()))
	require.NoError(t, err)

	err = ledger.UpdateAssociatedPeers(address, stateHash, ids[1])
	require.NoError(t, err)

	time.Sleep(3 * time.Second) // wait for 3 seconds

	peerList, err := ledger.fetchFromDB(address, stateHash)
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[1], "peer not available in db")

	// fetch peer list from db
	peerList, err = ledger.fetchFromCache(GetAgoraKey(stateHash))
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[1], "peer not available in cache")
}

func TestUpdateAssociatedPeers_NewEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMockDB()
	ledger := NewTestLedger(t, ctx, store)
	ledger.Start()

	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ids := tests.GetTestKramaIDs(t, 1)

	err := ledger.UpdateAssociatedPeers(address, stateHash, ids[0])
	require.NoError(t, err)

	time.Sleep(3 * time.Second) // wait for 3 seconds

	// fetch peer list from db
	peerList, err := ledger.fetchFromDB(address, stateHash)
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[0], "peer not available in db")

	// fetch peer list from db
	peerList, err = ledger.fetchFromCache(GetAgoraKey(stateHash))
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[0], "peer not available in cache")
}

func NewTestLedger(t *testing.T, ctx context.Context, db ledgerStore) *Ledger {
	t.Helper()

	ledger, err := NewLedger(ctx, hclog.NewNullLogger(), 1, db)
	require.NoError(t, err)

	return ledger
}
