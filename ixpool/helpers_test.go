package ixpool

import (
	"context"
	crand "crypto/rand"
	"errors"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/crypto"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/state"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
)

type expectedResult struct {
	nonce            uint64
	enqueued         uint64
	promoted         uint64
	promotedAccounts uint64
}

type MockStateManager struct {
	publicKey                map[identifiers.Identifier]map[uint64][]byte
	accountKeys              map[identifiers.Identifier]common.AccountKeys
	sequenceID               map[identifiers.Identifier]map[uint64]uint64
	balance                  map[identifiers.Identifier]map[identifiers.AssetID]*big.Int
	accountRegistration      map[identifiers.Identifier]bool
	logicRegistration        map[identifiers.LogicID]bool
	removedCacheStateObjects map[identifiers.Identifier]struct{}
	latestStateObjects       map[identifiers.Identifier]*state.Object
	accMetaInfos             map[identifiers.Identifier]*common.AccountMetaInfo
}

func (ms *MockStateManager) GetAssetInfo(assetID identifiers.AssetID,
	hash common.Hash,
) (*common.AssetDescriptor, error) {
	panic("implement me")
}

// NewMockStateManager returns a new instance of MockStateManager
func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	return &MockStateManager{
		publicKey:                make(map[identifiers.Identifier]map[uint64][]byte),
		accountKeys:              make(map[identifiers.Identifier]common.AccountKeys),
		sequenceID:               make(map[identifiers.Identifier]map[uint64]uint64),
		balance:                  map[identifiers.Identifier]map[identifiers.AssetID]*big.Int{},
		accountRegistration:      make(map[identifiers.Identifier]bool),
		logicRegistration:        make(map[identifiers.LogicID]bool),
		removedCacheStateObjects: make(map[identifiers.Identifier]struct{}),
		latestStateObjects:       make(map[identifiers.Identifier]*state.Object),
		accMetaInfos:             make(map[identifiers.Identifier]*common.AccountMetaInfo),
	}
}

func (ms *MockStateManager) setPublicKey(id identifiers.Identifier,
	keyID uint64, publicKey []byte,
) {
	_, ok := ms.publicKey[id]
	if !ok {
		ms.publicKey[id] = make(map[uint64][]byte)
	}

	ms.publicKey[id][keyID] = publicKey
}

func (ms *MockStateManager) GetPublicKey(id identifiers.Identifier,
	keyID uint64, stateHash common.Hash,
) ([]byte, error) {
	keys, ok := ms.publicKey[id]
	if ok {
		if publicKey, ok := keys[keyID]; ok {
			return publicKey, nil
		}
	}

	return nil, common.ErrPublicKeyNotFound
}

// setLatestSequenceID updates the mock account with the latest sequenceID
func (ms *MockStateManager) setLatestSequenceID(t *testing.T, id identifiers.Identifier, keyID, nonce uint64) {
	t.Helper()

	_, ok := ms.sequenceID[id]
	if !ok {
		ms.sequenceID[id] = make(map[uint64]uint64)
	}

	ms.sequenceID[id][keyID] = nonce
}

func (ms *MockStateManager) GetSequenceID(id identifiers.Identifier,
	keyID uint64, stateHash common.Hash,
) (uint64, error) {
	if accountKeys, ok := ms.sequenceID[id]; ok {
		if sequenceID, ok := accountKeys[keyID]; ok {
			return sequenceID, nil
		}
	}

	return 0, errors.New("account doesn't exists")
}

func (ms *MockStateManager) GetAccountKeys(id identifiers.Identifier,
	stateHash common.Hash,
) (common.AccountKeys, error) {
	accountKeys, ok := ms.accountKeys[id]
	if ok {
		return accountKeys, nil
	}

	return nil, errors.New("account keys not found")
}

func (ms *MockStateManager) SetAccountMetaInfo(id identifiers.Identifier, accMetaInfo *common.AccountMetaInfo) {
	ms.accMetaInfos[id] = accMetaInfo
}

func (ms *MockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := ms.accMetaInfos[id]
	if !ok {
		return nil, errors.New("account meta info not found")
	}

	return accMetaInfo, nil
}

func (ms *MockStateManager) setLatestStateObject(id identifiers.Identifier, obj *state.Object) {
	ms.latestStateObjects[id] = obj
}

func (ms *MockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	s, ok := ms.latestStateObjects[id]
	if !ok {
		return nil, errors.New("state object not found")
	}

	return s, nil
}

