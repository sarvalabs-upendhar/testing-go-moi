package lattice

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/mudra"
	mudracommon "github.com/sarvalabs/moichain/mudra/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
	"github.com/stretchr/testify/require"
)

const GenesisFile = "genesis_test"

var (
	validCommitSign   = []byte{1}
	invalidCommitSign = []byte{0}
)

type result struct {
	data interface{}
	err  error
}

// MockDB is an in-memory key-value database used for testing purposes
type MockDB struct {
	dbStorage                   map[string][]byte
	accMetaInfos                map[types.Address]*types.AccountMetaInfo
	updateMetaInfoHook          func() (int32, bool, error)
	setTesseractHook            func() error
	setInteractionsHook         func() error
	setReceiptsHook             func() error
	setTesseractHeightEntryHook func() error
	setIxLookupHook             func() error
	createEntryHook             func() error
}

type testTSArgs struct {
	cache           bool
	stateExists     bool
	tesseractExists bool
}

func mockDB(t *testing.T) *MockDB {
	t.Helper()

	return &MockDB{
		dbStorage:    make(map[string][]byte),
		accMetaInfos: make(map[types.Address]*types.AccountMetaInfo),
	}
}

func (m *MockDB) CreateEntry(key []byte, value []byte) error {
	if m.createEntryHook != nil {
		return m.createEntryHook()
	}

	m.dbStorage[string(key)] = value

	return nil
}

func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	val, ok := m.dbStorage[string(key)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return val, nil
}

func (m *MockDB) Contains(key []byte) (bool, error) {
	_, ok := m.dbStorage[string(key)]

	return ok, nil
}

func (m *MockDB) UpdateAccMetaInfo(
	id types.Address,
	height *big.Int,
	tesseractHash types.Hash,
	accType types.AccountType,
	latticeExists bool,
	tesseractExists bool,
) (int32, bool, error) {
	if m.updateMetaInfoHook != nil {
		return m.updateMetaInfoHook()
	}

	m.accMetaInfos[id] = &types.AccountMetaInfo{
		Address:       id,
		Type:          accType,
		Height:        height,
		TesseractHash: tesseractHash,
		StateExists:   tesseractExists,
		LatticeExists: latticeExists,
	}

	return 8, true, nil
}

func (m *MockDB) GetAccMetaInfo(id types.Address) (*types.AccountMetaInfo, bool) {
	val, ok := m.accMetaInfos[id]

	return val, ok
}

