package forage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"

	"github.com/sarvalabs/go-moi/syncer/agora/block"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/syncer"
	"github.com/sarvalabs/go-moi/syncer/cid"
	"github.com/stretchr/testify/require"
)

// common format for all tests
// 1. create one or multiple server and one client which can sync from servers
// 2. instantiate database for client and servers
// 3. instantiate syncer client and syncer servers using above generated servers
// 4. generate system accounts and normal accounts tesseracts with custom heights
// 5. store these tesseracts on servers or client according to the test scenario
// 6. start syncer servers and syncer clients
// 7. define and listen for expected events emitted by client during syncing
// 8. check if tesseracts got synced by checking the data in db

// TestFullSync checks if client syncs system accounts, normal accounts through system account sync,
// bucket sync, snap sync, tesseract sync and jobs are done
func TestFullSync(t *testing.T) {
	t.Parallel()

	clientCtx, clientCancel := context.WithTimeout(context.Background(), maxTimeout)
	defer clientCancel()

	serverCtx, serverCancel := context.WithTimeout(context.Background(), maxTimeout)
	defer serverCancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)
	connectClientToServers(t, servers[0], servers[1])
	connectClientToServers(t, servers[1], servers[0])

	clientPM, _ := createPersistenceManager(t, clientCtx)
	serverPM, _ := createPersistenceManager(t, serverCtx)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	clientSyncer := NewTestSyncer(
		clientCtx,
		defaultSyncerConfig(),
		servers[0],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM,
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	serverSyncer := NewTestSyncer(
		serverCtx,
		defaultSyncerConfig(),
		servers[1],
		&utils.TypeMux{},
		newMockAgora(),
		serverPM,
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 2)

	accountsToSync := map[identifiers.Address]int{
		common.GuardianLogicAddr: 4,
		common.SargaAddress:      4,
		addresses[0]:             4,
		addresses[1]:             4,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)

	clientSyncer.logger.Info(string(servers[0].GetKramaID()))
	serverSyncer.logger.Info(string(servers[1].GetKramaID()))

	serverSyncer.setInitialSyncDone(true)

	err := serverSyncer.Start(1)
	require.NoError(t, err)

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
	checkJobQueue(t, clientSyncer, serverSyncer)
}