func (ms *MockStateManager) RemoveCachedObject(id identifiers.Identifier) {
	ms.removedCacheStateObjects[id] = struct{}{}
}

func (ms *MockStateManager) setTestMOIBalance(t *testing.T, ids ...identifiers.Identifier) {
	t.Helper()

	for _, id := range ids {
		ms.setBalance(t, id, common.KMOITokenAssetID, big.NewInt(1000))
	}
}

func (ms *MockStateManager) updateAccountKeys(t *testing.T, accs []tests.AccountWithMnemonic) {
	t.Helper()

	for i := 0; i < len(accs); i++ {
		ms.setAccountKeys(accs[i].ID, common.AccountKeys{
			{
				Weight:    1000,
				PublicKey: accs[i].PublicKey,
			},
		})
		ms.setPublicKey(accs[i].ID, 0, accs[i].PublicKey)
	}
}

func (ms *MockStateManager) setAccountKeysAndPublicKeys(t *testing.T, ids []identifiers.Identifier, pk [][]byte) {
	t.Helper()

	for i := 0; i < len(ids); i++ {
		ms.setAccountKeys(ids[i], common.AccountKeys{
			{
				Weight:    1000,
				PublicKey: pk[i],
			},
		})
		ms.setPublicKey(ids[i], 0, pk[i])
	}
}

func (ms *MockStateManager) setAccountKeys(id identifiers.Identifier, accKeys common.AccountKeys) {
	ms.accountKeys[id] = accKeys
}

func (ms *MockStateManager) GetBalance(
	id identifiers.Identifier,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	assets, ok := ms.balance[id]
	if !ok {
		return big.NewInt(0), common.ErrFetchingBalance
	}

	va, ok := assets[assetID]
	if !ok {
		return big.NewInt(0), common.ErrFetchingBalance
	}

	return va, nil
}

func (ms *MockStateManager) setBalance(
	t *testing.T,
	id identifiers.Identifier,
	assetID identifiers.AssetID,
	amount *big.Int,
) {
	t.Helper()

	if _, ok := ms.balance[id]; !ok {
		ms.balance[id] = make(map[identifiers.AssetID]*big.Int)
	}

	ms.balance[id][assetID] = amount
}

const viewTimeOut = 10 * time.Second

func (ms *MockStateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	_, ok := ms.accountRegistration[id]

	return ok, nil
}

func (ms *MockStateManager) registerAccounts(ids ...identifiers.Identifier) {
	for _, id := range ids {
		ms.accountRegistration[id] = true
	}
}

func (ms *MockStateManager) IsLogicRegistered(logicID identifiers.LogicID) error {
	if _, ok := ms.logicRegistration[logicID]; !ok {
		return errors.New("logic id is not registered")
	}

	return nil
}

func (ms *MockStateManager) registerLogicID(t *testing.T, logicID identifiers.LogicID) {
	t.Helper()

	ms.logicRegistration[logicID] = true
}

// CreateTestIxpool returns a new instance of IxPool
func CreateTestIxpool(
	t *testing.T,
	cfgCallback func(cfg *config.IxPoolConfig),
	skipSignatureVerification bool,
	sm *MockStateManager,
	exec *MockExecutionManager,
	network *mockNetwork,
) *IxPool {
	t.Helper()

	verifier := crypto.Verify
	cfg := new(config.IxPoolConfig)

	if sm == nil {
		sm = NewMockStateManager(t)
	}

	cfg.ViewTimeout = viewTimeOut

	if cfgCallback != nil {
		cfgCallback(cfg)
	}

	if skipSignatureVerification {
		verifier = func(data, signature, pubBytes []byte) (bool, error) {
			return true, nil
		}
	}

	ixpool, err := NewIxPool(
		hclog.NewNullLogger(),
		new(utils.TypeMux),
		network,
		sm,
		exec,
		cfg,
		NilMetrics(),
		verifier,
		0,
	)
	require.NoError(t, err)

	return ixpool
}

type MockExecutionManager struct {
	validateLogicDeployHook func() error
	validateLogicInvokeHook func() error
}

func (ms *MockExecutionManager) ValidateLogicInvoke(
	op *common.IxOp, calleracc, logicacc *state.Object,
) error {
	if ms.validateLogicInvokeHook != nil {
		return ms.validateLogicInvokeHook()
	}

	return nil
}

func (ms *MockExecutionManager) ValidateLogicEnlist(
	op *common.IxOp, calleracc, logicacc *state.Object,
) error {
	// TODO implement me
	panic("implement me")
}

