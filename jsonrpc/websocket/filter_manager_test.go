package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/go-moi/jsonrpc/backend"

	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
)

type MessageParams struct {
	Subscription string          `json:"subscription"`
	Result       json.RawMessage `json:"result"`
}

type Message struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  interface{}    `json:"method"`
	Params  *MessageParams `json:"params"`
}

const (
	defaultTesseractRangeLimit = 10
	contextTimeout             = 5 * time.Second
)

var mockJSONRPCConfig = config.JSONRPCConfig{
	TesseractRangeLimit: defaultTesseractRangeLimit,
}

func TestTesseractSubscription(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)
	connManager := NewMockConnectionManager()

	// Create a new tesseracts subscription
	subscriptionID := filterManager.NewTesseractFilter(connManager)
	checkWsConn(t, filterManager, subscriptionID, true)

	tesseracts := tests.CreateTesseracts(t, 3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	respChan := make(chan tests.Result, 1)

	go readWSMessage(t, ctx, connManager, respChan, len(tesseracts))

	postTesseractAddedEvents(t, eventMux, tesseracts)

	for i := 0; i < 3; i++ {
		resp := processWSMessage(t, respChan)

		// match subscription field in subscriptionTemplate
		require.Equal(t, subscriptionID, resp.Params.Subscription)
		assertRPCTesseract(t, tesseracts[i], resp)
	}
}

func TestTesseractByAccountSubscription(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)
	connManager := NewMockConnectionManager()

	address := tests.GetAddresses(t, 2)
	paramsMap := map[int]*tests.CreateTesseractParams{
		0: {
			// will not be posted to websocket stream
			Address: address[1],
		},
		1: {
			Address: address[0],
		},
		2: {
			Address: address[0],
		},
	}
	tesseracts := tests.CreateTesseracts(t, 3, paramsMap)

	// Create a new tesseract by account subscription
	subscriptionID := filterManager.NewTesseractsByAccountFilter(connManager, address[0])
	checkWsConn(t, filterManager, subscriptionID, true)

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	respChan := make(chan tests.Result, 1)

	go readWSMessage(t, ctx, connManager, respChan, len(tesseracts))

	postTesseractAddedEvents(t, eventMux, tesseracts)

	// assert 2nd and 3rd ts
	for i := 1; i < 3; i++ {
		resp := processWSMessage(t, respChan)

		// match subscription field in subscriptionTemplate
		require.Equal(t, subscriptionID, resp.Params.Subscription)
		assertRPCTesseract(t, tesseracts[i], resp)
	}
}

func TestTesseractLogsSubscription(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)
	connManager := NewMockConnectionManager()

	addresses := tests.GetAddresses(t, 1)
	hashes := tests.GetHashes(t, 1)
	tesseracts, logs := createTSandLogs(t, addresses, hashes)

	filterQuery := &LogQuery{
		Address:     tesseracts[0].Address(),
		StartHeight: 5,
		EndHeight:   15,
		Topics: [][]common.Hash{
			{
				hashes[0],
			},
		},
	}

	// Create a new tesseract log subscription
	subscriptionID := filterManager.NewLogFilter(connManager, filterQuery)
	checkWsConn(t, filterManager, subscriptionID, true)

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	respChan := make(chan tests.Result, 1)

	go readWSMessage(t, ctx, connManager, respChan, len(tesseracts))

	postTesseractAddedEvents(t, eventMux, tesseracts)

	for i := 0; i < 3; i++ {
		resp := processWSMessage(t, respChan)

		// match subscription field in subscriptionTemplate
		require.Equal(t, subscriptionID, resp.Params.Subscription)
		assertRPCLogs(t, tesseracts[i], logs, hashes[0], resp)
	}
}

func TestPendingIxnsSubscription(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)
	connManager := NewMockConnectionManager()

	// Create a new pending ixns subscription
	subscriptionID := filterManager.PendingIxnsFilter(connManager)
	checkWsConn(t, filterManager, subscriptionID, true)

	interactions := tests.CreateIxns(t, 3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	respChan := make(chan tests.Result, 1)

	go readWSMessage(t, ctx, connManager, respChan, len(interactions))

	postPendingIxnsEvent(t, eventMux, interactions)

	for i := 0; i < 3; i++ {
		resp := processWSMessage(t, respChan)

		// match subscription field in subscriptionTemplate
		require.Equal(t, subscriptionID, resp.Params.Subscription)
		assertIxHashes(t, interactions[i], resp)
	}
}