func (m *MockDB) GetTesseract(hash types.Hash) ([]byte, error) {
	data, ok := m.dbStorage[hash.String()]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseract(hash types.Hash, data []byte) error {
	if m.setTesseractHook != nil {
		return m.setTesseractHook()
	}

	m.dbStorage[hash.String()] = data

	return nil
}

func (m *MockDB) HasTesseract(hash types.Hash) (bool, error) {
	_, ok := m.dbStorage[hash.String()]

	return ok, nil
}

func (m *MockDB) SetInteractions(hash types.Hash, data []byte) error {
	if m.setInteractionsHook != nil {
		return m.setInteractionsHook()
	}

	m.dbStorage[hash.String()] = data

	return nil
}

func (m *MockDB) GetTesseractHeightEntry(addr types.Address, height uint64) ([]byte, error) {
	key := addr.Hex() + strconv.Itoa(int(height))

	data, ok := m.dbStorage[key]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseractHeightEntry(addr types.Address, height uint64, hash types.Hash) error {
	if m.setTesseractHeightEntryHook != nil {
		return m.setTesseractHeightEntryHook()
	}

	key := addr.Hex() + strconv.Itoa(int(height))

	m.dbStorage[key] = hash.Bytes()

	return nil
}

func (m *MockDB) GetIxLookup(ixHash types.Hash) ([]byte, error) {
	data, ok := m.dbStorage[ixHash.String()]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetIxLookup(ixHash types.Hash, data []byte) error {
	if m.setIxLookupHook != nil {
		return m.setIxLookupHook()
	}

	m.dbStorage[ixHash.String()] = data

	return nil
}

func (m *MockDB) GetReceipts(receiptHash types.Hash) ([]byte, error) {
	data, ok := m.dbStorage[receiptHash.String()]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetReceipts(receiptHash types.Hash, data []byte) error {
	if m.setReceiptsHook != nil {
		return m.setReceiptsHook()
	}

	m.dbStorage[receiptHash.String()] = data

	return nil
}

type MockNetwork struct {
	kramaID id.KramaID
}

// mock network implementation

func (n *MockNetwork) SetKramaID(id id.KramaID) {
	n.kramaID = id
}

func (n *MockNetwork) GetKramaID() id.KramaID {
	return n.kramaID
}

func (n *MockNetwork) Broadcast(topic string, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (n *MockNetwork) Unsubscribe(topic string) error {
	// TODO implement me
	panic("implement me")
}

func (n *MockNetwork) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	// TODO implement me
	panic("implement me")
}

type MockExec struct {
	receipts                map[types.Hash]types.Receipts
	revertHook              func() error
	executeInteractionsHook func() (types.Receipts, error)
}

func mockExec(t *testing.T) *MockExec {
	t.Helper()

	return &MockExec{
		receipts: make(map[types.Hash]types.Receipts),
	}
}

// mock execution implementation
func (e *MockExec) ExecuteInteractions(
	clusterID types.ClusterID,
	ixs types.Interactions,
	contextDelta types.ContextDelta,
) (types.Receipts, error) {
	if e.executeInteractionsHook != nil {
		return e.executeInteractionsHook()
	}

	if val, ok := e.receipts[ixs[0].Hash()]; ok {
		return val, nil
	}

	return nil, types.ErrInvalidInteractions
}

func (e *MockExec) Revert(clusterID types.ClusterID) error {
	if e.revertHook != nil {
		return e.revertHook()
	}

	return nil
}

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type AccHash struct {
	contextHash types.Hash
	stateHash   types.Hash
}

type MockStateManager struct {
	dirtyObjects        map[types.Address]*guna.StateObject
	objects             map[types.Address]*guna.StateObject
	context             map[types.Hash]*Context
	publicKeys          map[id.KramaID][]byte
	latestTesseracts    map[types.Address]*types.Tesseract
	dbTesseracts        map[types.Hash]*types.Tesseract
	registeredAccounts  map[types.Address]*AccHash
	cleanUp             map[types.Address]bool
	accountTypes        map[types.Address]types.AccountType
	flushedDirtyObjects map[types.Address]bool

	flushHook               func() error
	newAccountHook          func() (types.Hash, types.Hash, error)
	sargaAccountHook        func() (types.Hash, types.Hash, error)
	accountRegistrationHook func(hash types.Hash) (bool, error)
}

func mockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	return &MockStateManager{
		dirtyObjects:        make(map[types.Address]*guna.StateObject),
		objects:             make(map[types.Address]*guna.StateObject),
		context:             make(map[types.Hash]*Context),
		publicKeys:          make(map[id.KramaID][]byte),
		latestTesseracts:    make(map[types.Address]*types.Tesseract),
		dbTesseracts:        make(map[types.Hash]*types.Tesseract),
		registeredAccounts:  make(map[types.Address]*AccHash),
		cleanUp:             make(map[types.Address]bool),
		accountTypes:        make(map[types.Address]types.AccountType),
		flushedDirtyObjects: make(map[types.Address]bool),
	}
}

func (sm *MockStateManager) isCleanup(addrs types.Address) bool {
	if _, ok := sm.cleanUp[addrs]; ok {
		return true
	}

	return false
}

func (sm *MockStateManager) CreateDirtyObject(addr types.Address, accType types.AccountType) *guna.StateObject {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) setAccType(address types.Address, accountType types.AccountType) {
	sm.accountTypes[address] = accountType
}

func (sm *MockStateManager) GetAccTypeUsingStateObject(address types.Address) (types.AccountType, error) {
	accType, ok := sm.accountTypes[address]
	if !ok {
		return 0, errors.New("account type not found")
	}

	return accType, nil
}

func (sm *MockStateManager) DeleteStateObject(addr types.Address) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) FlushDirtyObject(addrs types.Address) error {
	if sm.flushHook != nil {
		return sm.flushHook()
	}

	sm.flushedDirtyObjects[addrs] = true

	return nil
}

func (sm *MockStateManager) getFlushedDirtyObject(addrs types.Address) bool {
	_, ok := sm.flushedDirtyObjects[addrs]

	return ok
}

func (sm *MockStateManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	ts, ok := sm.latestTesseracts[addr]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	copyTS := *ts // copy, so that stored tesseract wont't be modified

	if !withInteractions {
		copyTS.Ixns = nil
	}

	return &copyTS, nil
}

func (sm *MockStateManager) insertContextNodes(
	ctxHash types.Hash,
	behaviouralNodes []id.KramaID,
	randomNodes ...id.KramaID,
) {
	sm.context[ctxHash] = &Context{
		behaviourNodes: behaviouralNodes,
		randomNodes:    randomNodes,
	}
}

func (sm *MockStateManager) GetContextByHash(
	addr types.Address,
	hash types.Hash,
) (
	types.Hash,
	[]id.KramaID,
	[]id.KramaID,
	error,
) {
	c, ok := sm.context[hash]

	if !ok {
		return types.NilHash, nil, nil, types.ErrContextStateNotFound
	}

	return hash, c.behaviourNodes, c.randomNodes, nil
}

func (sm *MockStateManager) GetPublicKeys(id ...id.KramaID) ([][]byte, error) {
	keys := make([][]byte, 0)

	for _, v := range id {
		key, ok := sm.publicKeys[v]
		if !ok {
			return nil, types.ErrKeyNotFound
		}

		keys = append(keys, key)
	}

	return keys, nil
}

func (sm *MockStateManager) insertPublicKeys(nodes []id.KramaID, pk [][]byte) {
	for i, kramaID := range nodes {
		sm.publicKeys[kramaID] = pk[i]
	}
}

func (sm *MockStateManager) Cleanup(addrs types.Address) {
	sm.cleanUp[addrs] = true
}

func (sm *MockStateManager) insertRegisteredAcc(addr types.Address) {
	sm.registeredAccounts[addr] = &AccHash{}
}

func (sm *MockStateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(types.NilHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func (sm *MockStateManager) IsAccountRegisteredAt(addr types.Address, tesseractHash types.Hash) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(tesseractHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func getContextLockHash(ts *types.Tesseract) types.Hash {
	return ts.Header.ContextLock[ts.Address()].ContextHash
}

func (sm *MockStateManager) FetchContextLock(ts *types.Tesseract) (*ktypes.ICSNodes, error) {
	ics := ktypes.NewICSNodes(6)

	_, behaviourSet, randomSet, err := sm.GetContextByHash(types.NilAddress, getContextLockHash(ts))
	if err != nil {
		return nil, err
	}

	// fetching public keys
	behaviouralPK, err := sm.GetPublicKeys(behaviourSet...)
	if err != nil {
		return nil, err
	}

	randomPK, err := sm.GetPublicKeys(randomSet...)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ktypes.SenderBehaviourSet, ktypes.NewNodeSet(behaviourSet, behaviouralPK))
	ics.UpdateNodeSet(ktypes.SenderRandomSet, ktypes.NewNodeSet(randomSet, randomPK))

	return ics, nil
}

func (sm *MockStateManager) FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	ts, ok := sm.dbTesseracts[hash]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract wont't be modified

	if !withInteractions {
		tsCopy.Ixns = nil
	}

	return &tsCopy, nil
}

func (sm *MockStateManager) SetupNewAccount(info *gtypes.AccountSetupArgs) (types.Hash, types.Hash, error) {
	if sm.newAccountHook != nil {
		return sm.newAccountHook()
	}

	return types.NilHash, types.NilHash, nil
}

func (sm *MockStateManager) SetupSargaAccount(
	sargaAcc *gtypes.AccountSetupArgs,
	otherAccounts []*gtypes.AccountSetupArgs,
) (types.Hash, types.Hash, error) {
	if sm.sargaAccountHook != nil {
		return sm.sargaAccountHook()
	}

	return types.NilHash, types.NilHash, nil
}

func (sm *MockStateManager) InsertLatestTesseracts(t *testing.T, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		sm.latestTesseracts[ts.Address()] = ts
	}
}

func (sm *MockStateManager) InsertTesseractsInDB(t *testing.T, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		sm.dbTesseracts[getTesseractHash(t, ts)] = ts
	}
}

func (sm *MockStateManager) setPublicKey(id id.KramaID, pk []byte) {
	sm.publicKeys[id] = pk
}

type MockIXPool struct {
	reset map[types.Hash]bool
}

type MockSenatus struct {
	NodeInclusivity       map[id.KramaID]int64
	UpdateInclusivityHook func() error
}

func (s *MockSenatus) UpdateInclusivity(key id.KramaID, delta int64) error {
	if s.UpdateInclusivityHook != nil {
		return s.UpdateInclusivityHook()
	}

	s.NodeInclusivity[key] += delta

	return nil
}

func (i *MockIXPool) ResetWithHeaders(ts *types.Tesseract) {
	tsHash, err := ts.Hash()
	if err != nil {
		panic(err)
	}

	i.reset[tsHash] = true
}

func (i *MockIXPool) IsReset(hash types.Hash) bool {
	if _, ok := i.reset[hash]; ok {
		return true
	}

	return false
}

type CreateIxParams struct {
	ixDataCallback func(ix *types.IxData)
}

func createIX(t *testing.T, params *CreateIxParams) *types.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &types.IxData{
		Input: types.IxInput{},
	}

	if params.ixDataCallback != nil {
		params.ixDataCallback(data)
	}

	ix := types.NewInteraction(*data, []byte{})

	return ix
}

func createIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) types.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make(types.Interactions, count)

	for i := 0; i < count; i++ {
		ixns[i] = createIX(t, paramsMap[i])
	}

	return ixns
}

func getIxParamsWithAddress(from types.Address, to types.Address) *CreateIxParams {
	return &CreateIxParams{
		ixDataCallback: func(ix *types.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
		},
	}
}

func getIxParamsMapWithAddresses(
	from []types.Address,
	to []types.Address,
) map[int]*CreateIxParams {
	ixParams := make(map[int]*CreateIxParams, len(from))

	for i := 0; i < len(from); i++ {
		ixParams[i] = getIxParamsWithAddress(from[i], to[i])
	}

	return ixParams
}

func mockCache(t *testing.T) *lru.Cache {
	t.Helper()

	cache, err := lru.New(1200)
	require.NoError(t, err)

	return cache
}

func getTestTesseractGrid(t *testing.T) *types.TesseractGridID {
	t.Helper()

	return &types.TesseractGridID{
		Hash: tests.RandomHash(t),
		Parts: &types.TesseractParts{
			Hashes: getHashes(t, 2, false),
		},
	}
}

func mockContextLock() map[types.Address]types.ContextLockInfo {
	return make(map[types.Address]types.ContextLockInfo)
}

func mockContextDelta() types.ContextDelta {
	return make(map[types.Address]*types.DeltaGroup)
}

func mockConsensusProof() types.PoXCData {
	return types.PoXCData{}
}

func mockCommitData() types.CommitData {
	return types.CommitData{}
}

func mockNetwork(t *testing.T) *MockNetwork {
	t.Helper()

	return &MockNetwork{}
}

func defaultCommitData() types.CommitData {
	commitData := mockCommitData()
	// vote-set is a bit array
	voteSet := types.ArrayOfBits{
		Size:     1,                 // represents node tsCount
		Elements: make([]uint64, 1), // each element holds eight votes
	}

	voteSet.Size = 5         // there are 5 ics nodes
	voteSet.Elements[0] = 31 // first 5 ics nodes voted yes

	commitData.VoteSet = voteSet.Copy()

	return commitData
}

type createTesseractParams struct {
	address  types.Address
	height   uint64
	ixns     types.Interactions
	callback func(ts *types.Tesseract)
}

func createTesseract(t *testing.T, params *createTesseractParams) *types.Tesseract {
	t.Helper()

	if params == nil {
		params = &createTesseractParams{}
	}

	if params.address == types.NilAddress {
		params.address = tests.RandomAddress(t)
	}

	ts := &types.Tesseract{
		Header: types.TesseractHeader{
			Address:     params.address,
			Height:      params.height,
			ContextLock: mockContextLock(),
			Extra:       defaultCommitData(),
		},
		Body: types.TesseractBody{
			ContextDelta:   mockContextDelta(),
			ConsensusProof: mockConsensusProof(),
		},
		Ixns: params.ixns,
	}

	if params.ixns != nil {
		hash, err := params.ixns.Hash()
		require.NoError(t, err)

		ts.Body.InteractionHash = hash
	}

	if params.callback != nil {
		params.callback(ts)
	}

	return ts
}

func createTesseracts(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*types.Tesseract {
	t.Helper()

	tesseracts := make([]*types.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	for i := 0; i < count; i++ {
		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func getAddresses(t *testing.T, count int) []types.Address {
	t.Helper()

	addresses := make([]types.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = tests.RandomAddress(t)
	}

	return addresses
}

func tesseractParamsWithICSClusterInfo(
	t *testing.T,
	ixns types.Interactions,
	icsInfo *ptypes.ICSClusterInfo,
) *createTesseractParams {
	t.Helper()

	rawData, err := icsInfo.Bytes()
	require.NoError(t, err)

	return &createTesseractParams{
		ixns: ixns,
		callback: func(ts *types.Tesseract) {
			ts.Body.ReceiptHash = tests.RandomHash(t)
			ts.Body.ConsensusProof.ICSHash = types.GetHash(rawData)
		},
	}
}

func tesseractParamsWithGridInfo(
	t *testing.T,
	address types.Address,
	stateHash, receiptHash types.Hash,
	clusterInfo *ptypes.ICSClusterInfo,
	ixns []*types.Interaction,
	gridSize int32,
) *createTesseractParams {
	t.Helper()

	rawBytes, err := clusterInfo.Bytes()
	require.NoError(t, err)

	return &createTesseractParams{
		address: address,
		ixns:    ixns,
		callback: func(ts *types.Tesseract) {
			ts.Header.Extra.GridID = getTestTesseractGrid(t)
			ts.Header.Extra.GridID.Parts.Total = gridSize
			ts.Body.ConsensusProof.ICSHash = types.GetHash(rawBytes)
			ts.Body.StateHash = stateHash
			ts.Body.ReceiptHash = receiptHash
			ts.Body.InteractionHash = tests.RandomHash(t) // make sure that nil hash won't be inserted as key
		},
	}
}

func tesseractParamsForExecution(
	t *testing.T,
	address types.Address,
	contextHash, stateHash types.Hash,
	ixns []*types.Interaction,
	receipts types.Receipts,
	clusterInfo *ptypes.ICSClusterInfo,
	gridSize int32,
) *createTesseractParams {
	t.Helper()

	rawBytes, err := clusterInfo.Bytes()
	require.NoError(t, err)

	return &createTesseractParams{
		address: address,
		ixns:    ixns,
		callback: func(ts *types.Tesseract) {
			// set header fields
			ts.Header.ContextLock[address] = types.ContextLockInfo{ContextHash: contextHash}
			ts.Header.Extra.GridID = getTestTesseractGrid(t)
			ts.Header.Extra.GridID.Parts.Total = gridSize
			ts.Header.Extra.CommitSignature = validCommitSign
			// set body fields
			ts.Body.StateHash = stateHash
			ts.Body.ReceiptHash = getReceiptHash(t, receipts)
			ts.Body.InteractionHash = tests.RandomHash(t) // make sure that nil hash won't be inserted as key
			ts.Body.ContextDelta[address] = getDeltaGroup(t, 2, 2, 0)
			ts.Body.ConsensusProof.ICSHash = types.GetHash(rawBytes)
		},
	}
}

func tesseractParamsWithContextDelta(
	t *testing.T,
	address types.Address,
	behaviouralCount, randomCount, replacedCount int,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		address: address,
		callback: func(ts *types.Tesseract) {
			ts.Body.ContextDelta[address] = getDeltaGroup(t, behaviouralCount, randomCount, replacedCount)
		},
	}
}

func tesseractParamsWithIxnsAndReceiptHash(
	t *testing.T,
	address types.Address,
	ixns types.Interactions,
	receipts types.Receipts,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		address: address,
		ixns:    ixns,
		callback: func(ts *types.Tesseract) {
			ts.Ixns = ixns
			ts.Body.InteractionHash = getInteractionsHash(t, ixns)
			ts.Body.ReceiptHash = getReceiptHash(t, receipts)
		},
	}
}

func tesseractParamsWithContextHash(
	t *testing.T,
	address types.Address,
	ctxHash types.Hash,
	sign []byte,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		address: address,
		callback: func(ts *types.Tesseract) {
			ts.Header.ContextLock[address] = types.ContextLockInfo{ContextHash: ctxHash}
			ts.Body.ContextDelta[address] = getDeltaGroup(t, 1, 1, 1)
			ts.Header.Extra.CommitSignature = sign
		},
	}
}

func tesseractParamsWithReceiptHash(t *testing.T, receiptHash types.Hash, gridHash types.Hash) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		callback: func(ts *types.Tesseract) {
			ts.Body.ReceiptHash = receiptHash
			ts.Header.GridHash = gridHash
		},
	}
}