// TestFullSync_TrustedPeers ensures that the client syncs only from trusted peers.
// A trusted peer is the only node that possesses all tesseracts.
// If the client syncs from nodes other than the trusted peer, syncing will fail.
func TestFullSync_TrustedPeers(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		4,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
		2: defaultConfig[2],
		3: defaultConfig[3],
	}

	servers := createMultipleServers(t, 4, paramsMap)

	for i := 0; i < 3; i++ {
		connectClientToServers(t, servers[3], servers[i])
		connectClientToServers(t, servers[i], servers[3])
	}

	pm, _ := createPersistenceManagers(t, ctx, 4)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	serverSyncers := make([]*Syncer, 3)

	for i := 0; i < 3; i++ {
		serverSyncers[i] = NewTestSyncer(
			ctx,
			defaultSyncerConfig(),
			servers[i],
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	clientSyncer := NewTestSyncer(
		ctx,
		&config.SyncerConfig{
			ShouldExecute:  false,
			SyncMode:       config.DefaultSyncMode,
			EnableSnapSync: true,
			TrustedPeers: []config.NodeInfo{
				{
					ID: servers[0].GetKramaID(),
				},
			},
		},
		servers[3],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[3],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	addr := tests.RandomAddress(t)

	accountsToSync := map[identifiers.Address]int{
		common.GuardianLogicAddr: 10,
		common.SargaAddress:      10,
		addr:                     10,
	}

	ts1 := generateTesseracts(t, 0, accountsToSync[common.GuardianLogicAddr], common.NilHash, common.GuardianLogicAddr)
	ts2 := generateTesseracts(t, 0, accountsToSync[common.SargaAddress], common.NilHash, common.SargaAddress)
	ts3 := generateTesseracts(t, 0, accountsToSync[addr], common.NilHash, addr)

	storeTesseractsInDB(t, serverSyncers[0], ts1...)
	storeTesseractsInDB(t, serverSyncers[0], ts2...)
	storeTesseractsInDB(t, serverSyncers[0], ts3...)

	storeTesseractsInDB(t, serverSyncers[1], ts1[0:3]...)
	storeTesseractsInDB(t, serverSyncers[1], ts2[0:2]...)
	storeTesseractsInDB(t, serverSyncers[1], ts3[0:4]...)

	storeTesseractsInDB(t, serverSyncers[2], ts1[0:3]...)
	storeTesseractsInDB(t, serverSyncers[2], ts2[0:2]...)
	storeTesseractsInDB(t, serverSyncers[2], ts3[0:4]...)

	clientSyncer.logger.Info(string(servers[3].GetKramaID()))
	serverSyncers[0].logger.Info(string(servers[0].GetKramaID()))
	serverSyncers[1].logger.Info(string(servers[1].GetKramaID()))
	serverSyncers[2].logger.Info(string(servers[2].GetKramaID()))

	for i := 0; i < 3; i++ {
		serverSyncers[i].setInitialSyncDone(true)

		err := serverSyncers[i].Start(1)
		require.NoError(t, err)
	}

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts1...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts2...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts3...)
	checkJobQueue(t, clientSyncer)
	checkJobQueue(t, serverSyncers...)
}

// TestFullSync_ChooseBestPeer, makes sure that the client syncs to the highest heights for all accounts,
// even when not all servers have the latest state of all accounts.
func TestFullSync_ChooseBestPeer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		4,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
		2: defaultConfig[2],
		3: defaultConfig[3],
	}

	servers := createMultipleServers(t, 4, paramsMap)

	for i := 0; i < 3; i++ {
		connectClientToServers(t, servers[3], servers[i])
		connectClientToServers(t, servers[i], servers[3])
	}

	pm, _ := createPersistenceManagers(t, ctx, 4)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	serverSyncers := make([]*Syncer, 3)

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[3],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[3],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	for i := 0; i < 3; i++ {
		serverSyncers[i] = NewTestSyncer(
			ctx,
			defaultSyncerConfig(),
			servers[i],
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)

	accountsToSync := map[identifiers.Address]int{
		common.GuardianLogicAddr: 10,
		common.SargaAddress:      10,
		addr:                     10,
	}

	ts1 := generateTesseracts(t, 0, accountsToSync[common.GuardianLogicAddr], common.NilHash, common.GuardianLogicAddr)
	ts2 := generateTesseracts(t, 0, accountsToSync[common.SargaAddress], common.NilHash, common.SargaAddress)
	ts3 := generateTesseracts(t, 0, accountsToSync[addr], common.NilHash, addr)

	storeTesseractsInDB(t, serverSyncers[0], ts1...)
	storeTesseractsInDB(t, serverSyncers[0], ts2...)
	storeTesseractsInDB(t, serverSyncers[0], ts3[0:7]...)

	storeTesseractsInDB(t, serverSyncers[1], ts1[0:3]...)
	storeTesseractsInDB(t, serverSyncers[1], ts2...)
	storeTesseractsInDB(t, serverSyncers[1], ts3...)

	storeTesseractsInDB(t, serverSyncers[2], ts1...)
	storeTesseractsInDB(t, serverSyncers[2], ts2[0:2]...)
	storeTesseractsInDB(t, serverSyncers[2], ts3...)

	clientSyncer.logger.Info(string(servers[3].GetKramaID()))
	serverSyncers[0].logger.Info(string(servers[0].GetKramaID()))
	serverSyncers[1].logger.Info(string(servers[1].GetKramaID()))
	serverSyncers[2].logger.Info(string(servers[2].GetKramaID()))

	for i := 0; i < 3; i++ {
		serverSyncers[i].setInitialSyncDone(true)

		err := serverSyncers[i].Start(1)
		require.NoError(t, err)
	}

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts1...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts2...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts3...)
	checkJobQueue(t, clientSyncer)
	checkJobQueue(t, serverSyncers...)
}

// TestSync_FromBroadcastedTesseract checks if client can sync from tesseract broadcast by server
// without executing tesseract using agora
func TestSync_FromBroadcastedTesseract(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)
	connectClientToServers(t, servers[0], servers[1])
	connectClientToServers(t, servers[1], servers[0])

	pm, _ := createPersistenceManagers(t, ctx, 2)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[0],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	serverSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[1],
		&utils.TypeMux{},
		newMockAgora(),
		pm[1],
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)

	clientSyncer.logger.Info(string(servers[0].GetKramaID()))
	serverSyncer.logger.Info(string(servers[1].GetKramaID()))

	clientSyncer.logger.Info(string(servers[0].GetKramaID()))
	serverSyncer.logger.Info(string(servers[1].GetKramaID()))

	serverSyncer.setInitialSyncDone(true)
	clientSyncer.setInitialSyncDone(true)

	err := serverSyncer.Start(1)
	require.NoError(t, err)

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	time.Sleep(600 * time.Millisecond)

	accountsToSync := map[identifiers.Address]int{
		addresses[2]: 3,
	}

	newTesseracts := generateTesseracts(t, 0, 3, common.NilHash, addresses[2])

	clientSyncer.agora = newMockAgora()
	storeTesseractsInSession(t, clientSyncer, newTesseracts...)
	broadcastTesseracts(t, clientSyncer, serverSyncer, newTesseracts...)

	expectedEvents := SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
	checkJobQueue(t, clientSyncer, serverSyncer)
}