func NewMockExecutionManager(t *testing.T) *MockExecutionManager {
	t.Helper()

	exec := new(MockExecutionManager)

	return exec
}

func (ms *MockExecutionManager) ValidateLogicDeploy(op *common.IxOp) error {
	if ms.validateLogicDeployHook != nil {
		return ms.validateLogicDeployHook()
	}

	return nil
}

type mockNetwork struct {
	broadcasted   map[string][]byte
	subscriptions map[string]struct{}
	kramaID       identifiers.KramaID
}

func newMockNetwork(kramaID identifiers.KramaID) *mockNetwork {
	return &mockNetwork{
		broadcasted:   make(map[string][]byte),
		subscriptions: make(map[string]struct{}),
		kramaID:       kramaID,
	}
}

func (m *mockNetwork) Subscribe(ctx context.Context, topicName string,
	validator utils.WrappedVal, defaultValidator bool, handler func(msg *pubsub.Message) error,
) error {
	m.subscriptions[topicName] = struct{}{}

	return nil
}

func (m *mockNetwork) Broadcast(topicName string, data []byte) error {
	m.broadcasted[topicName] = data

	return nil
}

func (m *mockNetwork) GetKramaID() identifiers.KramaID {
	return m.kramaID
}

// benchmark abstractions
type cachePusher interface {
	push()
}

type cacheMaker interface {
	make(size int) cachePusher
}

type (
	digestCacheMaker struct{}
	saltedCacheMaker struct{}
)

func (m digestCacheMaker) make(size int) cachePusher {
	return &digestCachePusher{c: newDigestCache(size)}
}

func (m saltedCacheMaker) make(size int) cachePusher {
	scp := &saltedCachePusher{c: newSaltedCache(size)}
	scp.c.Start(context.Background(), 0)

	return scp
}

type digestCachePusher struct {
	c *digestCache
}
type saltedCachePusher struct {
	c *ixSaltedCache
}

func (p *digestCachePusher) push() {
	var d [common.HashLength]byte

	if _, err := crand.Read(d[:]); err != nil {
		panic(err)
	}

	h := common.Hash(blake2b.Sum256(d[:])) // digestCache does not hashes so calculate hash here
	p.c.CheckAndInsert(&h)
}

func (p *saltedCachePusher) push() {
	var d [common.HashLength]byte
	if _, err := crand.Read(d[:]); err != nil {
		panic(err)
	}

	p.c.CheckAndPut(d[:]) // saltedCache hashes inside
}

func getIXParams(
	id identifiers.Identifier,
	ixType common.IxOpType,
	fuelPrice *big.Int,
	actionPayload common.AssetActionPayload,
	sign []byte,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Sender = common.Sender{
				ID: id,
			}
			ix.FuelPrice = fuelPrice
			ix.FuelLimit = 1
			ix.Funds = []common.IxFund{
				{
					AssetID: actionPayload.AssetID,
					Amount:  actionPayload.Amount,
				},
			}
			ix.IxOps = []common.IxOpRaw{
				{
					Type: ixType,
					Payload: func() []byte {
						payload, _ := actionPayload.Bytes()

						return payload
					}(),
				},
			}
			ix.Participants = []common.IxParticipant{
				{
					ID:       id,
					LockType: common.MutateLock,
				},
				{
					ID:       actionPayload.Beneficiary,
					LockType: common.MutateLock,
				},
			}
		},
		SenderSign: sign,
	}
}

