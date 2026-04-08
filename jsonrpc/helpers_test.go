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

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common/hexutil"

	gorillaWS "github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

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

func (m *MockChainManager) GetLatestTesseract(_ identifiers.Identifier, _ bool) (*common.Tesseract, error) {
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

func (m *MockChainManager) setTesseractHeightEntry(id identifiers.Identifier, height uint64, hash common.Hash) {
	key := id.Hex() + strconv.FormatUint(height, 10)
	m.TSHashByHeight[key] = hash
}

func (m *MockChainManager) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) (common.Hash, error) {
	key := id.Hex() + strconv.FormatUint(height, 10)

	if hash, ok := m.TSHashByHeight[key]; ok {
		return hash, nil
	}

	return common.NilHash, common.ErrKeyNotFound
}

type MockStateManager struct {
	storage     map[common.Hash][]byte
	accMetaInfo map[identifiers.Identifier]*common.AccountMetaInfo
}

func (m *MockStateManager) GetSubAccountCount(id identifiers.Identifier) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetMetaContextObject(id identifiers.Identifier,
	hash common.Hash,
) (*state.MetaContextObject, error) {
	// TODO implement me
	panic("implement me")
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)
	mockState.storage = make(map[common.Hash][]byte)
	mockState.accMetaInfo = make(map[identifiers.Identifier]*common.AccountMetaInfo)

	return mockState
}

func (m *MockStateManager) GetAccountKeys(id identifiers.Identifier,
	stateHash common.Hash,
) (common.AccountKeys, error) {
	panic("implement me")
}

func (m *MockStateManager) GetSequenceID(id identifiers.Identifier,
	keyID uint64, stateHash common.Hash,
) (uint64, error) {
	panic("implement me")
}

func (m *MockStateManager) GetMandates(
	id identifiers.Identifier, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetLockups(
	id identifiers.Identifier, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) FetchIxStateObjects(ixns common.Interactions,
	hashes map[identifiers.Identifier]common.Hash,
) (*state.Transition, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) CreateStateObject(id identifiers.Identifier,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetStateObjectByHash(id identifiers.Identifier, hash common.Hash) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetConsensusNodesByHash(id identifiers.Identifier,
	hash common.Hash,
) ([]identifiers.KramaID, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalances(id identifiers.Identifier, stateHash common.Hash) (common.AssetMap, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetBalance(
	id identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID,
	stateHash common.Hash,
) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetNonce(id identifiers.Identifier, stateHash common.Hash) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetAccountState(id identifiers.Identifier, stateHash common.Hash) (*common.Account, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetLogicManifest(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetPersistentStorageEntry(
	logicID identifiers.Identifier, slot []byte, state common.Hash,
) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetEphemeralStorageEntry(
	_ identifiers.Identifier, _ identifiers.Identifier, _ []byte, _ common.Hash,
) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	id identifiers.Identifier,
	accMetaInfo *common.AccountMetaInfo,
) {
	t.Helper()

	m.accMetaInfo[id] = accMetaInfo
}

func (m *MockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[id]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockStateManager) GetLogicIDs(
	id identifiers.Identifier,
	stateHash common.Hash,
) ([]identifiers.Identifier, error) {
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
	id identifiers.Identifier, stateHash common.Hash,
) (map[identifiers.Identifier]*common.AssetDescriptor, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetValidators() []*common.Validator {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) GetValidatorByKramaID(kramaID identifiers.KramaID) (*common.Validator, error) {
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
// Dummy logs are created using id, hash, logic ID and data.
func createTSandLogs(
	t *testing.T,
	id identifiers.Identifier,
	hashes []common.Hash,
) ([]*common.Tesseract, common.Log) {
	t.Helper()

	logic := tests.GetLogicID(t, id)
	data := []byte{1}

	// create dummy logs
	logs := common.Log{
		ID:      id,
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
			IDs:      []identifiers.Identifier{id},
			Heights:  []uint64{6},
			Receipts: common.Receipts{tests.RandomHash(t): receipts},
			Ixns:     common.NewInteractionsWithLeaderCheck(false, ixns),
		},
		1: {
			IDs:      []identifiers.Identifier{id},
			Heights:  []uint64{10},
			Receipts: common.Receipts{tests.RandomHash(t): receipts},
			Ixns:     common.NewInteractionsWithLeaderCheck(false, ixns),
		},
		2: {
			IDs:      []identifiers.Identifier{id},
			Heights:  []uint64{14},
			Receipts: common.Receipts{tests.RandomHash(t): receipts},
			Ixns:     common.NewInteractionsWithLeaderCheck(false, ixns),
		},
	}
	tesseracts := tests.CreateTesseracts(t, 3, paramsMap)

	return tesseracts, logs
}

func validateLogs(t *testing.T, log common.Log, rpcLog *rpcargs.RPCLog) {
	t.Helper()

	require.Equal(t, log.ID, rpcLog.ID)
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

	for _, id := range expectedTesseract.AccountIDs() {
		require.True(t, rpcTesseract.HasParticipant(id))
		require.Equal(t, expectedTesseract.Height(id), rpcTesseract.Height(id))
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
	rpcargs.CheckForRPCParticipantState(t, expectedTesseract.Participants(), rpcLog.Participants)
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
		Method: "moi.Subscribe",
		Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
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

	id := &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	}

	s := MockServer(t, id)

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
