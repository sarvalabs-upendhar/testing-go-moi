package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/hexutil"

	gorillaWS "github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/state"
)

var serverAddr = &net.TCPAddr{
	IP:   net.IPv4(192, 168, 1, 100),
	Port: 8080,
}

type MockMessage struct {
	messageType int
	data        []byte
}

type MockWSConn struct {
	mtx     sync.Mutex
	message chan *MockMessage
}

type MockConnManager struct {
	wsConn   *MockWSConn
	filterID string
}

func NewMockConnectionManager() *MockConnManager {
	return &MockConnManager{
		wsConn: &MockWSConn{
			message: make(chan *MockMessage),
		},
	}
}

func (mc *MockConnManager) HasConn() bool {
	return mc.wsConn != nil
}

func (mc *MockConnManager) WriteMessage(messageType int, data []byte) error {
	mc.wsConn.mtx.Lock()
	defer mc.wsConn.mtx.Unlock()

	if mc.wsConn != nil {
		mc.wsConn.message <- &MockMessage{
			messageType,
			data,
		}

		return nil
	}

	return errors.New("no message")
}

func (mc *MockConnManager) readMessage() chan *MockMessage {
	return mc.wsConn.message
}

func (mc *MockConnManager) SetFilterID(filterID string) {
	mc.filterID = filterID
}

func (mc *MockConnManager) GetFilterID() string {
	return mc.filterID
}

type MockChainManager struct {
	tesseractsByHash map[common.Hash]*common.Tesseract
	TSHashByHeight   map[string]common.Hash
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.tesseractsByHash = make(map[common.Hash]*common.Tesseract)
	mockChain.TSHashByHeight = make(map[string]common.Hash)

	return mockChain
}

func (m *MockChainManager) GetInteractionAndParticipantsByIxHash(ixHash common.Hash) (*common.Interaction,
	common.Hash, common.ParticipantsState, int, error,
) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) GetInteractionAndParticipantsByTSHash(tsHash common.Hash, ixIndex int) (*common.Interaction,
	common.ParticipantsState, error,
) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) GetLatestTesseract(_ identifiers.Address, _ bool) (*common.Tesseract, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) setTesseractByHash(
	t *testing.T,
	ts *common.Tesseract,
) {
	t.Helper()

	m.tesseractsByHash[tests.GetTesseractHash(t, ts)] = ts
}

func (m *MockChainManager) GetTesseract(
	hash common.Hash,
	withInteractions bool,
	withCommitInfo bool,
) (*common.Tesseract, error) {
	ts, ok := m.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	if !withCommitInfo {
		tsCopy = *tsCopy.GetTesseractWithoutCommitInfo()
	}

	return &tsCopy, nil
}

func (m *MockChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) setTesseractHeightEntry(address identifiers.Address, height uint64, hash common.Hash) {
	key := address.Hex() + strconv.FormatUint(height, 10)
	m.TSHashByHeight[key] = hash
}

func (m *MockChainManager) GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error) {
	key := address.Hex() + strconv.FormatUint(height, 10)

	if hash, ok := m.TSHashByHeight[key]; ok {
		return hash, nil
	}

	return common.NilHash, common.ErrKeyNotFound
}

type MockStateManager struct {
	storage     map[common.Hash][]byte
	accMetaInfo map[identifiers.Address]*common.AccountMetaInfo
}

func (m *MockStateManager) GetMandates(address identifiers.Address, hash common.Hash) ([]common.AssetMandate, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) FetchIxStateObjects(ixns common.Interactions,
	hashes map[identifiers.Address]common.Hash,
) (*state.Transition, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) CreateStateObject(address identifiers.Address,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) IsAccountRegistered(address identifiers.Address) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)
	mockState.storage = make(map[common.Hash][]byte)
	mockState.accMetaInfo = make(map[identifiers.Address]*common.AccountMetaInfo)

	return mockState
}

