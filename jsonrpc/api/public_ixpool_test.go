package api

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

type account struct {
	pending  common.Interactions
	queued   common.Interactions
	waitTime int64
}

type expectedResult struct {
	pending  int
	queued   int
	waitTime int64
	expired  bool
}

//nolint:dupl
func TestPublicIXPoolAPI_Content(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Address]*account
		testFn          func(accounts map[identifiers.Address]*account)
		expectedIxQueue map[identifiers.Address]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued:  common.Interactions{},
				},
			},
			expectedIxQueue: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending: 0,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending: 1,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending: 0,
					queued:  1,
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending: 0,
					queued:  2,
				},
				addressList[1]: {
					pending: 2,
					queued:  1,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.accounts)
			}

			response, err := ixPoolAPI.Content()
			require.NoError(t, err)

			for address, ixQueue := range testcase.expectedIxQueue {
				require.Equal(t, ixQueue.pending, len(response.Pending[address]))
				require.Equal(t, ixQueue.queued, len(response.Queued[address]))
			}
		})
	}
}

//nolint:dupl
func TestPublicIXPoolAPI_ContentFrom(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		args            *rpcargs.IxPoolArgs
		accounts        map[identifiers.Address]*account
		testFn          func(accounts map[identifiers.Address]*account)
		expectedIxQueue *expectedResult
		expectedErr     error
	}{
		{
			name: "nil address",
			args: &rpcargs.IxPoolArgs{
				Address: identifiers.NilAddress,
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Ix pool with no interactions",
			args: &rpcargs.IxPoolArgs{
				Address: addressList[0],
			},
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued:  common.Interactions{},
				},
			},
			expectedIxQueue: &expectedResult{
				pending: 0,
				queued:  0,
			},
			expectedErr: nil,
		},
		{
			name: "Ix pool with one pending interaction",
			args: &rpcargs.IxPoolArgs{
				Address: addressList[0],
			},
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: &expectedResult{
				pending: 1,
				queued:  0,
			},
			expectedErr: nil,
		},
		{
			name: "Ix pool with one queued interaction",
			args: &rpcargs.IxPoolArgs{
				Address: addressList[0],
			},
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: &expectedResult{
				pending: 0,
				queued:  1,
			},
			expectedErr: nil,
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			args: &rpcargs.IxPoolArgs{
				Address: addressList[1],
			},
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: &expectedResult{
				pending: 2,
				queued:  1,
			},
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.accounts)
			}

			response, err := ixPoolAPI.ContentFrom(testcase.args)

			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedIxQueue.pending, len(response.Pending))
			require.Equal(t, testcase.expectedIxQueue.queued, len(response.Queued))
		})
	}
}

//nolint:dupl
func TestPublicIXPoolAPI_Status(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Address]*account
		testFn          func(accounts map[identifiers.Address]*account)
		expectedIxQueue *expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued:  common.Interactions{},
				},
			},
			expectedIxQueue: &expectedResult{
				pending: 0,
				queued:  0,
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: &expectedResult{
				pending: 2,
				queued:  3,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.accounts)
			}

			response, err := ixPoolAPI.Status()
			require.NoError(t, err)

			require.Equal(t, uint64(testcase.expectedIxQueue.pending), response.Pending.ToUint64())
			require.Equal(t, uint64(testcase.expectedIxQueue.queued), response.Queued.ToUint64())
		})
	}
}

func TestPublicIXPoolAPI_Inspect(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Address]*account
		testFn          func(accounts map[identifiers.Address]*account)
		expectedAccInfo map[identifiers.Address]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending:  common.Interactions{},
					queued:   common.Interactions{},
					waitTime: 0,
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   0,
					waitTime: 0,
					expired:  true,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{
						newTestInteraction(t, 0, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Receiver = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
							ixData.Input.Type = common.IxValueTransfer
						}),
					},
					queued:   common.Interactions{},
					waitTime: int64(1500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending:  1,
					queued:   0,
					waitTime: int64(1500 * time.Millisecond),
					expired:  false,
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Receiver = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(2500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   1,
					waitTime: int64(2500 * time.Millisecond),
					expired:  false,
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Address]*account{
				addressList[0]: {
					pending: common.Interactions{},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(500 * time.Millisecond),
				},
				addressList[1]: {
					pending: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: common.Interactions{
						newTestInteraction(t, 1, func(ixData *common.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(1000 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   2,
					waitTime: int64(500 * time.Millisecond),
					expired:  false,
				},
				addressList[1]: {
					pending:  2,
					queued:   1,
					waitTime: int64(1000 * time.Millisecond),
					expired:  false,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.accounts)
			}

			response, err := ixPoolAPI.Inspect()
			require.NoError(t, err)

			for address, accInfo := range testcase.expectedAccInfo {
				require.Equal(t, accInfo.pending, len(response.Pending[address.Hex()]))
				require.Equal(t, accInfo.queued, len(response.Queued[address.Hex()]))
				require.Equal(t, big.NewInt(accInfo.waitTime), response.WaitTime[address.Hex()].Time.ToInt())
				require.Equal(t, accInfo.expired, response.WaitTime[address.Hex()].Expired)

				if len(testcase.accounts[address].pending) > 0 {
					interactionInfo := response.Pending[address.Hex()]
					require.NotNil(t, interactionInfo)

					for _, ix := range testcase.accounts[address].pending {
						require.NotNil(t, interactionInfo[strconv.Itoa(int(ix.Nonce()))])
					}
				}

				if len(testcase.accounts[address].queued) > 0 {
					interactionInfo := response.Queued[address.Hex()]
					require.NotNil(t, interactionInfo)

					for _, ix := range testcase.accounts[address].queued {
						require.NotNil(t, interactionInfo[strconv.Itoa(int(ix.Nonce()))])
					}
				}
			}
		})
	}
}

func TestPublicIXPoolAPI_WaitTime(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	address := tests.RandomAddress(t)
	waitTime := int64(3500 * time.Millisecond)

	ixpool.setWaitTime(address, waitTime)

	testcases := []struct {
		name             string
		args             *rpcargs.IxPoolArgs
		expectedWaitTime int64
		expectedErr      error
	}{
		{
			name: "nil address",
			args: &rpcargs.IxPoolArgs{
				Address: identifiers.NilAddress,
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: &rpcargs.IxPoolArgs{
				Address: tests.RandomAddress(t),
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "Account with state",
			args: &rpcargs.IxPoolArgs{
				Address: address,
			},
			expectedWaitTime: waitTime,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			waitTime, err := ixPoolAPI.WaitTime(testcase.args)

			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, big.NewInt(testcase.expectedWaitTime), waitTime.Time.ToInt())
			require.Equal(t, false, waitTime.Expired)
		})
	}
}

func TestCreateWaitTime(t *testing.T) {
	testcases := []struct {
		name         string
		waitTime     *big.Int
		expired      bool
		expectedTime *big.Int
	}{
		{
			name:         "after time out",
			waitTime:     big.NewInt(-105),
			expired:      true,
			expectedTime: big.NewInt(105),
		},
		{
			name:         "before time out",
			waitTime:     big.NewInt(105),
			expired:      false,
			expectedTime: big.NewInt(105),
		},
		{
			name:         "at time",
			waitTime:     big.NewInt(0),
			expired:      true,
			expectedTime: big.NewInt(0),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			waitTime := createWaitTime(test.waitTime)
			require.Equal(t, test.expired, waitTime.Expired)
			require.Equal(t, test.expectedTime, waitTime.Time.ToInt())
		})
	}
}