func tesseractParamsWithStateHash(t *testing.T, stateHash types.Hash) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		callback: func(ts *types.Tesseract) {
			ts.Body.StateHash = stateHash
		},
	}
}

func getTSParamsMapWithStateHash(t *testing.T, paramsCount int) map[int]*createTesseractParams {
	t.Helper()

	tsParamsMap := make(map[int]*createTesseractParams)

	for i := 0; i < paramsCount; i++ {
		tsParamsMap[i] = &createTesseractParams{
			callback: func(ts *types.Tesseract) {
				ts.Body.StateHash = tests.RandomHash(t)
			},
		}
	}

	return tsParamsMap
}

func getTesseractCallbackWithCommitSign(commitSign []byte) func(ts *types.Tesseract) {
	return func(ts *types.Tesseract) {
		ts.Header.Extra.CommitSignature = commitSign
	}
}

func tesseractParamsWithCommitSign(commitSign []byte) *createTesseractParams {
	return &createTesseractParams{
		callback: getTesseractCallbackWithCommitSign(commitSign),
	}
}

func smCallbackWithRegisteredAcc(address types.Address) func(sm *MockStateManager) {
	return func(sm *MockStateManager) {
		sm.insertRegisteredAcc(address)
	}
}

func smCallbackWithAccRegistrationHook() func(sm *MockStateManager) {
	return func(sm *MockStateManager) {
		sm.accountRegistrationHook = func(hash types.Hash) (bool, error) {
			return false, types.ErrAccountNotFound
		}
	}
}

func getTesseractParamsMapWithIxns(t *testing.T, tsCount int) map[int]*createTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*createTesseractParams, tsCount)
	addresses := getAddresses(t, 4*tsCount) // for each interaction, sender and receiver addresses needed
	ixns := createIxns(t, 2*tsCount, getIxParamsMapWithAddresses(addresses[:2*tsCount], addresses[2*tsCount:]))

	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &createTesseractParams{
			ixns: ixns[i*2 : i*2+2], // allocate two interactions per tesseract
		}
	}

	return tesseractParams
}

type CreateChainParams struct {
	db                   *MockDB
	sm                   *MockStateManager
	senatus              *MockSenatus
	ixPool               *MockIXPool
	dbCallback           func(db *MockDB)
	ixPoolCallback       func(ixPool *MockIXPool)
	smCallBack           func(sm *MockStateManager)
	senatusCallback      func(senatus *MockSenatus)
	networkCallback      func(network *MockNetwork)
	execCallback         func(exec *MockExec)
	chainManagerCallback func(c *ChainManager)
}

func mockChainConfig() *common.ChainConfig {
	return &common.ChainConfig{}
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		NodeInclusivity: make(map[id.KramaID]int64),
	}
}

func mockIXPool(t *testing.T) *MockIXPool {
	t.Helper()

	return &MockIXPool{
		reset: make(map[types.Hash]bool),
	}
}

func createTestChainManager(t *testing.T, params *CreateChainParams) *ChainManager {
	t.Helper()

	ctx := context.Background()

	if params == nil {
		params = &CreateChainParams{}
	}

	var (
		db      = mockDB(t)
		sm      = mockStateManager(t)
		senatus = mockSenatus(t)
		ixPool  = mockIXPool(t)
		exec    = mockExec(t)
		network = mockNetwork(t)
	)

	if params.senatus != nil {
		senatus = params.senatus
	}

	if params.sm != nil {
		sm = params.sm
	}

	if params.db != nil {
		db = params.db
	}

	if params.ixPool != nil {
		ixPool = params.ixPool
	}

	if params.ixPoolCallback != nil {
		params.ixPoolCallback(ixPool)
	}

	if params.smCallBack != nil {
		params.smCallBack(sm)
	}

	if params.senatusCallback != nil {
		params.senatusCallback(senatus)
	}

	if params.networkCallback != nil {
		params.networkCallback(network)
	}

	if params.execCallback != nil {
		params.execCallback(exec)
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	c, err := NewChainManager(
		ctx,
		mockChainConfig(),
		db,
		sm,
		hclog.NewNullLogger(),
		&utils.TypeMux{},
		network,
		ixPool,
		mockCache(t),
		exec,
		senatus,
		NilMetrics(),
		MockAggregateSignVerifier,
	)
	require.NoError(t, err)

	if params.chainManagerCallback != nil {
		params.chainManagerCallback(c)
	}

	return c
}

func fetchContextFromLattice(t *testing.T, ts types.Tesseract, c *ChainManager) []id.KramaID {
	t.Helper()

	var (
		address = ts.Address()
		peers   = make([]id.KramaID, 0)
	)

	for {
		if len(peers) >= 10 {
			break
		}

		delta := ts.Body.ContextDelta[address]
		peers = append(peers, delta.BehaviouralNodes...)
		peers = append(peers, delta.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(address, ts.Header.ContextLock[address].ContextHash)
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.PreviousHash().IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.PreviousHash(), false)
		if err != nil {
			return nil
		}

		ts = *t
	}

	return peers
}

func makeChain(t *testing.T, tesseracts []*types.Tesseract) {
	t.Helper()

	for i, ts := range tesseracts {
		if i == 0 {
			continue
		}

		if ts.Address() == tesseracts[i-1].Address() {
			ts.Header.PrevHash = getTesseractHash(t, tesseracts[i-1])
		}
	}
}

func getTesseractHash(t *testing.T, ts *types.Tesseract) types.Hash {
	t.Helper()

	hash, err := ts.Hash()
	require.NoError(t, err)

	return hash
}

func insertTesseractsInDB(t *testing.T, db db, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		bytes, err := ts.Bytes()
		require.NoError(t, err)

		err = db.SetTesseract(getTesseractHash(t, ts), bytes)
		require.NoError(t, err)
	}
}

func getDeltaGroup(t *testing.T, behaviouralCount int, randomCount int, replaceCount int) *types.DeltaGroup {
	t.Helper()

	return &types.DeltaGroup{
		Role:             1,
		BehaviouralNodes: tests.GetTestKramaIDs(t, behaviouralCount),
		RandomNodes:      tests.GetTestKramaIDs(t, randomCount),
		ReplacedNodes:    tests.GetTestKramaIDs(t, replaceCount),
	}
}