// TestSync_FromRejoining checks When client restarted, sarga tesseracts should sync through agora
// and normal account tesseracts should sync through snap sync
func TestSync_FromRejoining(t *testing.T) {
	t.Parallel()

	clientCtx, clientCancel := context.WithTimeout(context.Background(), maxTimeout)

	serverCtx, serverCancel := context.WithTimeout(context.Background(), maxTimeout)
	defer serverCancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)
	connectClientToServers(t, servers[0], servers[1])
	connectClientToServers(t, servers[1], servers[0])

	clientPM, clientDir := createPersistenceManagers(t, clientCtx, 1)
	serverPM, _ := createPersistenceManagers(t, clientCtx, 1)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	clientSyncer := NewTestSyncer(
		clientCtx,
		defaultSyncerConfig(),
		servers[0],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM[0],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	serverSyncer := NewTestSyncer(
		serverCtx,
		defaultSyncerConfig(),
		servers[1],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		serverPM[0],
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)
	accountsToSync := map[identifiers.Address]int{
		common.GuardianLogicAddr: 4,
		common.SargaAddress:      4,
		addresses[0]:             4,
		addresses[1]:             4,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)
	storeTesseractsInDB(t, clientSyncer, ts...)

	clientSyncer.logger.Info(string(servers[0].GetKramaID()))
	serverSyncer.logger.Info(string(servers[1].GetKramaID()))

	serverSyncer.setInitialSyncDone(true)
	clientSyncer.setInitialSyncDone(true)

	err := serverSyncer.Start(1)
	require.NoError(t, err)

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	time.Sleep(600 * time.Millisecond)

	clientCancel()
	clientPM[0].Close()

	err = clientSyncer.network.Unsubscribe(config.TesseractTopic)
	require.NoError(t, err)

	clientCtx, clientCancel = context.WithTimeout(context.Background(), maxTimeout)
	defer clientCancel()

	accountsToSync = map[identifiers.Address]int{
		addresses[2]:        4,
		common.SargaAddress: 8,
	}

	metaInfo, err := serverSyncer.db.GetAccountMetaInfo(common.SargaAddress)
	require.NoError(t, err)

	sargaTesseracts := generateTesseracts(t, int(metaInfo.Height+1), 8, metaInfo.TesseractHash, common.SargaAddress)

	newTesseracts := generateTesseracts(t, 0, 4, common.NilHash, addresses[2])
	storeTesseractsInDB(t, serverSyncer, newTesseracts...)
	storeTesseractsInDB(t, serverSyncer, sargaTesseracts...)

	clientDB, err := storage.NewPersistenceManager(hclog.NewNullLogger(), &config.DBConfig{
		CleanDB:      false,
		DBFolderPath: clientDir[0],
		MaxSnapSize:  1073741824,
	}, db.NilMetrics())
	require.NoError(t, err)

	client := createMultipleServers(t, 1, paramsMap)
	connectClientToServers(t, client[0], servers[1])
	connectClientToServers(t, servers[1], client[0])

	clientSyncer = NewTestSyncer(
		clientCtx,
		defaultSyncerConfig(),
		client[0],
		&utils.TypeMux{},
		newMockAgora(),
		clientDB,
		types.NewSlots(2, 3),
		"CLIENT-RESTART",
		nil,
	)

	storeTesseractsInSession(t, clientSyncer, sargaTesseracts...)

	mockSession := newMockSession(addresses[2])
	mockAgora, ok := clientSyncer.agora.(*MockAgora)
	require.True(t, ok)

	for _, ts := range newTesseracts {
		mockAgora.addSession(mockSession, cid.AccountCID(ts.StateHash(addresses[2])))
	}

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts: map[identifiers.Address]AccountSpecificEvents{
			common.SargaAddress: newAccountSpecificEvents(0, 1, 5, 8),
			addresses[2]:        newAccountSpecificEvents(1, 1, 0, 4),
		},
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)

	newTesseracts = append(newTesseracts, sargaTesseracts...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
	checkJobQueue(t, clientSyncer, serverSyncer)
}

// TestSync_ThroughExecution checks if client can sync from tesseract broadcast by server
// by executing tesseract
func TestSync_ThroughExecution(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)
	connectClientToServers(t, servers[0], servers[1])
	connectClientToServers(t, servers[1], servers[0])

	pm, _ := createPersistenceManagers(t, ctx, 2)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	clientSyncer := NewTestSyncer(
		ctx,
		&config.SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       config.DefaultSyncMode,
			EnableSnapSync: true,
		},
		servers[0],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	serverSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[1],
		&utils.TypeMux{},
		newMockAgora(),
		pm[1],
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	clientSyncer.logger.Info(string(servers[0].GetKramaID()))
	serverSyncer.logger.Info(string(servers[1].GetKramaID()))

	serverSyncer.setInitialSyncDone(true)
	clientSyncer.setInitialSyncDone(true)

	err := serverSyncer.Start(1)
	require.NoError(t, err)

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	time.Sleep(600 * time.Millisecond)

	addresses := tests.GetAddresses(t, 3)
	accountsToSync := map[identifiers.Address]int{
		addresses[0]: 4,
		addresses[1]: 4,
		addresses[2]: 4,
	}

	newTesseracts := generateTesseractsGridByMap(t, accountsToSync)
	broadcastTesseracts(t, clientSyncer, serverSyncer, newTesseracts...)

	expectedEvents := SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, true, newTesseracts...)
	checkJobQueue(t, clientSyncer, serverSyncer)
}