// getOperationInfo determines and returns the relevant IxOpRaw, IxFund, and IxParticipant
// based on the given ix type and payload.
func getOperationInfo(
	t *testing.T, ixType common.IxOpType,
	opPayload interface{},
) (*common.IxOpRaw, *common.IxFund, *common.IxParticipant) {
	t.Helper()

	switch ixType {
	case common.IxInvalid:
		ixOperation := &common.IxOpRaw{
			Type: ixType,
		}

		return ixOperation, nil, nil

	case common.IxParticipantCreate:
		payload, ok := opPayload.(common.ParticipantCreatePayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		ixParticipant := &common.IxParticipant{
			ID:       payload.ID,
			LockType: common.MutateLock,
		}

		return ixOperation, nil, ixParticipant
	case common.IXAccountInherit:
		payload, ok := opPayload.(common.AccountInheritPayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		return ixOperation, nil, nil

	case common.IXAccountConfigure:
		payload, ok := opPayload.(common.AccountConfigurePayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		return ixOperation, nil, nil

	case common.IxAssetTransfer:
		payload, ok := opPayload.(common.AssetActionPayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		ixFund := &common.IxFund{
			AssetID: payload.AssetID,
			Amount:  payload.Amount,
		}

		ixParticipant := &common.IxParticipant{
			ID:       payload.Beneficiary,
			LockType: common.MutateLock,
		}

		return ixOperation, ixFund, ixParticipant
	case common.IxAssetCreate:
		payload, ok := opPayload.(common.AssetCreatePayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		return ixOperation, nil, nil
	case common.IxAssetMint, common.IxAssetBurn:
		payload, ok := opPayload.(common.AssetSupplyPayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		ixFund := &common.IxFund{
			AssetID: payload.AssetID,
			Amount:  payload.Amount,
		}

		ixParticipant := &common.IxParticipant{
			ID:       payload.AssetID.AsIdentifier(),
			LockType: common.MutateLock,
		}

		return ixOperation, ixFund, ixParticipant
	case common.IxLogicDeploy:
		payload, ok := opPayload.(common.LogicPayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		return ixOperation, nil, nil
	case common.IxLogicInvoke, common.IxLogicEnlist:
		payload, ok := opPayload.(common.LogicPayload)
		require.True(t, ok)

		rawPayload, err := payload.Bytes()
		require.NoError(t, err)

		ixOperation := &common.IxOpRaw{
			Type:    ixType,
			Payload: rawPayload,
		}

		ixParticipant := &common.IxParticipant{
			ID:       payload.Logic.AsIdentifier(),
			LockType: common.MutateLock,
		}

		return ixOperation, nil, ixParticipant
	default:
		panic(common.ErrInvalidInteractionType)
	}
}

// newTestInteraction returns a new instance of types.Interaction with the input
func newTestInteraction(
	t *testing.T,
	ixType common.IxOpType,
	opPayload interface{},
	nonce int,
	senderID identifiers.Identifier,
	keyID uint64,
	cb func(ixData *common.IxData),
) *common.Interaction {
	t.Helper()

	if senderID.IsNil() {
		senderID = tests.RandomIdentifier(t)
	}

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         senderID,
			KeyID:      keyID,
			SequenceID: uint64(nonce),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 1,
		Funds:     []common.IxFund{},
		IxOps:     []common.IxOpRaw{},
		Participants: []common.IxParticipant{
			{
				ID:       senderID,
				LockType: common.MutateLock,
			},
		},
	}

	ixOperation, ixFund, ixParticipant := getOperationInfo(t, ixType, opPayload)

	ixData.IxOps = append(ixData.IxOps, *ixOperation)

	if ixFund != nil {
		ixData.Funds = append(ixData.Funds, *ixFund)
	}

	if ixParticipant != nil {
		ixData.Participants = append(ixData.Participants, *ixParticipant)
	}

	if cb != nil {
		cb(ixData)
	}

	signatures := common.Signatures{{
		ID:    senderID,
		KeyID: keyID,
	}}

	ix, err := common.NewInteraction(*ixData, signatures)
	require.NoError(t, err)

	return ix
}

// createTestIxs creates and returns multiple instances of types.Interactions based on the given range
func createTestIxs(
	t *testing.T,
	startNonce int,
	endNonce int,
	id identifiers.Identifier,
) []*common.Interaction {
	t.Helper()

	ixs := make([]*common.Interaction, 0)

	for nonce := startNonce; nonce < endNonce; nonce++ {
		ixs = append(ixs, newTestInteraction(
			t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
			nonce, id, 0, nil,
		))
	}

	return ixs
}

// createTestIxs creates and returns multiple instances of types.Interactions based on the given range
// It generates ixns for each key of same account based on key count is given
func createTestAssetTransferIxs(
	t *testing.T,
	startNonce int,
	endNonce int,
	id identifiers.Identifier,
	keyCount int,
	sm *MockStateManager,
) []*common.Interaction {
	t.Helper()

	ixs := make([]*common.Interaction, 0)

	for nonce := startNonce; nonce < endNonce; nonce++ {
		for i := 0; i < keyCount; i++ {
			beneficiary := tests.RandomIdentifierWithZeroVariant(t)
			ixs = append(ixs, newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, beneficiary),
				nonce, id, uint64(i), nil,
			))

			sm.registerAccounts(beneficiary)
		}
	}

	return ixs
}

// getTesseractWithIxs returns a new instance of types.Tesseract with interactions
func getTesseractWithIxs(t *testing.T, id identifiers.Identifier, nonce int) *common.Tesseract {
	t.Helper()

	ixs := common.NewInteractionsWithLeaderCheck(false, newTestInteraction(
		t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.Nil),
		nonce, id, 0, nil,
	))

	tsParams := &tests.CreateTesseractParams{
		IDs:     []identifiers.Identifier{id},
		Heights: []uint64{0},
		ParticipantsCallback: func(participants common.ParticipantsState) {
			state := participants[id]

			state.ContextDelta = &common.DeltaGroup{
				ConsensusNodes: tests.RandomKramaIDs(t, 2),
			}

			participants[id] = state
		},
		Ixns: ixs,
	}

	ts := tests.CreateTesseract(t, tsParams)

	return ts
}

// newIxWithFuelPrice returns a new instance of types.Interaction with the given fuelPrice
func newIxWithFuelPrice(t *testing.T, nonce int, id identifiers.Identifier, fuelPrice int64) *common.Interaction {
	t.Helper()

	return newTestInteraction(
		t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.Nil),
		nonce, id, 0, func(ixData *common.IxData) {
			ixData.FuelPrice = big.NewInt(fuelPrice)
		},
	)
}

// newIxWithWaitCounter returns a new instance of WaitInteractions with the given waitCounter and new interaction
func newIxWithWaitCounter(t *testing.T, nonce int, id identifiers.Identifier, waitCounter int32) *WaitInteractions {
	t.Helper()

	ix := newTestInteraction(
		t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.Nil),
		nonce, id, 0, nil,
	)

	return &WaitInteractions{waitCounter, ix}
}

// newIxWithPayload returns a new instance of types.Interaction with the given payload
func newIxWithPayload(
	t *testing.T,
	ixType common.IxOpType,
	nonce int,
	id identifiers.Identifier,
	payload []byte,
) *common.Interaction {
	t.Helper()

	return newTestInteraction(t, common.IxInvalid, nil, nonce, id, 0, func(ixData *common.IxData) {
		ixData.IxOps = []common.IxOpRaw{
			{
				Type:    ixType,
				Payload: payload,
			},
		}
	})
}

// addAndProcessIxs enqueues and promotes the ixs based on sequenceID
func addAndProcessIxs(t *testing.T, sm *MockStateManager, ixPool *IxPool, ixs ...*common.Interaction) {
	t.Helper()

	for _, v := range ixs {
		sm.setTestMOIBalance(t, v.SenderID())
	}

	errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(false, ixs...))
	require.Len(t, errs, 0)
}