func getContextLockInfo(contextHash types.Hash, tsHash types.Hash, height uint64) types.ContextLockInfo {
	return types.ContextLockInfo{
		ContextHash:   contextHash,
		Height:        height,
		TesseractHash: tsHash,
	}
}

func setContextDelta(ts *types.Tesseract, address types.Address, delta *types.DeltaGroup) {
	ts.Body.ContextDelta[address] = delta
}

func setContextLock(ts *types.Tesseract, address types.Address, info types.ContextLockInfo) {
	ts.Header.ContextLock[address] = info
}

func insertTesseractsInCache(t *testing.T, c *ChainManager, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		c.tesseracts.Add(getTesseractHash(t, ts), ts)
	}
}

func getPublicKeys(t *testing.T, count int) [][]byte {
	t.Helper()

	var pk [][]byte

	for i := 0; i < count; i++ {
		addr := tests.RandomAddress(t).Bytes()
		pk = append(pk, addr)
	}

	return pk
}

// if randomSet is empty random set is filled with random nodes
func getTestClusterInfoWithRandomSet(t *testing.T, randomSet []id.KramaID, idCount int) *ptypes.ICSClusterInfo {
	t.Helper()

	info := new(ptypes.ICSClusterInfo)

	if len(randomSet) == 0 {
		info.RandomSet = utils.KramaIDToString(tests.GetTestKramaIDs(t, idCount))
	} else {
		info.RandomSet = utils.KramaIDToString(randomSet)
	}

	return info
}

func insertTesseractByHeight(t *testing.T, db db, ts *types.Tesseract) {
	t.Helper()

	err := db.SetTesseractHeightEntry(ts.Address(), ts.Height(), getTesseractHash(t, ts))
	require.NoError(t, err)
}

func insertAssetDataByAssetHashInDB(t *testing.T, db db, assetHash types.Hash, assetData []byte) {
	t.Helper()

	err := db.CreateEntry(assetHash.Bytes(), assetData)
	require.NoError(t, err)
}