// TestFullSync_RemoveBestPeer, makes sure that the client syncs to the highest heights for all accounts,
// even when two server doesn't have lattice even though it has the latest account meta info and snap data
func TestFullSync_RemoveBestPeer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		4,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
		2: defaultConfig[2],
	}

	servers := createMultipleServers(t, 3, paramsMap)

	for i := 0; i < 2; i++ {
		connectClientToServers(t, servers[2], servers[i])
		connectClientToServers(t, servers[i], servers[2])
	}

	pm, _ := createPersistenceManagers(t, ctx, 3)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	serverSyncers := make([]*Syncer, 2)

	mux := &utils.TypeMux{}
	jq := &JobQueue{
		jobs:  make(map[identifiers.Address]*SyncJob),
		mux:   mux,
		krama: NewMockKramaEngine(nil, nil),
	}

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[2],
		mux,
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[2],
		types.NewSlots(2, 3),
		"CLIENT",
		func(s *Syncer) {
			s.jobQueue = jq
		},
	)

	for i := 0; i < 2; i++ {
		serverSyncers[i] = NewTestSyncer(
			ctx,
			defaultSyncerConfig(),
			servers[i],
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)

	accountsToSync := map[identifiers.Address]int{
		common.GuardianLogicAddr: 1,
		common.SargaAddress:      1,
		addr:                     1,
	}

	ts := generateTesseractsByMap(t, accountsToSync)

	storeTesseractsInDB(t, serverSyncers[0], ts...)

	storeAccountMetaInfosAndSnapInDB(t, serverSyncers[1], ts...)

	clientSyncer.logger.Info(string(servers[2].GetKramaID()))
	serverSyncers[0].logger.Info(string(servers[0].GetKramaID()))
	serverSyncers[1].logger.Info(string(servers[1].GetKramaID()))

	serverSyncers[1].setInitialSyncDone(true)

	err := serverSyncers[1].Start(1)
	require.NoError(t, err)

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		expectedEvents := SyncEvents{
			bucketSync:    1,
			SystemAccSync: 1,
			accounts:      make(map[identifiers.Address]AccountSpecificEvents),
		}

		for addr, height := range accountsToSync {
			expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
		}

		SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	}()

	// start the server that has lattices later as we need to simulate removal of best peers in client
	// and when this is started, lattices will be available on this and client should complete accounts syncing
	_, err = tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
		job, ok := jq.getJob(common.GuardianLogicAddr)
		if !ok {
			return nil, true
		}

		if job.bestPeerLen() != 0 {
			return nil, true
		}

		return nil, false
	})
	require.NoError(t, err)

	serverSyncers[0].setInitialSyncDone(true)

	// start the server-0 and it has lattices
	err = serverSyncers[0].Start(1)
	require.NoError(t, err)

	wg.Wait()

	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
	checkJobQueue(t, clientSyncer)
	checkJobQueue(t, serverSyncers...)
}

