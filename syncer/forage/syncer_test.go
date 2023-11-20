package forage

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/storage/db"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/syncer/cid"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/syncer"
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM,
		newMockStateManager(),
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
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 2)

	accountsToSync := map[common.Address]int{
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
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[3],
		newMockStateManager(),
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
			newMockStateManager(),
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)

	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 10,
		common.SargaAddress:      10,
		addr:                     10,
	}

	ts1 := generateTesseracts(t, common.GuardianLogicAddr, 0, accountsToSync[common.GuardianLogicAddr], common.NilHash, 1)
	ts2 := generateTesseracts(t, common.SargaAddress, 0, accountsToSync[common.SargaAddress], common.NilHash, 1)
	ts3 := generateTesseracts(t, addr, 0, accountsToSync[addr], common.NilHash, 1)

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
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts1...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts2...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts3...)
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		newMockStateManager(),
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
		newMockStateManager(),
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

	accountsToSync := map[common.Address]int{
		addresses[2]: 3,
	}

	newTesseracts := generateTesseracts(t, addresses[2], 0, 3, common.NilHash, 1)

	clientSyncer.agora = newMockAgora()
	storeTesseractsInSession(t, clientSyncer, newTesseracts...)
	broadcastTesseracts(t, clientSyncer, serverSyncer, newTesseracts...)

	expectedEvents := SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM[0],
		newMockStateManager(),
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		serverPM[0],
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)
	accountsToSync := map[common.Address]int{
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

	accountsToSync = map[common.Address]int{
		addresses[2]:        4,
		common.SargaAddress: 8,
	}

	metaInfo, err := serverSyncer.db.GetAccountMetaInfo(common.SargaAddress)
	require.NoError(t, err)

	sargaTesseracts := generateTesseracts(t, common.SargaAddress, int(metaInfo.Height+1), 8, metaInfo.TesseractHash, 1)

	newTesseracts := generateTesseracts(t, addresses[2], 0, 4, common.NilHash, 1)
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

	clientSyncer = NewTestSyncer(
		clientCtx,
		defaultSyncerConfig(),
		client[0],
		&utils.TypeMux{},
		newMockAgora(),
		clientDB,
		newMockStateManager(),
		types.NewSlots(2, 3),
		"CLIENT-RESTART",
		nil,
	)

	storeTesseractsInSession(t, clientSyncer, sargaTesseracts...)

	mockSession := newMockSession(addresses[2])
	mockAgora, ok := clientSyncer.agora.(*MockAgora)
	require.True(t, ok)

	for _, ts := range newTesseracts {
		mockAgora.addSession(mockSession, cid.AccountCID(ts.StateHash()))
	}

	err = clientSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts: map[common.Address]AccountSpecificEvents{
			common.SargaAddress: newAccountSpecificEvents(0, 1, 5, 8),
			addresses[2]:        newAccountSpecificEvents(1, 1, 0, 4),
		},
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)

	newTesseracts = append(newTesseracts, sargaTesseracts...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		newMockStateManager(),
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
		newMockStateManager(),
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
	accountsToSync := map[common.Address]int{
		addresses[0]: 4,
		addresses[1]: 4,
		addresses[2]: 4,
	}

	newTesseracts := generateTesseractsGridByMap(t, accountsToSync)
	broadcastTesseracts(t, clientSyncer, serverSyncer, newTesseracts...)

	expectedEvents := SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, true, newTesseracts...)
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
	}

	pm, _ := createPersistenceManagers(t, ctx, 3)

	t.Cleanup(func() {
		closeTestServers(t, servers...)
	})

	serverSyncers := make([]*Syncer, 2)

	mux := &utils.TypeMux{}
	jq := &JobQueue{
		jobs: make(map[common.Address]*SyncJob),
		mux:  mux,
	}

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[2],
		mux,
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[2],
		newMockStateManager(),
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
			newMockStateManager(),
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)

	accountsToSync := map[common.Address]int{
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
			accounts:      make(map[common.Address]AccountSpecificEvents),
		}

		for addr, height := range accountsToSync {
			expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
		}

		SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	}()

	// start the server that has lattices later as we need to simulate removal of best peers in client
	// and when this is started, lattices will be available on this and client should complete accounts syncing
	_, err = tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
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
}

// TestJobProcessor makes sure sync tesseract routine not blocked
// server-0 has tesseracts for address-a1 from 0-7
// server-1 has tesseracts for address-a1 from 0-11
// create new sync job on client for address-a1 with best peer as server-0
// client syncs from server-0 for tesseracts(0-7)
// server-0 throws error that it doesn't have other tesseracts (8-11)
// so client still has some tesseracts to add and it returns early from lattice sync
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
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[2],
		newMockStateManager(),
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
			newMockStateManager(),
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)
	expectedHeight := 12
	accountsToSync := map[common.Address]int{
		addr: expectedHeight,
	}

	ts := generateTesseracts(t, addr, 0, accountsToSync[addr], common.NilHash, 1)

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
		[]id.KramaID{servers[0].GetKramaID()},
	)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		accounts: make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, testingLogger(t.Name()), clientSyncer.mux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
}

func TestSyncJobFromCanonicalInfo(t *testing.T) {
	pm, _ := createPersistenceManager(t, context.Background())
	j := &SyncJob{
		db:                    pm,
		address:               tests.RandomAddress(t),
		mode:                  common.LatestSync,
		snapDownloaded:        false,
		expectedHeight:        8,
		currentHeight:         2,
		jobState:              Sleep,
		lastModifiedAt:        time.Now(),
		tesseractQueue:        NewTesseractQueue(),
		latticeSyncInProgress: true,
	}

	err := j.commitJob()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		expectedJob *SyncJob
	}{
		{
			name:        "fetch sync job from db successfully",
			expectedJob: j,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountSyncInfos, err := pm.GetAccountsSyncStatus()
			require.NoError(t, err)

			syncJob, err := SyncJobFromCanonicalInfo(hclog.NewNullLogger(), pm, accountSyncInfos[0])
			require.NoError(t, err)

			require.Equal(t, test.expectedJob.db, syncJob.db)
			require.Equal(t, test.expectedJob.address, syncJob.address)
			require.Equal(t, test.expectedJob.expectedHeight, syncJob.expectedHeight)
			require.Equal(t, test.expectedJob.snapDownloaded, syncJob.snapDownloaded)
			require.Equal(t, test.expectedJob.mode, syncJob.mode)
			require.Equal(t, Pending, syncJob.jobState)
			require.True(t, test.expectedJob.lastModifiedAt.Equal(syncJob.lastModifiedAt))
			require.NotNil(t, syncJob.bestPeers)
			require.NotNil(t, syncJob.tesseractQueue)
		})
	}
}