func TestAllSubscriptions(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)
	connManager := NewMockConnectionManager()

	addresses := tests.GetAddresses(t, 1)
	hashes := tests.GetHashes(t, 1)
	tesseracts, logs := createTSandLogs(t, addresses, hashes)

	filterQuery := &LogQuery{
		Address:     tesseracts[0].Address(),
		StartHeight: 5,
		EndHeight:   15,
		Topics: [][]common.Hash{
			{
				hashes[0],
			},
		},
	}

	interactions := tests.CreateIxns(t, 3, nil)

	// Create all subscriptions
	tsSubID := filterManager.NewTesseractFilter(connManager)
	checkWsConn(t, filterManager, tsSubID, true)

	tsByAccountSubID := filterManager.NewTesseractsByAccountFilter(connManager, tesseracts[0].Address())
	checkWsConn(t, filterManager, tsByAccountSubID, true)

	logSubID := filterManager.NewLogFilter(connManager, filterQuery)
	checkWsConn(t, filterManager, logSubID, true)

	pendingIxnsSubID := filterManager.PendingIxnsFilter(connManager)
	checkWsConn(t, filterManager, pendingIxnsSubID, true)

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	respChan := make(chan tests.Result, len(tesseracts))

	// expected events = (3 (types of ts filters) * no of ts) + no of ixns
	go readWSMessage(t, ctx, connManager, respChan, 3*len(tesseracts)+len(interactions))

	postTesseractAddedEvents(t, eventMux, tesseracts)
	postPendingIxnsEvent(t, eventMux, interactions)

	tsSubCount := 0
	tsByAccSubCount := 0
	logSubCount := 0
	ixnsSubCount := 0

	for i := 0; i < 12; i++ {
		resp := processWSMessage(t, respChan)

		switch resp.Params.Subscription {
		case tsSubID:
			assertRPCTesseract(t, tesseracts[tsSubCount], resp)
			tsSubCount++
		case tsByAccountSubID:
			assertRPCTesseract(t, tesseracts[tsByAccSubCount], resp)
			tsByAccSubCount++
		case logSubID:
			assertRPCLogs(t, tesseracts[logSubCount], logs, hashes[0], resp)
			logSubCount++
		case pendingIxnsSubID:
			assertIxHashes(t, interactions[ixnsSubCount], resp)
			ixnsSubCount++
		default:
			require.FailNow(t, "unknown subscription type")
		}
	}
}

func TestFilterTimeout(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := NewFilterManager(hclog.NewNullLogger(), eventMux, &mockJSONRPCConfig, nil)

	defer filterManager.Close()

	filterManager.timeout = 200 * time.Millisecond

	go filterManager.Run()

	tesseract := tests.CreateTesseract(t, nil)

	filterID := filterManager.NewTesseractsByAccountFilter(nil, tesseract.Address())

	// Check if the filter manager has the filter
	require.True(t, filterManager.exists(filterID))
	// Wait for timeout
	time.Sleep(600 * time.Millisecond)
	// Check if the filter manager has removed the filter or not
	require.False(t, filterManager.exists(filterID))
}

func TestGetNumericTesseractNumber(t *testing.T) {
	stateManager := NewMockStateManager(t)
	newBackend := backend.NewBackend(nil, nil, nil, stateManager, nil, nil, nil)
	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, newBackend)

	acc := tests.GetRandomAccMetaInfo(t, 5)
	stateManager.setAccountMetaInfo(t, acc.Address, acc)

	testcases := []struct {
		name           string
		height         int64
		address        common.Address
		expectedHeight uint64
		expectedError  error
	}{
		{
			name:           "Latest Tesseract Height",
			height:         -1,
			address:        acc.Address,
			expectedHeight: 5,
			expectedError:  nil,
		},
		{
			name:           "Valid height",
			height:         3,
			address:        tests.RandomAddress(t),
			expectedHeight: 3,
			expectedError:  nil,
		},
		{
			name:           "Invalid height",
			height:         -5,
			address:        tests.RandomAddress(t),
			expectedHeight: 0,
			expectedError:  common.ErrInvalidHeight,
		},
		{
			name:           "Account Not Found",
			height:         -1,
			address:        tests.RandomAddress(t),
			expectedHeight: 0,
			expectedError:  common.ErrAccountNotFound,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			height, err := filterManager.getNumericTesseractNumber(testcase.height, testcase.address)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedHeight, height)
		})
	}
}