// TestJobProcessor makes sure sync tesseract routine not blocked
// server-0 has tesseracts for address-a1 from 0-7
// server-1 has tesseracts for address-a1 from 0-11
// create new sync job on client for address-a1 with best peer as server-0
// client syncs from server-0 for tesseracts(0-7)
// server-0 throws error that it doesn't have other tesseracts (8-11)
// so client still has some tesseracts to add, and it returns early from lattice sync
// so job shouldn't be blocked here on client instead it should make progress and sync from server-1
func TestJobProcessor_checkSyncTesseractNotBlocked(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
	defer cancel()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		3,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
		2: defaultConfig[2],
	}

	servers := createMultipleServers(t, 3, paramsMap)

	for i := 0; i < 2; i++ {
		connectClientToServers(t, servers[2], servers[i])
		connectClientToServers(t, servers[i], servers[2])
	}

	pm, _ := createPersistenceManagers(t, ctx, 3)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	serverSyncers := make([]*Syncer, 2)

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[2],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[2],
		types.NewSlots(2, 3),
		"CLIENT",
		nil,
	)

	for i := 0; i < 2; i++ {
		serverSyncers[i] = NewTestSyncer(
			ctx,
			defaultSyncerConfig(),
			servers[i],
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)
	expectedHeight := 12
	accountsToSync := map[identifiers.Address]int{
		addr: expectedHeight,
	}

	ts := generateTesseracts(t, 0, accountsToSync[addr], common.NilHash, addr)

	storeTesseractsInDB(t, serverSyncers[0], ts[:8]...)
	storeTesseractsInDB(t, serverSyncers[1], ts...)

	clientSyncer.agora = newMockAgora()
	storeTesseractsInSession(t, clientSyncer, ts...)

	clientSyncer.logger.Info(string(servers[2].GetKramaID()))
	serverSyncers[0].logger.Info(string(servers[0].GetKramaID()))
	serverSyncers[1].logger.Info(string(servers[1].GetKramaID()))

	clientSyncer.setInitialSyncDone(true)

	for i := 0; i < 2; i++ {
		serverSyncers[i].setInitialSyncDone(true)

		err := serverSyncers[i].Start(1)
		require.NoError(t, err)
	}

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	err = clientSyncer.NewSyncRequest(
		addr,
		uint64(expectedHeight),
		common.FullSync,
		[]kramaid.KramaID{servers[0].GetKramaID()},
		false,
	)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		accounts: make(map[identifiers.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
	checkJobQueue(t, clientSyncer)
	checkJobQueue(t, serverSyncers...)
}

func TestSyncJobFromCanonicalInfo(t *testing.T) {
	t.Parallel()

	count := 1
	addrs := tests.GetAddresses(t, count)
	pm, _ := createPersistenceManager(t, context.Background())

	jobs := createSyncJobs(
		t,
		count,
		addrs,
		WithDB(pm),
		WithSnapDownloaded(false),
		WithExpectedHeight(8),
		WithCurrentHeight(2),
		WithJobState(Sleep),
		WithTesseractQueue(NewTesseractQueue()),
		WithLatticeSyncInProgress(true),
	)

	for i := 0; i < count; i++ {
		err := jobs[i].commitJob()
		require.NoError(t, err)
	}

	accountSyncInfos, err := pm.GetAccountsSyncStatus()
	require.NoError(t, err)

	syncJob, err := SyncJobFromCanonicalInfo(hclog.NewNullLogger(), pm, accountSyncInfos[0])
	require.NoError(t, err)

	checkIfSyncJobMatches(t, jobs[0], syncJob)
}

func TestPendingAccounts_AddJob(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mux := utils.TypeMux{}
	sub := mux.Subscribe(utils.PendingAccountEvent{})

	syncStatusTracker := NewSyncStatusTracker(0)
	go syncStatusTracker.StartSyncStatusTracker(ctx, sub)

	count := 3
	addrs := tests.GetAddresses(t, count)

	jobs := createSyncJobs(
		t,
		count,
		addrs,
		WithJobState(Done),
	)

	jobQueue := NewJobQueue(&mux, NewMockKramaEngine(nil, nil))

	for i := 0; i < count; i++ {
		err := jobQueue.AddJob(jobs[i])
		require.NoError(t, err)
	}

	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		pendingAccounts := syncStatusTracker.ReadPendingAccounts()
		if pendingAccounts != uint64(count) {
			return nil, true
		}

		pendingAddrs := jobQueue.GetPendingAccounts()
		sortAddresses(addrs)
		sortAddresses(pendingAddrs)

		require.Equal(t, addrs, pendingAddrs)

		return nil, false
	})
	require.NoError(t, err)
}

func TestPendingAccounts_RemoveJob(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mux := utils.TypeMux{}
	sub := mux.Subscribe(utils.PendingAccountEvent{})

	jobQueue := NewJobQueue(&mux, NewMockKramaEngine(nil, nil))

	syncStatusTracker := NewSyncStatusTracker(0)
	go syncStatusTracker.StartSyncStatusTracker(ctx, sub)

	count := 3
	addrs := tests.GetAddresses(t, count)

	jobs := createSyncJobs(
		t,
		count,
		addrs,
		WithDB(NewMockDB()),
		WithLogger(hclog.NewNullLogger()),
		WithJobState(Done),
		WithTesseractQueue(NewTesseractQueue()),
	)

	for i := 0; i < count; i++ {
		err := jobs[i].commitJob()
		require.NoError(t, err)
	}

	for i := 0; i < count; i++ {
		err := jobQueue.AddJob(jobs[i])
		require.NoError(t, err)
	}

	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		pendingAccounts := syncStatusTracker.ReadPendingAccounts()
		if pendingAccounts != uint64(count) {
			return nil, true
		}

		return nil, false
	})
	require.NoError(t, err)

	jobQueue.NextJob()

	_, err = tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		pendingAccounts := syncStatusTracker.ReadPendingAccounts()
		if pendingAccounts != uint64(0) {
			return nil, true
		}

		pendingAddrs := jobQueue.GetPendingAccounts()
		if len(pendingAddrs) != 0 {
			return nil, true
		}

		return nil, false
	})
	require.NoError(t, err)
}

func TestGetSyncJobInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mux := &utils.TypeMux{}
	s := NewTestSyncerWithJobQueue(ctx, mux)

	count := 1
	addrs := tests.GetAddresses(t, 2)
	bestPeers := make(map[kramaid.KramaID]struct{})
	testKramaIDs := tests.RandomKramaIDs(t, 1)
	bestPeers[testKramaIDs[0]] = struct{}{}

	jobs := createSyncJobs(
		t,
		count,
		addrs,
		WithJobState(Done),
		WithSnapDownloaded(true),
		WithCurrentHeight(0),
		WithExpectedHeight(1),
		WithLatticeSyncInProgress(true),
		WithTesseractQueue(NewTesseractQueue()),
		WithBestPeers(bestPeers),
	)

	err := s.jobQueue.AddJob(jobs[0])
	require.NoError(t, err)

	testcases := []struct {
		name          string
		addr          identifiers.Address
		expectedError error
	}{
		{
			name: "sync job exits for given address",
			addr: addrs[0],
		},
		{
			name:          "sync job doesn't exits for given address",
			addr:          addrs[1],
			expectedError: common.ErrSyncJobNotFound,
		},
		{
			name:          "nil address, return invalid address error",
			addr:          identifiers.NilAddress,
			expectedError: common.ErrInvalidAddress,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			resp, err := s.GetSyncJobInfo(test.addr)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.Equal(t, common.LatestSync.String(), resp.SyncMode)
			require.Equal(t, true, resp.SnapDownloaded)
			require.Equal(t, uint64(1), resp.ExpectedHeight)
			require.Equal(t, uint64(0), resp.CurrentHeight)
			require.Equal(t, Done.String(), resp.JobState)
			require.Greater(t, time.Now(), resp.LastModifiedAt)
			require.Equal(t, uint64(0), resp.TesseractQueueLen)
			require.Equal(t, true, resp.LatticeSyncInProgress)
			require.Equal(t, testKramaIDs, resp.BestPeers)
		})
	}
}

func TestTesseractValidator(t *testing.T) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), maxTimeout)
	defer cancelFunc()

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		1,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
	}

	server := createMultipleServers(t, 1, paramsMap)
	pm, _ := createPersistenceManager(t, ctx)
	sm := newMockStateManager(nil)

	s := NewTestSyncerForValidation(
		ctx,
		defaultSyncerConfig(),
		server[0],
		pm,
		hclog.NewNullLogger(),
		sm,
	)

	testTesseract := tests.CreateTesseract(t, nil)

	// polorize canonical tesseract
	rawTS, err := testTesseract.Canonical().Bytes()
	require.NoError(t, err)

	kramaID := s.network.GetKramaID()
	peerID, err := kramaID.DecodedPeerID()
	require.NoError(t, err)

	from, err := peerID.Marshal()
	require.NoError(t, err)

	// create dummy ICS cluster info
	info := common.ICSClusterInfo{
		RandomSet: []kramaid.KramaID{kramaID},
	}

	// polorize ICS cluster info
	rawInfo, err := info.Bytes()
	require.NoError(t, err)

	tsMsg := message.TesseractMsg{
		RawTesseract: rawTS,
		IxnsHashes:   tests.GetHashes(t, 1),
		Extra: map[string][]byte{
			testTesseract.ICSHash().String(): rawInfo,
		},
	}

	// polorize tesseract message
	rawData, err := tsMsg.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name             string
		msgData          []byte
		from             []byte
		preTestFn        func(tesseract *common.Tesseract)
		postTestFn       func(tesseract *common.Tesseract)
		expectedResponse pubsub.ValidationResult
		expectedError    string
	}{
		{
			name:    "Valid pubsub message",
			msgData: rawData,
			preTestFn: func(tessract *common.Tesseract) {
				sm.setTSSeal(tessract)
			},
			postTestFn: func(tesseract *common.Tesseract) {
				sm.removeTSSeal(tesseract)
			},
			expectedResponse: pubsub.ValidationAccept,
		},
		{
			name:             "Local pubsub message",
			from:             from,
			expectedResponse: pubsub.ValidationAccept,
		},
		{
			name:             "Nil pubsub message",
			msgData:          nil,
			expectedResponse: pubsub.ValidationReject,
			expectedError:    "failed to depolorize tesseract message",
		},
		{
			name:             "Failed to depolorize tesseract-message",
			msgData:          []byte{1},
			expectedResponse: pubsub.ValidationReject,
			expectedError:    "failed to depolorize tesseract message",
		},
		{
			name:             "Failed to verify tesseract signature",
			msgData:          rawData,
			expectedResponse: pubsub.ValidationReject,
			expectedError:    "tesseract seal does not exist",
		},
		{
			name:    "Message already exists in Tesseract Registry",
			msgData: rawData,
			preTestFn: func(tesseract *common.Tesseract) {
				// set mock tesseract seal
				sm.setTSSeal(tesseract)

				// Storing tesseract in Cache
				s.tesseractRegistry.Add(tesseract.Hash())
			},
			postTestFn: func(tesseract *common.Tesseract) {
				sm.removeTSSeal(tesseract)
			},
			expectedResponse: pubsub.ValidationIgnore,
		},
		{
			name:    "Message already exists in DB",
			msgData: rawData,
			preTestFn: func(tesseract *common.Tesseract) {
				// set mock tesseract seal
				sm.setTSSeal(tesseract)

				// Storing tesseract in DB
				err = pm.SetTesseract(tesseract.Hash(), rawTS)
				require.NoError(t, err)
			},
			postTestFn: func(tesseract *common.Tesseract) {
				sm.removeTSSeal(tesseract)
			},
			expectedResponse: pubsub.ValidationIgnore,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			msg := &pubsub.Message{
				Message: &pb.Message{
					Data: testcase.msgData,
					From: testcase.from,
				},
			}

			if testcase.preTestFn != nil {
				testcase.preTestFn(testTesseract)
			}

			resp, err := s.TesseractValidator(context.Background(), "", msg)

			if testcase.expectedError != "" {
				require.ErrorContains(t, err, testcase.expectedError)

				return
			}

			require.NoError(t, err)

			require.Equal(t, testcase.expectedResponse, resp)

			if testcase.postTestFn != nil {
				testcase.postTestFn(testTesseract)
			}
		})
	}
}

