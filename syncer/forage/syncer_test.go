package forage

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

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
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer clientCancel()

	serverCtx, serverCancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM,
		newMockLattice(clientPM),
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
		&utils.TypeMux{},
		newMockAgora(),
		serverPM,
		newMockLattice(serverPM),
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 2)

	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 100,
		common.SargaAddress:      100,
		addresses[0]:             100,
		addresses[1]:             100,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)

	fmt.Println("CLIENT  ", servers[0].GetKramaID())
	fmt.Println("SERVER  ", servers[1].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	err = serverSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
}

// TestFullSync_ChooseBestPeer, makes sure that the client syncs to the highest heights for all accounts,
// even when not all servers have the latest state of all accounts.
func TestFullSync_ChooseBestPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[3],
		newMockLattice(pm[3]),
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
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			newMockLattice(pm[i]),
			newMockStateManager(),
			types.NewSlots(2, 3),
			"SERVER"+"-"+strconv.Itoa(i),
			nil,
		)
	}

	addr := tests.RandomAddress(t)

	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 100,
		common.SargaAddress:      100,
		addr:                     100,
	}

	ts1 := generateTesseracts(t, common.GuardianLogicAddr, 0, accountsToSync[common.GuardianLogicAddr], common.NilHash)
	ts2 := generateTesseracts(t, common.SargaAddress, 0, accountsToSync[common.SargaAddress], common.NilHash)
	ts3 := generateTesseracts(t, addr, 0, accountsToSync[addr], common.NilHash)

	storeTesseractsInDB(t, serverSyncers[0], ts1...)
	storeTesseractsInDB(t, serverSyncers[0], ts2...)
	storeTesseractsInDB(t, serverSyncers[0], ts3[0:75]...)

	storeTesseractsInDB(t, serverSyncers[1], ts1[0:25]...)
	storeTesseractsInDB(t, serverSyncers[1], ts2...)
	storeTesseractsInDB(t, serverSyncers[1], ts3...)

	storeTesseractsInDB(t, serverSyncers[2], ts1...)
	storeTesseractsInDB(t, serverSyncers[2], ts2[0:20]...)
	storeTesseractsInDB(t, serverSyncers[2], ts3...)

	fmt.Println("CLIENT  ", servers[3].GetKramaID())
	fmt.Println("SERVER  0 ", servers[0].GetKramaID())
	fmt.Println("SERVER  1 ", servers[1].GetKramaID())
	fmt.Println("SERVER  2 ", servers[2].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err := serverSyncers[i].Start(1)
		require.NoError(t, err)
	}

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts1...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts2...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts3...)
}

// TestSync_FromBroadcastedTesseract checks if client can sync from tesseract broadcast by server
// without executing tesseract using agora
func TestSync_FromBroadcastedTesseract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		newMockLattice(pm[0]),
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
		&utils.TypeMux{},
		newMockAgora(),
		pm[1],
		newMockLattice(pm[1]),
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)

	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 100,
		common.SargaAddress:      100,
		addresses[0]:             100,
		addresses[1]:             100,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)

	fmt.Println("CLIENT  ", servers[0].GetKramaID())
	fmt.Println("SERVER  ", servers[1].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	err = serverSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)

	accountsToSync = map[common.Address]int{
		addresses[2]: 5,
	}

	newTesseracts := generateTesseracts(t, addresses[2], 0, 5, common.NilHash)

	clientSyncer.agora = newMockAgora()
	storeTesseractsInSession(t, clientSyncer, newTesseracts...)
	broadcastTesseracts(t, serverSyncer, newTesseracts...)

	expectedEvents = SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
}

// TestSync_FromRejoining checks When client restarted, sarga tesseracts should sync through agora
// and normal account tesseracts should sync through snap sync
func TestSync_FromRejoining(t *testing.T) {
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 1*time.Minute)

	serverCtx, serverCancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		clientPM[0],
		newMockLattice(clientPM[0]),
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		serverPM[0],
		newMockLattice(serverPM[0]),
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)
	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 100,
		common.SargaAddress:      100,
		addresses[0]:             100,
		addresses[1]:             100,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)

	fmt.Println("CLIENT  ", servers[0].GetKramaID())
	fmt.Println("SERVER  ", servers[1].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	err = serverSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)

	clientCancel()
	clientPM[0].Close()

	err = clientSyncer.network.Unsubscribe(config.TesseractTopic)
	require.NoError(t, err)

	clientCtx, clientCancel = context.WithTimeout(context.Background(), 1*time.Minute)
	defer clientCancel()

	accountsToSync = map[common.Address]int{
		addresses[2]:        5,
		common.SargaAddress: 110,
	}

	metaInfo, err := serverSyncer.db.GetAccountMetaInfo(common.SargaAddress)
	require.NoError(t, err)

	sargaTesseracts := generateTesseracts(t, common.SargaAddress, int(metaInfo.Height+1), 110, metaInfo.TesseractHash)

	newTesseracts := generateTesseracts(t, addresses[2], 0, 5, common.NilHash)
	storeTesseractsInDB(t, serverSyncer, newTesseracts...)
	storeTesseractsInDB(t, serverSyncer, sargaTesseracts...)

	clientDB, err := storage.NewPersistenceManager(clientCtx, hclog.NewNullLogger(), &config.DBConfig{
		CleanDB:      false,
		DBFolderPath: clientDir[0],
		MaxSnapSize:  1073741824,
	})
	require.NoError(t, err)

	client := createMultipleServers(t, 1, paramsMap)
	connectClientToServers(t, client[0], servers[1])

	clientSyncer = NewTestSyncer(
		clientCtx,
		defaultSyncerConfig(),
		client[0],
		&utils.TypeMux{},
		&utils.TypeMux{},
		newMockAgora(),
		clientDB,
		newMockLattice(clientDB),
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

	expectedEvents = SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts: map[common.Address]AccountSpecificEvents{
			common.SargaAddress: newAccountSpecificEvents(0, 1, 101, 110),
			addresses[2]:        newAccountSpecificEvents(1, 1, 0, 5),
		},
	}

	SubscribeAndListenForSyncEvents(t, clientCtx, clientSyncer.testMux, expectedEvents)

	newTesseracts = append(newTesseracts, sargaTesseracts...)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, newTesseracts...)
}