// mintIxs mints and returns the interactions from the interactionQueue
func mintIxs(t *testing.T, ixPool *IxPool) []*common.Interaction {
	t.Helper()

	mintedIxs := make([]*common.Interaction, 0)

	interactionQueue := ixPool.Executables()

	for interactionQueue.Len() > 0 {
		ix, ok := interactionQueue.Pop().(*common.Interaction)
		require.True(t, ok)
		ixPool.Pop(ix)

		mintedIxs = append(mintedIxs, ix)
	}

	return mintedIxs
}

// getSuccessfulIxs mints and returns the expected number of interactions from the interactionQueue
func getSuccessfulIxs(t *testing.T, ixPool *IxPool, noOfExpectedIxs int) []*common.Interaction {
	t.Helper()

	successfulIxs := make([]*common.Interaction, 0)

	for len(successfulIxs) < noOfExpectedIxs {
		successfulIxs = append(successfulIxs, mintIxs(t, ixPool)...)
	}

	return successfulIxs
}

// setDelayCounter updates the given account's delay counter
func setDelayCounter(t *testing.T, acc *account, delayCount int32) {
	t.Helper()

	acc.waitLock.Lock()
	defer acc.waitLock.Unlock()

	acc.delayCounter = delayCount
}

// getIxNonce returns a map of ix sender id to sequenceID
func getIxNonce(t *testing.T, ixs []*common.Interaction) map[identifiers.Identifier]uint64 {
	t.Helper()

	ixNonce := make(map[identifiers.Identifier]uint64)

	for _, ix := range ixs {
		ixNonce[ix.SenderID()] = ix.SequenceID()
	}

	return ixNonce
}

func calcCacheSize(numIter int) int {
	size := numIter / 3 // in order to exercise map swaps
	if size == 0 {
		size++
	}

	return size
}