func TestIsAnyOtherParticipantStored(t *testing.T) {
	mockDB := NewMockDB()
	s := Syncer{
		db:     mockDB,
		logger: hclog.NewNullLogger(),
	}

	addresses := tests.GetAddresses(t, 3)
	heights := []uint64{99, 33, 21}

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: addresses,
		Heights:   heights,
	})

	for i := 0; i < 2; i++ {
		mockDB.setAccMetaInfoAt(addresses[i], heights[i])
	}

	testcases := []struct {
		name          string
		msg           *TesseractInfo
		expectedValue bool
	}{
		{
			name: "this participant is already stored",
			msg: &TesseractInfo{
				addr:      addresses[0],
				tesseract: ts,
			},
			expectedValue: false,
		},
		{
			name: "this participant is not stored and other participants stored",
			msg: &TesseractInfo{
				addr:      addresses[2],
				tesseract: ts,
			},
			expectedValue: true,
		},
		{
			name: "all participants are not stored",
			msg: &TesseractInfo{
				addr:      addresses[0],
				tesseract: tests.CreateTesseract(t, nil),
			},
			expectedValue: false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			isStored := s.isAnyOtherParticipantStored(test.msg)
			require.Equal(t, test.expectedValue, isStored)
		})
	}
}

func TestTrustedPeerInPeerstore(t *testing.T) {
	t.Parallel()

	ctx, ctxCancel := context.WithTimeout(context.Background(), maxTimeout)
	defer ctxCancel()

	// setup mock p2p server
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		1,
		true,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
	}

	server := createMultipleServers(t, 1, paramsMap)

	kid := tests.RandomKramaID(t, 0)
	peerID, err := kid.DecodedPeerID()
	require.NoError(t, err)

	ip := "/ip4/1.1.1.1/tcp/5000"
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", ip, peerID))
	require.NoError(t, err)

	// setup mock syncer config
	cfg := defaultSyncerConfig()
	cfg.TrustedPeers = []config.NodeInfo{
		{
			ID:      kid,
			Address: addr,
		},
	}

	pm, _ := createPersistenceManager(t, ctx)

	t.Cleanup(func() {
		closeTestServers(t, server...)
	})

	testSyncer := NewTestSyncer(
		ctx,
		cfg,
		server[0],
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address identifiers.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm,
		types.NewSlots(0, 0),
		"CLIENT",
		nil,
	)

	err = testSyncer.Start(0)
	require.NoError(t, err)

	multiAddr := make([]multiaddr.Multiaddr, 0)

	// retry until trusted peer is added to peer store
	_, err = tests.RetryUntilTimeout(ctx, 100*time.Millisecond, func() (interface{}, bool) {
		resp := testSyncer.network.GetAddrsFromPeerStore(peerID)

		multiAddr = append(multiAddr, resp...)

		if len(multiAddr) == 1 {
			return multiAddr, false
		}

		return nil, true
	})
	require.NoError(t, err)

	// check if ip matches, decapsulated multiAddr
	require.Equal(t, ip, multiAddr[0].String())
}

func TestFetchReceipts(t *testing.T) {
	mockDB := NewMockDB()
	mockSession := newMockSession(tests.RandomAddress(t))

	hashes := tests.GetHashes(t, 2)
	receipts := tests.CreateReceiptsWithTestData(t, tests.RandomHash(t))

	rawReceipts, err := receipts.Bytes()
	require.NoError(t, err)

	err = mockDB.SetReceipts(hashes[0], rawReceipts)
	require.NoError(t, err)

	cID := cid.ReceiptsCID(hashes[1])
	mockSession.setBlock(cID, block.NewBlock(cID, rawReceipts))

	s := Syncer{
		db:     mockDB,
		logger: hclog.NewNullLogger(),
	}

	testcases := []struct {
		name             string
		hash             common.Hash
		session          syncer.Session
		expectedReceipts common.Receipts
	}{
		{
			name:             "receipts fetched successfully from the db",
			hash:             hashes[0],
			session:          nil,
			expectedReceipts: receipts,
		},
		{
			name:             "receipts fetched successfully from agora when not found in db",
			hash:             hashes[1],
			session:          mockSession,
			expectedReceipts: receipts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipts, err = s.fetchReceipts(context.Background(), test.session, test.hash)
			require.NoError(t, err)

			checkForReceipts(t, test.expectedReceipts, receipts)
		})
	}
}