func getICSNodeset(t *testing.T, count int) *ktypes.ICSNodes {
	t.Helper()

	ics := ktypes.NewICSNodes(6)

	senderBehaviourSet := tests.GetTestKramaIDs(t, count)
	senderRandomSet := tests.GetTestKramaIDs(t, count)
	receiverBehaviourSet := tests.GetTestKramaIDs(t, count)
	receiverRandomSet := tests.GetTestKramaIDs(t, count)
	randomNodes := tests.GetTestKramaIDs(t, count)

	ics.UpdateNodeSet(ktypes.SenderBehaviourSet, ktypes.NewNodeSet(senderBehaviourSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(ktypes.SenderRandomSet, ktypes.NewNodeSet(senderRandomSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(ktypes.ReceiverBehaviourSet, ktypes.NewNodeSet(receiverBehaviourSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(ktypes.ReceiverRandomSet, ktypes.NewNodeSet(receiverRandomSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(ktypes.RandomSet, ktypes.NewNodeSet(randomNodes, getPublicKeys(t, count)))

	return ics
}

func getTestDirtyEntriesWithClusterInfo(
	t *testing.T,
	clusterInfo *ptypes.ICSClusterInfo,
	count int,
) map[types.Hash][]byte {
	t.Helper()

	d := make(map[types.Hash][]byte, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t)] = tests.RandomAddress(t).Bytes()
	}

	if clusterInfo != nil {
		rawData, err := clusterInfo.Bytes()
		require.NoError(t, err)

		d[types.GetHash(rawData)] = rawData
	}

	return d
}

// MockAggregateSignVerifier returns true if first byte is true else return false
func MockAggregateSignVerifier(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if aggSignature[0] == 1 {
		return true, nil
	}

	return false, types.ErrSignatureVerificationFailed
}

func getReceipt(ixHash types.Hash) *types.Receipt {
	return &types.Receipt{
		IxType:        1,
		IxHash:        ixHash,
		FuelUsed:      rand.Uint64(),
		StateHashes:   make(map[types.Address]types.Hash),
		ContextHashes: make(map[types.Address]types.Hash),
		ExtraData:     nil,
	}
}

func getIX(t *testing.T) *types.Interaction {
	t.Helper()

	return createIX(
		t,
		getIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t)),
	)
}

func getIxAndReceipt(t *testing.T) (*types.Interaction, *types.Receipt) {
	t.Helper()
	ix := getIX(t)

	return ix, getReceipt(ix.Hash())
}

func getIxAndReceipts(t *testing.T, ixCount int) ([]*types.Interaction, types.Receipts) {
	t.Helper()

	var ixs []*types.Interaction

	receipts := make(map[types.Hash]*types.Receipt, ixCount)

	for i := 0; i < ixCount; i++ {
		ix, r := getIxAndReceipt(t)
		ixs = append(ixs, ix)
		receipts[ix.Hash()] = r
	}

	return ixs, receipts
}

func getIxAndReceiptsWithStateHash(
	t *testing.T,
	stateHash map[types.Address]types.Hash,
	ixCount int,
) ([]*types.Interaction, types.Receipts) {
	t.Helper()

	var (
		ixs      []*types.Interaction
		receipts = make(map[types.Hash]*types.Receipt, ixCount)
	)

	for i := 0; i < ixCount; i++ {
		ix, r := getIxAndReceipt(t)
		r.StateHashes = stateHash

		ixs = append(ixs, ix)

		receipts[ix.Hash()] = r
	}

	return ixs, receipts
}

func insertIxLookup(t *testing.T, db db, ixHash types.Hash, receipts types.Receipts) {
	t.Helper()

	err := db.SetIxLookup(ixHash, getReceiptHash(t, receipts).Bytes())
	require.NoError(t, err)
}

func insertReceipts(t *testing.T, db db, receipts types.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = db.SetReceipts(getReceiptHash(t, receipts), rawData)
	require.NoError(t, err)
}

func getReceiptHash(t *testing.T, receipts types.Receipts) types.Hash {
	t.Helper()

	hash, err := receipts.Hash()
	require.NoError(t, err)

	return hash
}

func getInteractionsHash(t *testing.T, ixns types.Interactions) types.Hash {
	t.Helper()

	hash, err := ixns.Hash()
	require.NoError(t, err)

	return hash
}

func getHashes(t *testing.T, count int, nilHash bool) []types.Hash {
	t.Helper()

	var hashes []types.Hash

	for i := 0; i < count; i++ {
		if nilHash {
			hashes = append(hashes, types.NilHash)
		} else {
			hashes = append(hashes, tests.RandomHash(t))
		}
	}

	return hashes
}

func getTestClusterInfo(t *testing.T, nodeCount int) *ptypes.ICSClusterInfo {
	t.Helper()

	info := new(ptypes.ICSClusterInfo)
	info.RandomSet = utils.KramaIDToString(tests.GetTestKramaIDs(t, nodeCount))
	info.ObserverSet = utils.KramaIDToString(tests.GetTestKramaIDs(t, nodeCount))

	return info
}

// signs tesseract and stores signed bytes in seal also stores the public key associated with krama id
func signTesseract(t *testing.T, sm *MockStateManager, ts *types.Tesseract) (signer id.KramaID) {
	t.Helper()

	rawData, err := ts.Bytes()
	require.NoError(t, err)

	seal, pk := signBytes(t, rawData) // calculate the seal of tesseract
	ts.Seal = seal                    // store seal in tesseract
	ids := tests.GetTestKramaIDs(t, 1)

	sm.setPublicKey(ids[0], pk) // store the public key for the signed sender

	return ids[0]
}

func signBytes(t *testing.T, msg []byte) (sigBytes, pk []byte) {
	t.Helper()

	// create keystore.json in current directory
	dataDir := "./"
	password := "test123"

	_, _, err := poi.RandGenKeystore(dataDir, password)
	require.NoError(t, err)

	config := &mudra.VaultConfig{
		DataDir:       dataDir,
		NodePassword:  password,
		MoiIDUsername: "",
		MoiIDPassword: "",
		MoiIDURL:      "dev",
	}

	vault, err := mudra.NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	// gets the public key of signer
	pk = vault.GetConsensusPrivateKey().GetPublicKeyInBytes()

	// signs the bytes
	sigBytes, err = vault.Sign(msg, mudracommon.BlsBLST)
	require.NoError(t, err)

	// remove keystore.json in current directory
	err = os.Remove("./keystore.json")
	require.NoError(t, err)

	return sigBytes, pk
}

func insertValidatedTesseracts(t *testing.T, c *ChainManager, tesseracts []*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		c.validatedTesseracts.Store(getTesseractHash(t, ts), nil)
	}
}

func setAccountType(sm *MockStateManager, accType types.AccountType, tesseracts ...*types.Tesseract) {
	for _, ts := range tesseracts {
		sm.setAccType(ts.Address(), accType)
	}
}

func getAssetInfo(t *testing.T, symbol string, assetKind int) *AssetInfo {
	t.Helper()

	return &AssetInfo{
		Type:        assetKind,
		Symbol:      symbol,
		Owner:       tests.RandomAddress(t).Hex(),
		TotalSupply: 10000,

		Dimension: 1,
		Decimals:  2,

		IsFungible:     true,
		IsMintable:     true,
		IsTransferable: true,

		LogicID: "abcd",
	}
}

func getBalanceInfo(assetID string, amount int64) *BalanceInfo {
	return &BalanceInfo{
		AssetID: assetID,
		Amount:  amount,
	}
}

// waitForResponse waits for response on respChannel
// and checks if datatype of data received on channel is equal to datatype of data received as argument
func waitForResponse(t *testing.T, respChan chan result, data interface{}) interface{} {
	t.Helper()

	res := <-respChan
	require.NoError(t, res.err)

	require.Equal(t, reflect.TypeOf(res.data), reflect.TypeOf(data))

	return res.data
}

// handleMuxEvents sends the data to resp channel if it receives data on subscription channel
// sends time out error when context is closed
func handleMuxEvents(ctx context.Context, s *utils.Subscription, resp chan result) {
	for {
		select {
		case <-ctx.Done():
			resp <- result{data: nil, err: types.ErrTimeOut}

			return
		case data := <-s.Chan():
			resp <- result{data: data.Data, err: nil}

			return
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// getTesseractSyncEvent extracts TesseractSyncEvent from interface
func getTesseractSyncEvent(t *testing.T, data interface{}) utils.TesseractSyncEvent {
	t.Helper()

	event, ok := data.(utils.TesseractSyncEvent)
	require.True(t, ok)

	return event
}

// getTesseractAddedEvent extracts TesseractAddedEvent from interface
func getTesseractAddedEvent(t *testing.T, data interface{}) utils.TesseractAddedEvent {
	t.Helper()

	event, ok := data.(utils.TesseractAddedEvent)
	require.True(t, ok)

	return event
}

// getSyncStatusUpdate extracts SyncStatusUpdate from interface
func getSyncStatusUpdate(t *testing.T, data interface{}) utils.SyncStatusUpdate {
	t.Helper()

	event, ok := data.(utils.SyncStatusUpdate)
	require.True(t, ok)

	return event
}

func getPubSubMsg(t *testing.T, ts *types.Tesseract, sender id.KramaID, info *ptypes.ICSClusterInfo) *pubsub.Message {
	t.Helper()

	msg := new(ptypes.TesseractMessage)
	msg.Delta = make(map[types.Hash][]byte)

	msg.CanonicalTesseract = ts.Canonical()
	msg.Sender = sender
	rawIxns, err := ts.Ixns.Bytes()
	require.NoError(t, err)

	msg.Ixns = rawIxns

	rawInfo, err := info.Bytes()
	require.NoError(t, err)

	msg.Delta[ts.GetICSHash()] = rawInfo

	tsMsg, err := msg.Bytes()
	require.NoError(t, err)

	message := &pb.Message{Data: tsMsg}
	pub := &pubsub.Message{Message: message}

	return pub
}

func getAccountInfo(
	t *testing.T,
	address string,
	accType types.AccountType,
	moiID string,
	behaviourContext []string,
	randomContext []string,
	assetDetails []*AssetInfo,
	balances []*BalanceInfo,
) AccountInfo {
	t.Helper()

	return AccountInfo{
		Address:          address,
		AccountType:      accType,
		MOIId:            moiID,
		BehaviourContext: behaviourContext,
		RandomContext:    randomContext,
		AssetDetails:     assetDetails,
		Balances:         balances,
	}
}

// createMockGenesisFile is a mock function used to create genesis file
func createMockGenesisFile(
	t *testing.T,
	dir string,
	invalidData bool,
	sargaAccount AccountInfo,
	accInfo []AccountInfo,
) string {
	t.Helper()

	var (
		data []byte
		err  error
	)

	genesis := &Genesis{
		SargaAccount: sargaAccount,
		Accounts:     accInfo,
	}

	if invalidData {
		data = []byte{1, 2, 3}
	} else {
		data, err = json.MarshalIndent(genesis, "", " ")
		require.NoError(t, err)
	}

	file, err := ioutil.TempFile(dir, "*config.json")
	require.NoError(t, err)

	_, err = file.Write(data)
	require.NoError(t, err)

	return file.Name()
}

func getTestAccountWithAccType(t *testing.T, accType types.AccountType) AccountInfo {
	t.Helper()

	ids := tests.GetTestKramaIDs(t, 4)
	ctx := utils.KramaIDToString(ids)
	address := tests.RandomAddress(t).Hex()

	return getAccountInfo(
		t,
		address,
		accType,
		"moi-id",
		ctx[:2],
		ctx[2:4],
		[]*AssetInfo{
			getAssetInfo(t, "btc", 1),
			getAssetInfo(t, "eth", 1),
		},
		[]*BalanceInfo{
			getBalanceInfo(
				"0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
				8333300,
			),
			getBalanceInfo(
				"0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9fcd84abca",
				1222100,
			),
		},
	)
}

// validation

func validateNodeInclusivity(t *testing.T, senatus *MockSenatus, ctxDelta types.ContextDelta) {
	t.Helper()

	for _, deltaGroup := range ctxDelta {
		for _, kramaID := range deltaGroup.BehaviouralNodes {
			require.Equal(t, senatus.NodeInclusivity[kramaID], int64(1))
		}

		for _, kramaID := range deltaGroup.RandomNodes {
			require.Equal(t, senatus.NodeInclusivity[kramaID], int64(1))
		}

		for _, kramaID := range deltaGroup.ReplacedNodes {
			require.Equal(t, senatus.NodeInclusivity[kramaID], int64(-1))
		}
	}
}

func checkForTSInCache(t *testing.T, c *ChainManager, ts *types.Tesseract, exists bool) {
	t.Helper()

	if exists { // check if cache has both tesseract address and hash
		hash, ok := c.tesseracts.Get(ts.Address())
		require.True(t, ok)
		require.Equal(t, getTesseractHash(t, ts), hash)

		cachedTS, ok := c.tesseracts.Get(getTesseractHash(t, ts))
		require.True(t, ok)
		require.Equal(t, ts.GetTesseractWithoutIxns(), cachedTS)

		return
	}

	_, ok := c.tesseracts.Get(ts.Address()) // make sure tesseract address not present in cache
	require.False(t, ok)
}

func checkForCanonicalTSInDB(t *testing.T, c *ChainManager, expectedTS *types.Tesseract) {
	t.Helper()

	// check if tesseract matches
	rawData, err := c.db.GetTesseract(getTesseractHash(t, expectedTS))
	require.NoError(t, err)

	canonicalTS := new(types.CanonicalTesseract)
	err = canonicalTS.FromBytes(rawData)
	require.NoError(t, err)

	require.Equal(t, expectedTS.Canonical(), canonicalTS)
}

func checkForIxnsInDB(t *testing.T, c *ChainManager, expectedTS *types.Tesseract) {
	t.Helper()

	// check if tesseract matches
	rawData, err := c.db.GetTesseract(expectedTS.InteractionHash())
	require.NoError(t, err)

	actualIxns := new(types.Interactions)
	err = actualIxns.FromBytes(rawData)
	require.NoError(t, err)

	require.Equal(t, expectedTS.Ixns, *actualIxns)
}

func checkForTesseractByHeight(t *testing.T, c *ChainManager, ts *types.Tesseract) {
	t.Helper()

	// check if tesseract height key added
	_, err := c.db.GetTesseractHeightEntry(ts.Address(), ts.Height())
	require.NoError(t, err)
}

func checkIfAccMetaInfoMatches(
	t *testing.T,
	accMetaInfo *types.AccountMetaInfo,
	ts *types.Tesseract,
	accType types.AccountType,
	latticeExists bool,
	stateExists bool,
) {
	t.Helper()

	require.Equal(t, ts.Address(), accMetaInfo.Address)
	require.Equal(t, new(big.Int).SetUint64(ts.Height()), accMetaInfo.Height)
	require.Equal(t, getTesseractHash(t, ts), accMetaInfo.TesseractHash)
	require.Equal(t, accType, accMetaInfo.Type)
	require.Equal(t, latticeExists, accMetaInfo.LatticeExists)
	require.Equal(t, stateExists, accMetaInfo.StateExists)
}

func checkIfTesseractAdded(
	t *testing.T,
	c *ChainManager,
	db *MockDB,
	sm *MockStateManager,
	senatus *MockSenatus,
	ixPool *MockIXPool,
	args testTSArgs,
	count int,
	latticeExists bool,
	accType types.AccountType,
	ts *types.Tesseract,
) {
	t.Helper()

	// cache should be checked first
	// otherwise tesseract may get added to cache while doing other validations
	checkForTSInCache(t, c, ts, args.cache)
	checkForCanonicalTSInDB(t, c, ts)
	checkForIxnsInDB(t, c, ts)
	validateNodeInclusivity(t, senatus, ts.ContextDelta())
	checkForTesseractByHeight(t, c, ts)

	if args.stateExists {
		found := sm.getFlushedDirtyObject(ts.Address())
		require.True(t, found)
	}

	// check if acc meta info inserted
	accMetaInfo, found := db.GetAccMetaInfo(ts.Address())
	require.True(t, found)

	checkIfAccMetaInfoMatches(
		t,
		accMetaInfo,
		ts,
		accType,
		latticeExists,
		args.tesseractExists,
	)

	require.True(t, sm.isCleanup(ts.Address()))              // check if dirty objects cleaned up
	require.True(t, ixPool.IsReset(getTesseractHash(t, ts))) // check if interactions are reset
}

func checkIfICSNodeSetMatches(
	t *testing.T,
	ics *ktypes.ICSNodes,
	senderBehaviourSet *ktypes.NodeSet,
	senderRandomSet *ktypes.NodeSet,
	randomSet *ktypes.NodeSet,
) {
	t.Helper()

	require.Equal(t,
		ics.Nodes[ktypes.SenderBehaviourSet],
		senderBehaviourSet,
	)
	require.Equal(t,
		ics.Nodes[ktypes.SenderRandomSet],
		senderRandomSet,
	)
	require.Equal(t,
		ics.Nodes[ktypes.RandomSet],
		randomSet,
	)
}

func checkIfTesseractCachedInCM(t *testing.T, c *ChainManager, withInteractions bool, tsHash types.Hash) {
	t.Helper()

	tesseractData, isCached := c.tesseracts.Get(tsHash)
	if withInteractions { // If fetched without interactions Make sure tesseract added to cache
		require.False(t, isCached) // makesure tesseract not cached if added with interaction

		return
	}

	require.True(t, isCached)

	cachedTS, ok := tesseractData.(*types.Tesseract)
	require.True(t, ok)

	// make sure tesseract in cache doesn't have interactions
	require.Equal(t, 0, len(cachedTS.Ixns))
}

func checkForTesseracts(t *testing.T, expectedTS, actualTS *types.Tesseract, withInteractions bool) {
	t.Helper()

	if withInteractions {
		require.Equal(t, expectedTS, actualTS)

		return
	}

	require.Equal(t, expectedTS.Canonical(), actualTS.Canonical())
	require.Nil(t, actualTS.Ixns)
}

func checkForValidatedTesseracts(t *testing.T, c *ChainManager, exists bool, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		_, ok := c.validatedTesseracts.Load(getTesseractHash(t, ts))
		if exists {
			require.True(t, ok)

			continue
		}

		require.False(t, ok)
	}
}

func checkForClusterInfo(t *testing.T, db db, clusterInfo *ptypes.ICSClusterInfo) {
	t.Helper()

	rawInfo, err := clusterInfo.Bytes()
	require.NoError(t, err)

	fetchedData, err := db.ReadEntry(types.GetHash(rawInfo).Bytes())
	require.NoError(t, err)

	require.Equal(t, rawInfo, fetchedData)
}

func checkForDirtyEntries(t *testing.T, db db, dirtyEntries map[types.Hash][]byte) {
	t.Helper()

	// check if dirty entries inserted in db
	for k, v := range dirtyEntries {
		val, err := db.ReadEntry(k.Bytes())
		require.NoError(t, err)
		require.Equal(t, v, val)
	}
}

func checkForAddedTesseracts(
	t *testing.T,
	checkValidatedTSCache bool,
	c *ChainManager,
	ts []*types.Tesseract,
	accType types.AccountType,
) {
	t.Helper()

	checkIfTesseractsAdded(
		t,
		accType,
		false,
		true,
		c.db.(*MockDB), //nolint
		ts...,
	)

	if checkValidatedTSCache {
		// check if tesseracts removed from validated tesseracts
		checkForValidatedTesseracts(t, c, false, ts...)
	}
}

func checkForOrphanTSInCache(t *testing.T, c *ChainManager, expectedTS *types.Tesseract) {
	t.Helper()

	// check if cache has both tesseract address and hash
	actualTS, ok := c.orphanTesseracts.Get(getTesseractHash(t, expectedTS))
	require.True(t, ok)
	require.Equal(t, expectedTS, actualTS)
}

func fetchContextWithNodes(t *testing.T, c *ChainManager, ts *types.Tesseract, randomset []string) []id.KramaID {
	t.Helper()

	fetchContext := fetchContextFromLattice(t, *ts, c)

	return append(fetchContext, utils.KramaIDFromString(randomset)...)
}

func checkIfTSSyncEventsMatch(
	t *testing.T,
	ts *types.Tesseract,
	event utils.TesseractSyncEvent,
	randomContext []id.KramaID,
) {
	t.Helper()

	require.Equal(t, ts, event.Tesseract)
	require.Equal(t, randomContext, event.Context)
}

func validateTSSyncEvent(
	t *testing.T,
	c *ChainManager,
	ts *types.Tesseract,
	resp chan result,
	info *ptypes.ICSClusterInfo,
) {
	t.Helper()

	eventData := waitForResponse(t, resp, utils.TesseractSyncEvent{}) // waits for eventData from goroutine
	checkIfTSSyncEventsMatch(
		t,
		ts,
		getTesseractSyncEvent(t, eventData), // convert interface type to concrete type
		fetchContextWithNodes(t, c, ts, info.RandomSet),
	)
}

func validateTSAddedEvent(t *testing.T, tsAddedResp chan result, ts *types.Tesseract) {
	t.Helper()

	data := waitForResponse(t, tsAddedResp, utils.TesseractAddedEvent{}) // waits for data from goroutine
	event := getTesseractAddedEvent(t, data)                             // convert interface type to concrete type
	require.Equal(t, ts, event.Tesseract)
}

func validateSyncStatusEvent(t *testing.T, syncStatusResp chan result) {
	t.Helper()

	data := waitForResponse(t, syncStatusResp, utils.SyncStatusUpdate{}) // waits for data from goroutine
	syncStatusUpdate := getSyncStatusUpdate(t, data)                     // convert interface type to concrete type
	require.Equal(t, int32(8), syncStatusUpdate.BucketID)                // 8 is hard coded
	require.Equal(t, int64(1), syncStatusUpdate.Count)
}

func checkIfTesseractsAdded(
	t *testing.T,
	accType types.AccountType,
	latticeExists, stateExists bool,
	db *MockDB,
	ts ...*types.Tesseract,
) {
	t.Helper()

	for i := 0; i < len(ts); i++ {
		// check if acc meta info inserted
		accMetaInfo, found := db.GetAccMetaInfo(ts[i].Address())
		require.True(t, found)

		checkIfAccMetaInfoMatches(
			t,
			accMetaInfo,
			ts[i],
			accType, // check accType to make sure tesseract added with state
			latticeExists,
			stateExists,
		)
	}
}

func checkForAccountCreation(t *testing.T, accountInfo AccountInfo, accSetupArgs *gtypes.AccountSetupArgs) {
	t.Helper()

	require.Equal(t, accountInfo.AccountType, accSetupArgs.AccType)
	require.Equal(t, accountInfo.Address, accSetupArgs.Address.Hex())
	require.Equal(t, accountInfo.MOIId, accSetupArgs.MoiID)
	require.Equal(t, accountInfo.BehaviourContext, utils.KramaIDToString(accSetupArgs.BehaviouralContext))
	require.Equal(t, accountInfo.RandomContext, utils.KramaIDToString(accSetupArgs.RandomContext))

	for i, assetDetails := range accountInfo.AssetDetails {
		require.Equal(t, types.AssetKind(assetDetails.Type), accSetupArgs.Assets[i].Type)
		require.Equal(t, assetDetails.Symbol, accSetupArgs.Assets[i].Symbol)
		require.Equal(t, assetDetails.Owner, accSetupArgs.Assets[i].Owner.Hex())
		require.Equal(t, assetDetails.TotalSupply, accSetupArgs.Assets[i].Supply.Uint64())
		require.Equal(t, assetDetails.Dimension, accSetupArgs.Assets[i].Dimension)
		require.Equal(t, assetDetails.Decimals, accSetupArgs.Assets[i].Decimals)
		require.Equal(t, assetDetails.IsFungible, accSetupArgs.Assets[i].IsFungible)
		require.Equal(t, assetDetails.IsMintable, accSetupArgs.Assets[i].IsMintable)
		require.Equal(t, assetDetails.IsTransferable, accSetupArgs.Assets[i].IsTransferable)
		require.Equal(t, assetDetails.LogicID, accSetupArgs.Assets[i].LogicID.Hex())
	}

	for _, balance := range accountInfo.Balances {
		amount, ok := accSetupArgs.Balances[types.AssetID(strings.TrimPrefix(balance.AssetID, "0x"))]
		require.True(t, ok)
		require.Equal(t, balance.Amount, amount.Int64())
	}
}

// checkForGenesisTesseract fetches added tesseract and checks if it valid
func checkForGenesisTesseract(
	t *testing.T,
	c *ChainManager,
	address types.Address,
	stateHash types.Hash,
	contextHash types.Hash,
	contextDelta types.ContextDelta,
) {
	t.Helper()

	hashData, ok := c.tesseracts.Get(address) // fetch tesseract hash from cache
	require.True(t, ok)

	tsHash, ok := hashData.(types.Hash)
	require.True(t, ok)

	ok, err := c.db.HasTesseract(tsHash)
	require.NoError(t, err)
	require.True(t, ok)

	ts, err := c.GetTesseract(tsHash, false)
	require.NoError(t, err)

	require.Equal(t, address, ts.Address())
	require.Equal(t, stateHash, ts.StateHash())
	require.Equal(t, contextHash, ts.ContextHash())
	require.Equal(t, contextDelta, ts.ContextDelta())
}
