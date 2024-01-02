package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/state"
)

func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()

	// create a new filter manager
	filterMan := NewFilterManager(logger, eventMux, &mockJSONRPCConfig, nil)

	// create a new websocket handler
	wsHandler := NewHandler(hclog.NewNullLogger(), filterMan)
	mux.HandleFunc("/ws", wsHandler.HandleWsRequests)

	return httptest.NewServer(mux)
}

func initWSConnection(t *testing.T, address string) (*websocket.Conn, *http.Response) {
	t.Helper()

	dialer := websocket.Dialer{}
	// Establish a websocket connection
	conn, resp, err := dialer.Dial("ws://"+address+"/ws", nil)
	require.NoError(t, err)

	return conn, resp
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
	tesseractParts   map[common.Hash]*common.TesseractParts
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.tesseractsByHash = make(map[common.Hash]*common.Tesseract)
	mockChain.TSHashByHeight = make(map[string]common.Hash)
	mockChain.tesseractParts = make(map[common.Hash]*common.TesseractParts)

	return mockChain
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

func (m *MockChainManager) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := m.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (m *MockChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) GetInteractionAndPartsByIxHash(
	ixHash common.Hash,
) (*common.Interaction, *common.TesseractParts, int, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockChainManager) GetInteractionAndPartsByTSHash(tsHash common.Hash,
	ixIndex int,
) (*common.Interaction, *common.TesseractParts, error) {
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

func (m *MockChainManager) setTesseractPartsByGridHash(gridHash common.Hash, parts *common.TesseractParts) {
	m.tesseractParts[gridHash] = parts
}

func (m *MockChainManager) GetTesseractPartsByGridHash(gridHash common.Hash) (*common.TesseractParts, error) {
	parts, ok := m.tesseractParts[gridHash]
	if !ok {
		return nil, common.ErrTesseractPartsNotFound
	}

	return parts, nil
}

type MockStateManager struct {
	storage     map[common.Hash][]byte
	accMetaInfo map[identifiers.Address]*common.AccountMetaInfo
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

func (m *MockStateManager) GetContextByHash(address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalances(addrs identifiers.Address, stateHash common.Hash) (*state.BalanceObject, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalance(addr identifiers.Address,
	assetID identifiers.AssetID, stateHash common.Hash,
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

func (m *MockStateManager) GetStorageEntry(_ identifiers.LogicID, _ []byte, _ common.Hash) ([]byte, error) {
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

func (m *MockStateManager) GetRegistry(addr identifiers.Address, stateHash common.Hash) (map[string][]byte, error) {
	// TODO implement me
	panic("implement me")
}

func createReceipt(t *testing.T, callBack func(r *common.Receipt)) *common.Receipt {
	t.Helper()

	receipt := &common.Receipt{
		IxType: 2,
		IxHash: tests.RandomHash(t),
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
	addresses []identifiers.Address,
	hashes []common.Hash,
) ([]*common.Tesseract, *common.Log) {
	t.Helper()

	logic := tests.GetLogicID(t, addresses[0])
	data := []byte{1}

	// create dummy logs
	logs := &common.Log{
		Addresses: addresses,
		Topics:    hashes,
		LogicID:   logic,
		Data:      data,
	}

	// create dummy receipts with logs
	receipts := createReceipt(t, func(r *common.Receipt) {
		r.Logs = []*common.Log{logs}
		r.IxHash = hashes[0]
	})

	headerCallbackWithGridHash := func(
		t *testing.T,
		address identifiers.Address,
		hash common.Hash,
		height uint64,
	) func(header *common.TesseractHeader) {
		t.Helper()

		return func(header *common.TesseractHeader) {
			header.Extra = common.CommitData{
				GridID: &common.TesseractGridID{
					Hash: tests.RandomHash(t),
					Parts: &common.TesseractParts{
						Total: 2,
						Grid: map[identifiers.Address]common.TesseractHeightAndHash{
							address: {
								Height: height,
								Hash:   hash,
							},
						},
					},
				},
			}
		}
	}

	ixns := tests.CreateIX(t, nil)

	paramsMap := map[int]*tests.CreateTesseractParams{
		0: {
			Address:        addresses[0],
			Height:         6,
			HeaderCallback: headerCallbackWithGridHash(t, addresses[0], hashes[0], 6),
			Receipts:       common.Receipts{tests.RandomHash(t): receipts},
			Ixns:           common.Interactions{ixns},
		},
		1: {
			Address:        addresses[0],
			Height:         10,
			HeaderCallback: headerCallbackWithGridHash(t, addresses[0], hashes[0], 10),
			Receipts:       common.Receipts{tests.RandomHash(t): receipts},
			Ixns:           common.Interactions{ixns},
		},
		2: {
			Address:        addresses[0],
			Height:         14,
			HeaderCallback: headerCallbackWithGridHash(t, addresses[0], hashes[0], 14),
			Receipts:       common.Receipts{tests.RandomHash(t): receipts},
			Ixns:           common.Interactions{ixns},
		},
	}
	tesseracts := tests.CreateTesseracts(t, 3, paramsMap)

	return tesseracts, logs
}

func validateLogs(t *testing.T, log *common.Log, rpcLog *args.RPCLog) {
	t.Helper()

	require.Equal(t, log.Addresses, rpcLog.Addresses)
	require.Equal(t, log.LogicID, rpcLog.LogicID)
	require.Equal(t, log.Topics, rpcLog.Topics)
	require.Equal(t, log.Data, rpcLog.Data)
}

func validateTSGrid(
	t *testing.T,
	grid map[identifiers.Address]common.TesseractHeightAndHash,
	rpcGrid args.RPCTesseractParts,
) {
	t.Helper()

	for address, heightAndHash := range grid {
		found := false

		for _, rpcPart := range rpcGrid {
			if rpcPart.Address == address && rpcPart.Hash == heightAndHash.Hash &&
				rpcPart.Height.ToUint64() == heightAndHash.Height {
				found = true
			}
		}

		require.True(t, found)
	}
}

func createAndRunFilterManager(
	t *testing.T,
	eventMux *utils.TypeMux,
	backend *backend.Backend,
) *FilterManager {
	t.Helper()

	filterManager := NewFilterManager(hclog.NewNullLogger(), eventMux, &mockJSONRPCConfig, backend)

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

func postPendingIxnsEvent(t *testing.T, eventMux *utils.TypeMux, interactions common.Interactions) {
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
	require.Equal(t, websocket.TextMessage, wsMessage.messageType)

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

	var rpcTesseract args.RPCTesseract

	err := json.Unmarshal(res.Params.Result, &rpcTesseract)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	require.Equal(t, expectedTesseract.Address(), rpcTesseract.Address())
	require.Equal(t, expectedTesseract.Height(), rpcTesseract.Height())
}

func assertRPCLogs(
	t *testing.T,
	expectedTesseract *common.Tesseract,
	logs *common.Log,
	expectedHash common.Hash,
	res *Message,
) {
	t.Helper()

	var rpcLog args.RPCLog
	err := json.Unmarshal(res.Params.Result, &rpcLog)
	require.NoError(t, err)

	gridBytes, err := json.Marshal(rpcLog.Grid)
	require.NoError(t, err)

	var grid args.RPCTesseractParts
	err = json.Unmarshal(gridBytes, &grid)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	validateLogs(t, logs, &rpcLog)
	require.Equal(t, expectedHash, rpcLog.IxHash)
	require.Equal(t, expectedTesseract.Address(), grid[0].Address)
	require.Equal(t, expectedHash, grid[0].Hash)
	require.Equal(t, expectedTesseract.Height(), grid[0].Height.ToUint64())
}

func assertIxHashes(t *testing.T, expectedIx *common.Interaction, res *Message) {
	t.Helper()

	var ixHash string
	err := json.Unmarshal(res.Params.Result, &ixHash)
	require.NoError(t, err)

	// match result field in subscriptionTemplate
	require.Equal(t, expectedIx.Hash(), common.HexToHash(ixHash))
}