func (m *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetContextByHash(
	address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalances(addrs identifiers.Address, stateHash common.Hash) (common.AssetMap, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalance(addr identifiers.Address, assetID identifiers.AssetID, stateHash common.Hash,
) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetAccountState(addr identifiers.Address, stateHash common.Hash) (*common.Account, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetLogicManifest(_ identifiers.LogicID, _ common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetPersistentStorageEntry(_ identifiers.LogicID, _ []byte, _ common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetEphemeralStorageEntry(
	_ identifiers.Address, _ identifiers.LogicID, _ []byte, _ common.Hash,
) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	address identifiers.Address,
	accMetaInfo *common.AccountMetaInfo,
) {
	t.Helper()

	m.accMetaInfo[address] = accMetaInfo
}

func (m *MockStateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[addr]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockStateManager) GetLogicIDs(addr identifiers.Address, stateHash common.Hash) ([]identifiers.LogicID, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetAssetInfo(
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*common.AssetDescriptor, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetDeeds(
	addr identifiers.Address, stateHash common.Hash,
) (map[string]*common.AssetDescriptor, error) {
	// TODO implement me
	panic("implement me")
}

func createReceipt(t *testing.T, callBack func(r *common.Receipt)) *common.Receipt {
	t.Helper()

	receipt := &common.Receipt{
		IxHash: tests.RandomHash(t),
		IxOps: []*common.IxOpResult{
			{
				IxType: 2,
			},
		},
	}

	if callBack != nil {
		callBack(receipt)
	}

	return receipt
}

// createTSandLogs creates 3 dummy tesseracts with receipts.
// The receipts contain dummy logs.
// Dummy logs are created using address, hash, logic ID and data.
func createTSandLogs(
	t *testing.T,
	address identifiers.Address,
	hashes []common.Hash,
) ([]*common.Tesseract, common.Log) {
	t.Helper()

	logic := tests.GetLogicID(t, address)
	data := []byte{1}

	// create dummy logs
	logs := common.Log{
		Address: address,
		Topics:  hashes,
		LogicID: logic,
		Data:    data,
	}

	// create dummy receipts with logs
	receipts := createReceipt(t, func(r *common.Receipt) {
		r.IxOps[0].Logs = []common.Log{logs}
		r.IxHash = hashes[0]
	})

	ixns := tests.CreateIX(t, nil)

	paramsMap := map[int]*tests.CreateTesseractParams{
		0: {
			Addresses: []identifiers.Address{address},
			Heights:   []uint64{6},
			Receipts:  common.Receipts{tests.RandomHash(t): receipts},
			Ixns:      common.NewInteractionsWithLeaderCheck(false, ixns),
		},
		1: {
			Addresses: []identifiers.Address{address},
			Heights:   []uint64{10},
			Receipts:  common.Receipts{tests.RandomHash(t): receipts},
			Ixns:      common.NewInteractionsWithLeaderCheck(false, ixns),
		},
		2: {
			Addresses: []identifiers.Address{address},
			Heights:   []uint64{14},
			Receipts:  common.Receipts{tests.RandomHash(t): receipts},
			Ixns:      common.NewInteractionsWithLeaderCheck(false, ixns),
		},
	}
	tesseracts := tests.CreateTesseracts(t, 3, paramsMap)

	return tesseracts, logs
}

func validateLogs(t *testing.T, log common.Log, rpcLog *rpcargs.RPCLog) {
	t.Helper()

	require.Equal(t, log.Address, rpcLog.Address)
	require.Equal(t, log.LogicID, rpcLog.LogicID)
	require.Equal(t, log.Topics, rpcLog.Topics)
	require.Equal(t, hexutil.Bytes(log.Data), rpcLog.Data)
}

func createAndRunFilterManager(
	t *testing.T,
	eventMux *utils.TypeMux,
	backend *backend.Backend,
) *FilterManager {
	t.Helper()

	filterManager := NewFilterManager(hclog.NewNullLogger(), eventMux, &rpcargs.MockJSONRPCConfig, backend)

	go filterManager.Run()

	return filterManager
}

func postTesseractAddedEvents(t *testing.T, eventMux *utils.TypeMux, tesseracts []*common.Tesseract) {
	t.Helper()

	for i := 0; i < len(tesseracts); i++ {
		err := eventMux.Post(utils.TesseractAddedEvent{Tesseract: tesseracts[i]})
		require.NoError(t, err)
	}
}

func postPendingIxnsEvent(t *testing.T, eventMux *utils.TypeMux, interactions []*common.Interaction) {
	t.Helper()

	err := eventMux.Post(utils.AddedInteractionEvent{Ixs: interactions})
	require.NoError(t, err)
}

func checkWsConn(t *testing.T, filterManager *FilterManager, id string, expected bool) {
	t.Helper()

	filterBase := filterManager.filters[id].getFilterBase()
	// Check whether websocket connection exists or not
	require.Equal(t, expected, filterBase.hasWSConn())
}

func readWSMessage(
	t *testing.T,
	ctx context.Context,
	connManager *MockConnManager,
	resp chan tests.Result,
	expectedEvents int,
) {
	t.Helper()

	defer close(resp) // Ensure the channel is closed when the function exits

	receivedEvents := 0

	for {
		select {
		case <-ctx.Done():
			resp <- tests.Result{Data: nil, Err: common.ErrTimeOut}

			return
		case data := <-connManager.readMessage():
			resp <- tests.Result{Data: data, Err: nil}

			receivedEvents++

			if receivedEvents >= expectedEvents {
				return // Exit if we have received the expected number of events
			}
		}
	}
}

func processWSMessage(t *testing.T, respChan <-chan tests.Result) *Message {
	t.Helper()

	res := <-respChan
	require.NoError(t, res.Err)
	require.Equal(t, reflect.TypeOf(&MockMessage{}), reflect.TypeOf(res.Data))

	// Assert and extract information from MockMessage
	wsMessage, ok := res.Data.(*MockMessage)
	require.True(t, ok)
	require.Equal(t, gorillaWS.TextMessage, wsMessage.messageType)

	// Unmarshal the data field of MockMessage into a Message struct
	resp := new(Message)
	err := json.Unmarshal(wsMessage.data, &resp)
	require.NoError(t, err)

	return resp
}

func assertRPCTesseract(
	t *testing.T,
	expectedTesseract *common.Tesseract,
	res *Message,
) {
	t.Helper()

	var rpcTesseract rpcargs.RPCTesseract

	err := json.Unmarshal(res.Params.Result, &rpcTesseract)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	require.Equal(t, len(expectedTesseract.Participants()), len(rpcTesseract.Participants))

	for _, addr := range expectedTesseract.Addresses() {
		require.True(t, rpcTesseract.HasParticipant(addr))
		require.Equal(t, expectedTesseract.Height(addr), rpcTesseract.Height(addr))
	}
}

func assertRPCLogs(
	t *testing.T,
	expectedTesseract *common.Tesseract,
	logs common.Log,
	expectedHash common.Hash,
	res *Message,
) {
	t.Helper()

	var rpcLog rpcargs.RPCLog
	err := json.Unmarshal(res.Params.Result, &rpcLog)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	validateLogs(t, logs, &rpcLog)
	require.Equal(t, expectedHash, rpcLog.IxHash)

	require.Equal(t, expectedTesseract.Hash(), rpcLog.TSHash)
	rpcargs.CheckForRPCParticipants(t, expectedTesseract.Participants(), rpcLog.Participants)
}

func assertIxHashes(t *testing.T, expectedIx *common.Interaction, res *Message) {
	t.Helper()

	var ixHash string
	err := json.Unmarshal(res.Params.Result, &ixHash)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	require.Equal(t, expectedIx.Hash(), common.HexToHash(ixHash))
}

func subscribeToNewTesseractEvent(t *testing.T, dispatcher *dispatcher, mockConnManager *MockConnManager) {
	t.Helper()

	request := Request{
		ID:     1.0,
		Method: "moi.subscribe",
		Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"address": "%s"}]`, tests.RandomAddress(t))),
	}

	// Forward the message to dispatcher
	response := dispatcher.handleSingleWs(request, mockConnManager)

	successResponse, ok := response.(*SuccessResponse)
	require.True(t, ok)
	require.Nil(t, successResponse.Error)

	// Unmarshal the dispatcher result from the response
	var result string
	err := json.Unmarshal(successResponse.Result, &result)
	require.NoError(t, err)

	// Check whether the connection manager's subscription id and dispatcher result matches
	require.Equal(t, mockConnManager.GetFilterID(), result)
}

func setupTestHTTPServer(t *testing.T) (*gorillaWS.Conn, *http.Response) {
	t.Helper()

	port, err := tests.GetAvailablePort(t)
	require.NoError(t, err)

	addr := &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	}

	s := MockServer(t, addr)

	s.router.HandleFunc("/ws", s.handleWs)

	server := &http.Server{
		Addr:              s.addr.String(),
		Handler:           s.router,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		err := server.ListenAndServe()
		require.NoError(t, err)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	var (
		conn *gorillaWS.Conn
		resp *http.Response
	)

	_, err = tests.RetryUntilTimeout(ctx, 100*time.Millisecond, func() (interface{}, bool) {
		dialer := gorillaWS.Dialer{}

		conn, resp, err = dialer.Dial("ws://"+s.addr.String()+"/ws", nil)
		if err != nil {
			return nil, true
		}

		return nil, false
	})
	require.NoError(t, err)

	return conn, resp
}