func benchmarkDigestCache(b *testing.B, m cacheMaker, numThreads int) {
	b.Helper()

	p := m.make(calcCacheSize(b.N))
	numHashes := b.N / numThreads // num hashes per goroutine
	// b.Logf("inserting %d (%d) values in %d threads into cache of size %d", b.N, numHashes,
	// numThreads, calcCacheSize(b.N))
	var wg sync.WaitGroup

	wg.Add(numThreads)

	for i := 0; i < numThreads; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numHashes; j++ {
				p.push()
			}
		}()
	}

	wg.Wait()
}

type CreateBatch struct {
	ixnCount                int
	consensusNodesHashCount int
}

type CreateBatches struct {
	batchCount int
	batch      CreateBatch
}

func addBatch(t *testing.T, registry *IxBatchRegistry, ixnCount int, consensusNodesHashCount int) {
	t.Helper()

	ix := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, tests.RandomIdentifier(t)),
				},
			}

			for i := 0; i < consensusNodesHashCount-2; i++ {
				ix.Participants = append(ix.Participants, common.IxParticipant{
					ID: tests.RandomIdentifier(t),
				})
			}
		},
	})

	for id := range ix.Participants() {
		registry.consensusNodesHash[id] = tests.RandomHash(t)
	}

	for i := 0; i < ixnCount; i++ {
		registry.addIx(ix)
	}
}

func addBatches(t *testing.T, registry *IxBatchRegistry, batchesList []CreateBatches) {
	t.Helper()

	for _, batches := range batchesList {
		for i := 0; i < batches.batchCount; i++ {
			addBatch(t, registry, batches.batch.ixnCount, batches.batch.consensusNodesHashCount)
		}
	}
}

func insertIxnsInPromotedQueue(ixPool *IxPool, input []*common.Interaction) {
	for _, ix := range input {
		_, accKey := ixPool.getOrCreateAccountQueue(ix.SenderID(), ix.SenderKeyID(), ix.SequenceID())
		accKey.promoted.push(ix)
		ixPool.accounts.addToSortedAccounts(ix.SenderID())
	}
}

func validateAllocatedView(t *testing.T, allocatedView uint64, currentView uint64, nodePos uint64) {
	t.Helper()

	require.Equal(t, nodePos, allocatedView%common.ConsensusNodesSize)
	require.GreaterOrEqual(t, allocatedView, currentView)
	require.Less(t, allocatedView, currentView+common.ConsensusNodesSize)
}

// createIxnsFromParticipants processes a list of participant groups to generate interactions.
// Each group in the input represents a interaction and consists of participants.
// - The first element in each group represents the sender.
// - If the second element is a "sarga" id, a "participant creation" interaction is created.
// - Otherwise, an "asset transfer" interaction is generated.
// - always use the participants indexes in sequential manner.
// interactions are generated in increasing sequenceID order starting from zero
// 0-100 is reserved for primary accounts and 101-200 reserved for sub accounts
func createIxnsFromParticipants(t *testing.T, input [][]int) []*common.Interaction {
	t.Helper()

	participantCount := 0

	for _, i := range input {
		for _, j := range i {
			if j != 999 {
				participantCount = common.Max(participantCount, j+1)
			}
		}
	}

	primaryAccounts := common.IdentifierList(tests.GetIdentifiers(t, participantCount))
	sort.Sort(primaryAccounts)

	subAccounts := common.IdentifierList(tests.GetSubAccountIdentifiers(t, participantCount))
	sort.Sort(subAccounts)

	getAccount := func(index int) identifiers.Identifier {
		if index >= 101 {
			return subAccounts[index]
		} else {
			return primaryAccounts[index]
		}
	}

	nonces := make(map[identifiers.Identifier]int)
	ixns := make([]*common.Interaction, len(input))

	for i, list := range input {
		nonce := 0

		if n, ok := nonces[getAccount(list[0])]; ok {
			nonce = n
		}

		if list[1] == 999 {
			ixns[i] = newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				nonce, getAccount(list[0]), 0, func(ixData *common.IxData) {
					for i := 2; i < len(list); i++ {
						ixData.Participants = append(ixData.Participants, common.IxParticipant{
							ID: getAccount(list[i]),
						})
					}
				},
			)
		} else {
			ixns[i] = newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, getAccount(list[1])),
				nonce, getAccount(list[0]), 0, func(ixData *common.IxData) {
					for i := 2; i < len(list); i++ {
						ixData.Participants = append(ixData.Participants, common.IxParticipant{
							ID: getAccount(list[i]),
						})
					}
				},
			)
		}

		nonces[getAccount(list[0])] = nonce + 1
	}

	return ixns
}