func TestFetchInteractions(t *testing.T) {
	mockDB := NewMockDB()
	mockSession := newMockSession(tests.RandomAddress(t))

	hashes := tests.GetHashes(t, 2)
	ixns := tests.CreateIxns(t, 1, nil)

	rawIxns, err := ixns.Bytes()
	require.NoError(t, err)

	err = mockDB.SetInteractions(hashes[0], rawIxns)
	require.NoError(t, err)

	cID := cid.InteractionsCID(hashes[1])
	mockSession.setBlock(cID, block.NewBlock(cID, rawIxns))

	s := Syncer{
		db:     mockDB,
		logger: hclog.NewNullLogger(),
	}

	testcases := []struct {
		name         string
		hash         common.Hash
		session      syncer.Session
		expectedIxns common.Interactions
	}{
		{
			name:         "interactions fetched successfully from the db",
			hash:         hashes[0],
			session:      nil,
			expectedIxns: ixns,
		},
		{
			name:         "interactions fetched successfully from agora when not found in db",
			hash:         hashes[1],
			session:      mockSession,
			expectedIxns: ixns,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixns, err = s.fetchInteractions(context.Background(), test.session, test.hash)
			require.NoError(t, err)

			require.Equal(t, test.expectedIxns, ixns)
		})
	}
}

func TestFetchContextForAgora(t *testing.T) {
	// initializing address here as we need same address for multiple tesseracts in chain
	address := tests.RandomAddress(t)
	hash := tests.RandomHash(t)
	nodes := tests.RandomKramaIDs(t, 3)

	testcases := []struct {
		name          string
		tsCount       int
		paramsMap     map[int]*tests.CreateTesseractParams
		smCallBack    func(sm *MockStateManager)
		shouldInsert  bool
		peerCount     int
		expectedError error
	}{
		{
			name:    "lattice has less than 10 context nodes",
			tsCount: 2,
			paramsMap: map[int]*tests.CreateTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 1, 3, 0),
				1: tesseractParamsWithContextDelta(t, address, 1, 1, 0),
			},
			shouldInsert: true,
			peerCount:    6,
		},
		{
			name:    "latest tesseracts context delta has more than 10 nodes",
			tsCount: 2,
			paramsMap: map[int]*tests.CreateTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 3, 5, 0),
				1: tesseractParamsWithContextDelta(t, address, 6, 5, 0),
			},
			shouldInsert: true,
			peerCount:    11,
		},
		{
			name:    "lattice has both context lock and context delta",
			tsCount: 2,
			paramsMap: map[int]*tests.CreateTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 3, 3, 0),
				1: {
					Addresses: []identifiers.Address{address},
					Participants: common.ParticipantsState{
						address: {
							ContextDelta:    getDeltaGroup(t, 2, 2, 0),
							PreviousContext: hash,
						},
					},
				},
			},
			smCallBack: func(sm *MockStateManager) {
				fmt.Println("insert", hash)
				sm.insertContextNodes(hash, nodes[:2], nodes[2])
			},
			shouldInsert: true,
			peerCount:    7,
		},
		{
			name:    "previous tesseract doesn't exist",
			tsCount: 1,
			paramsMap: map[int]*tests.CreateTesseractParams{
				0: {
					Addresses: []identifiers.Address{address},
					Participants: common.ParticipantsState{
						address: {
							TransitiveLink:  tests.RandomHash(t),
							ContextDelta:    getDeltaGroup(t, 1, 3, 0),
							PreviousContext: hash,
						},
					},
				},
			},
			shouldInsert:  true,
			expectedError: errors.New("error fetching tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseractsWithChain(t, test.tsCount, test.paramsMap)

			cm := newMockLattice(nil, nil)
			cm.insertTesseractsByHash(t, ts...)

			sm := newMockStateManager(nil)
			if test.smCallBack != nil {
				test.smCallBack(sm)
			}

			s := &Syncer{
				lattice: cm,
				state:   sm,
			}

			peers, err := s.fetchContextForAgora(ts[test.tsCount-1].AnyAddress(), *ts[test.tsCount-1])

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.peerCount, len(peers)) // check if peer count matches

			nodes := fetchContextFromLattice(t, ts[test.tsCount-1].AnyAddress(), *ts[test.tsCount-1], s)
			require.Equal(t, nodes, peers) // check if context nodes matches
		})
	}
}
