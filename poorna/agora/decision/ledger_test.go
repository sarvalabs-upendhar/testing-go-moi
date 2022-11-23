package decision

import (
	"context"
	"testing"
	"time"

	"gitlab.com/sarvalabs/moichain/dhruva"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/polo/go-polo"
)

func TestGetAssociatedPeers_FetchFromCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ledger := NewTestLedger(t, ctx)

	address := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	ids := tests.GetTestKramaIDs(t, 1)

	pList := atypes.NewPeerList()
	pList.AddPeer(ids[0])

	ledger.cache.Add(GetAgoraKey(stateHash.Key()), pList)

	peers, err := ledger.GetAssociatedPeers(address, stateHash)
	require.NoError(t, err, err)
	require.Contains(t, peers, ids[0])
}

func TestGetAssociatedPeers_FetchFromDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ledger := NewTestLedger(t, ctx)

	address := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	ids := tests.GetTestKramaIDs(t, 1)

	pList := atypes.NewPeerList()
	pList.AddPeer(ids[0])

	// Write the list to db
	err := ledger.db.GetBatchWriter().Set(
		GetAgoraDBKey(address, stateHash.Key()),
		polo.Polorize(pList.CanonicalPeerList()),
	)
	require.NoError(t, err)

	peers, err := ledger.GetAssociatedPeers(address, stateHash)
	require.NoError(t, err, err)
	require.Contains(t, peers, ids[0])
}

func TestUpdateAssociatedPeers_EntryAlreadyExists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ledger := NewTestLedger(t, ctx)
	ledger.Start()

	address := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	ids := tests.GetTestKramaIDs(t, 2)

	// Create an entry and add it to db
	pList := atypes.NewPeerList()
	pList.AddPeer(ids[0])

	// Write the list to db
	err := ledger.db.GetBatchWriter().Set(
		GetAgoraDBKey(address, stateHash.Key()),
		polo.Polorize(pList.CanonicalPeerList()),
	)
	require.NoError(t, err)

	err = ledger.UpdateAssociatedPeers(address, stateHash, ids[1])
	require.NoError(t, err)

	time.Sleep(3 * time.Second) // wait for 3 seconds

	peerList, err := ledger.fetchFromDB(address, stateHash)
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[1], "peer not available in db")

	// fetch peer list from db
	peerList, err = ledger.fetchFromCache(GetAgoraKey(stateHash.Key()))
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[1], "peer not available in cache")
}

func TestUpdateAssociatedPeers_NewEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ledger := NewTestLedger(t, ctx)
	ledger.Start()

	address := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
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
	peerList, err = ledger.fetchFromCache(GetAgoraKey(stateHash.Key()))
	require.NoError(t, err)

	// check for the added peer
	require.Contains(t, peerList.Peers(), ids[0], "peer not available in cache")
}

func NewTestLedger(t *testing.T, ctx context.Context) *Ledger {
	t.Helper()

	ledger, err := NewLedger(ctx, hclog.NewNullLogger(), 1, NewMockDB())
	require.NoError(t, err)

	return ledger
}