// TestSync_ThroughExecution checks if client can sync from tesseract broadcast by server
// by executing tesseract
func TestSync_ThroughExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[0],
		newMockLattice(pm[0]),
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
		&utils.TypeMux{},
		newMockAgora(),
		pm[1],
		newMockLattice(pm[1]),
		newMockStateManager(),
		types.NewSlots(2, 3),
		"SERVER",
		nil,
	)

	addresses := tests.GetAddresses(t, 3)

	accountsToSync := map[common.Address]int{
		common.GuardianLogicAddr: 100,
		common.SargaAddress:      100,
		addresses[0]:             100,
		addresses[1]:             100,
	}

	ts := generateTesseractsByMap(t, accountsToSync)
	storeTesseractsInDB(t, serverSyncer, ts...)

	fmt.Println("CLIENT  ", servers[0].GetKramaID())
	fmt.Println("SERVER  ", servers[1].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	err = serverSyncer.Start(1)
	require.NoError(t, err)

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)

	accountsToSync = map[common.Address]int{
		addresses[2]: 5,
	}

	newTesseracts := generateTesseracts(t, addresses[2], 0, 5, common.NilHash)
	broadcastTesseracts(t, serverSyncer, newTesseracts...)

	expectedEvents = SyncEvents{
		bucketSync:    0,
		SystemAccSync: 0,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(0, 0, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, true, newTesseracts...)
}

// TestFullSync_RemoveBestPeer, makes sure that the client syncs to the highest heights for all accounts,
// even when two server doesn't have lattice even though it has the latest account meta info and snap data
func TestFullSync_RemoveBestPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	mux := &utils.TypeMux{}
	jq := &JobQueue{
		jobs: make(map[common.Address]*SyncJob),
		mux:  mux,
	}

	clientSyncer := NewTestSyncer(
		ctx,
		defaultSyncerConfig(),
		servers[3],
		mux,
		&utils.TypeMux{},
		&MockAgora{
			newSession: func(address common.Address) (syncer.Session, error) {
				return newMockSession(address), nil
			},
		},
		pm[3],
		newMockLattice(pm[3]),
		newMockStateManager(),
		types.NewSlots(2, 3),
		"CLIENT",
		func(s *Syncer) {
			s.jobQueue = jq
		},
	)

	for i := 0; i < 3; i++ {
		serverSyncers[i] = NewTestSyncer(
			ctx,
			defaultSyncerConfig(),
			servers[i],
			&utils.TypeMux{},
			&utils.TypeMux{},
			newMockAgora(),
			pm[i],
			newMockLattice(pm[i]),
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
	storeAccountMetaInfosAndSnapInDB(t, serverSyncers[2], ts...)

	fmt.Println("CLIENT  ", servers[3].GetKramaID())
	fmt.Println("SERVER  0 ", servers[0].GetKramaID())
	fmt.Println("SERVER  1 ", servers[1].GetKramaID())
	fmt.Println("SERVER  2 ", servers[2].GetKramaID())

	err := clientSyncer.Start(1)
	require.NoError(t, err)

	for i := 1; i < 3; i++ {
		err := serverSyncers[i].Start(1)
		require.NoError(t, err)
	}

	// start the server that has lattices later as we need to simulate removal of best peers in client
	// and when this is started lattices will be available on this and client should complete accounts syncing
	go func() {
		time.Sleep(2 * time.Second)

		// make sure best peers are empty as they don't have lattices
		job, ok := jq.getJob(common.GuardianLogicAddr)
		require.True(t, ok)
		require.Equal(t, 0, job.bestPeerLen())

		// start the server-0 and it has lattices
		err = serverSyncers[0].Start(1)
		require.NoError(t, err)
	}()

	expectedEvents := SyncEvents{
		bucketSync:    1,
		SystemAccSync: 1,
		accounts:      make(map[common.Address]AccountSpecificEvents),
	}

	for addr, height := range accountsToSync {
		expectedEvents.accounts[addr] = newAccountSpecificEvents(1, 1, 0, height)
	}

	SubscribeAndListenForSyncEvents(t, ctx, clientSyncer.testMux, expectedEvents)
	checkIfTesseractsSynced(t, clientSyncer, accountsToSync, false, ts...)
}