func TestGetLogsFromTesseract(t *testing.T) {
	chainManager := NewMockChainManager(t)
	newBackend := backend.NewBackend(nil, chainManager, nil, nil, nil, nil, nil)
	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, newBackend)

	addresses := tests.GetAddresses(t, 2)
	logic := tests.GetLogicID(t, addresses[0])
	hashes := tests.GetHashes(t, 2)

	// create dummy logs
	log := &common.Log{
		Addresses: addresses,
		LogicID:   logic,
		Topics:    hashes,
		Data:      []byte{1},
	}

	// create dummy receipts with logs
	receipts := createReceipt(t, func(r *common.Receipt) {
		r.Logs = []*common.Log{
			log,
			{
				Addresses: addresses,
				LogicID:   logic,
				Topics:    tests.GetHashes(t, 2),
				Data:      []byte{1},
			},
		}
	})

	// create tesseract with logs in receipts
	params := &tests.CreateTesseractParams{
		Address: addresses[0],
		Height:  5,
		HeaderCallback: func(header *common.TesseractHeader) {
			header.Extra = common.CommitData{
				GridID: &common.TesseractGridID{
					Hash: hashes[0],
				},
			}
		},
		BodyCallback: func(body *common.TesseractBody) {
			body.InteractionHash = tests.RandomHash(t)
		},
		Receipts: common.Receipts{tests.RandomHash(t): receipts},
	}

	tesseract := tests.CreateTesseract(t, params)

	// create dummy tesseract parts to validate address and height in RPCLogs
	tsParts := &common.TesseractParts{
		Total: 1,
		Grid: map[common.Address]common.TesseractHeightAndHash{
			tests.RandomAddress(t): {
				Height: 33,
				Hash:   tests.RandomHash(t),
			},
		},
	}

	chainManager.setTesseractPartsByGridHash(hashes[0], tsParts)

	testcases := []struct {
		name              string
		filter            *LogQuery
		expectedTesseract *common.Tesseract
		expectedLogs      []*common.Log
		expectedTSParts   *common.TesseractParts
		expectedError     error
	}{
		{
			name: "fetch logs from tesseract successfully",
			filter: &LogQuery{
				Address:     addresses[0],
				StartHeight: 5,
				EndHeight:   5,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedTesseract: tesseract,
			expectedLogs:      []*common.Log{log},
			expectedTSParts:   tsParts,
		},
		{
			name: "failed to get logs as tesseract parts not found",
			filter: &LogQuery{
				Address: addresses[0],
			},
			expectedTesseract: tests.CreateTesseract(t, &tests.CreateTesseractParams{
				HeaderCallback: func(header *common.TesseractHeader) {
					header.Extra = common.CommitData{
						GridID: &common.TesseractGridID{
							Hash: tests.RandomHash(t),
						},
					}
				},
			}),
			expectedError: common.ErrTesseractPartsNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcLogs, err := filterManager.getLogsFromTesseract(test.filter, test.expectedTesseract)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for i, log := range test.expectedLogs {
				rpcLog := rpcLogs[i]
				validateLogs(t, log, rpcLog)
				require.Equal(t, test.expectedTesseract.InteractionHash(), rpcLog.IxHash)
				validateTSGrid(t, test.expectedTSParts.Grid, rpcLog.Grid)
			}
		})
	}
}

func TestGetLogsForQuery(t *testing.T) {
	chainManager := NewMockChainManager(t)
	newBackend := backend.NewBackend(nil, chainManager, nil, nil, nil, nil, nil)
	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, newBackend)

	addresses := tests.GetAddresses(t, 2)
	logic := tests.GetLogicID(t, addresses[0])
	hashes := tests.GetHashes(t, 2)

	log := &common.Log{
		Addresses: addresses,
		LogicID:   logic,
		Topics:    hashes,
		Data:      []byte{1},
	}

	receipts := createReceipt(t, func(r *common.Receipt) {
		r.Logs = []*common.Log{
			log,
			{
				Addresses: addresses,
				LogicID:   logic,
				Topics:    tests.GetHashes(t, 2),
				Data:      []byte{1},
			},
		}
	})

	// ts 0 and 1 are used to fetch logs successfully
	// ts 2 doesn't have interactions, so no logs from tesseract is fetched
	// ts 3 is used to simulate invalid grid hash
	paramsMap := map[int]*tests.CreateTesseractParams{
		0: {
			Address: addresses[0],
			Height:  0,
			HeaderCallback: func(header *common.TesseractHeader) {
				header.Extra = common.CommitData{
					GridID: &common.TesseractGridID{
						Hash: hashes[0],
					},
				}
			},
			Receipts: common.Receipts{tests.RandomHash(t): receipts},
			Ixns:     tests.CreateIxns(t, 1, nil),
		},
		1: {
			Address: addresses[0],
			Height:  1,
			HeaderCallback: func(header *common.TesseractHeader) {
				header.Extra = common.CommitData{
					GridID: &common.TesseractGridID{
						Hash: hashes[0],
					},
				}
			},
			Receipts: common.Receipts{tests.RandomHash(t): receipts},
			Ixns:     tests.CreateIxns(t, 1, nil),
		},
		2: {
			Address: addresses[0],
			Height:  2,
			HeaderCallback: func(header *common.TesseractHeader) {
				header.Extra = common.CommitData{
					GridID: &common.TesseractGridID{
						Hash: hashes[0],
					},
				}
			},
		},
		3: {
			Address: addresses[0],
			Height:  3,
			HeaderCallback: func(header *common.TesseractHeader) {
				header.Extra = common.CommitData{
					GridID: &common.TesseractGridID{
						Hash: tests.RandomHash(t),
					},
				}
			},
			Ixns: tests.CreateIxns(t, 1, nil),
		},
	}

	tesseracts := tests.CreateTesseracts(t, 4, paramsMap)

	tsGrid := map[common.Address]common.TesseractHeightAndHash{
		tests.RandomAddress(t): {
			Height: 33,
			Hash:   tests.RandomHash(t),
		},
	}

	tsParts := &common.TesseractParts{
		Total: 1,
		Grid:  tsGrid,
	}

	chainManager.setTesseractPartsByGridHash(hashes[0], tsParts)

	for i := 0; i < 4; i++ {
		chainManager.setTesseractHeightEntry(tesseracts[i].Address(), tesseracts[i].Height(), tesseracts[i].Hash())
		chainManager.setTesseractByHash(t, tesseracts[i])
	}

	testcases := []struct {
		name               string
		filter             LogQuery
		expectedTesseracts []*common.Tesseract
		expectedLogs       []*common.Log
		expectedTSGrid     map[common.Address]common.TesseractHeightAndHash
		expectedError      error
	}{
		{
			name: "fetch logs for the query successfully",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 0,
				EndHeight:   1,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedTesseracts: tesseracts,
			expectedLogs:       []*common.Log{log, log},
			expectedTSGrid:     tsGrid,
		},
		{
			name: "invalid start height",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: -5,
				EndHeight:   5,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedError: common.ErrInvalidHeight,
		},
		{
			name: "invalid end height",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 0,
				EndHeight:   -5,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedError: common.ErrInvalidHeight,
		},
		{
			name: "StartHeight is greater than EndHeight",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 2,
				EndHeight:   1,
			},
			expectedError: ErrInvalidHeightQuery,
		},
		{
			name: "Difference between StartHeight and EndHeight is greater than 10",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 0,
				EndHeight:   100,
			},
			expectedError: ErrInvalidQueryRange,
		},
		{
			name: "failed to fetch logs as tesseract height not found",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 5,
				EndHeight:   8,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedTesseracts: tesseracts,
			expectedTSGrid:     tsGrid,
			expectedError:      nil,
		},
		{
			name: "failed to fetch logs as tesseract doesnt have interaction",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 2,
				EndHeight:   2,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedTesseracts: []*common.Tesseract{tests.CreateTesseract(t, nil)},
			expectedTSGrid:     tsGrid,
			expectedError:      nil,
		},
		{
			name: "failed to fetch logs from tesseract",
			filter: LogQuery{
				Address:     addresses[0],
				StartHeight: 3,
				EndHeight:   3,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			expectedError: common.ErrTesseractPartsNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcLogs, err := filterManager.GetLogsForQuery(test.filter)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for i, log := range test.expectedLogs {
				rpcLog := rpcLogs[i]
				validateLogs(t, log, rpcLog)
				require.Equal(t, test.expectedTesseracts[i].InteractionHash(), rpcLog.IxHash)
				validateTSGrid(t, test.expectedTSGrid, rpcLog.Grid)
			}
		})
	}
}

func TestGetFilterChangesAndUninstall(t *testing.T) {
	t.Parallel()

	eventMux := new(utils.TypeMux)
	filterManager := createAndRunFilterManager(t, eventMux, nil)

	// Create a new tesseracts filter
	filterID := filterManager.NewTesseractFilter(nil)
	checkWsConn(t, filterManager, filterID, false)

	tesseracts := tests.CreateTesseracts(t, 3, nil)

	// post only 2 tesseracts
	go postTesseractAddedEvents(t, eventMux, tesseracts[:2])

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	count := 2 // Posting 2 tesseracts first
	res := make([]*rpcargs.RPCTesseract, 0)
	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		filterChanges, err := filterManager.GetFilterChanges(filterID)
		require.NoError(t, err)

		rpcTesseracts, ok := filterChanges.([]*rpcargs.RPCTesseract)
		require.True(t, ok)

		count -= len(rpcTesseracts)
		res = append(res, rpcTesseracts...)

		if count == 0 {
			return res, false
		}

		return nil, true
	})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		require.Equal(t, tesseracts[i].Address(), res[i].Address())
		require.Equal(t, tesseracts[i].Height(), res[i].Height())
	}

	ok := filterManager.Uninstall(filterID)
	require.True(t, ok)

	// post 3rd tesseract
	go postTesseractAddedEvents(t, eventMux, tesseracts[2:])

	_, err = filterManager.GetFilterChanges(filterID)
	require.Error(t, err, ErrFilterNotFound)
}
