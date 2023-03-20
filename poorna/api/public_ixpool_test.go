package api

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

type account struct {
	pending  types.Interactions
	queued   types.Interactions
	waitTime int64
}

type expectedResult struct {
	pending  int
	queued   int
	waitTime int64
}

//nolint:dupl
func TestPublicIXPoolAPI_Content(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[types.Address]*account
		testFn          func(accounts map[types.Address]*account)
		expectedIxQueue map[types.Address]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued:  types.Interactions{},
				},
			},
			expectedIxQueue: map[types.Address]*expectedResult{
				addressList[0]: {
					pending: 0,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[types.Address]*expectedResult{
				addressList[0]: {
					pending: 1,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[types.Address]*expectedResult{
				addressList[0]: {
					pending: 0,
					queued:  1,
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[types.Address]*expectedResult{
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
		args            *ptypes.IxPoolArgs
		accounts        map[types.Address]*account
		testFn          func(accounts map[types.Address]*account)
		expectedIxQueue *expectedResult
		expectedErr     error
	}{
		{
			name: "Invalid address",
			args: &ptypes.IxPoolArgs{
				From: "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Ix pool with no interactions",
			args: &ptypes.IxPoolArgs{
				From: addressList[0].Hex(),
			},
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued:  types.Interactions{},
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
			args: &ptypes.IxPoolArgs{
				From: addressList[0].Hex(),
			},
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
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
			args: &ptypes.IxPoolArgs{
				From: addressList[0].Hex(),
			},
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
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
			args: &ptypes.IxPoolArgs{
				From: addressList[1].Hex(),
			},
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
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
		accounts        map[types.Address]*account
		testFn          func(accounts map[types.Address]*account)
		expectedIxQueue *expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued:  types.Interactions{},
				},
			},
			expectedIxQueue: &expectedResult{
				pending: 0,
				queued:  0,
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
				addressList[1]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[types.Address]*account) {
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

			require.Equal(t, uint64(testcase.expectedIxQueue.pending), response.Pending)
			require.Equal(t, uint64(testcase.expectedIxQueue.queued), response.Queued)
		})
	}
}

func TestPublicIXPoolAPI_Inspect(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	addressList := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[types.Address]*account
		testFn          func(accounts map[types.Address]*account)
		expectedAccInfo map[types.Address]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending:  types.Interactions{},
					queued:   types.Interactions{},
					waitTime: 0,
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[types.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   0,
					waitTime: 0,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{
						newTestInteraction(t, 0, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Receiver = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued:   types.Interactions{},
					waitTime: int64(1500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[types.Address]*expectedResult{
				addressList[0]: {
					pending:  1,
					queued:   0,
					waitTime: int64(1500 * time.Millisecond),
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Receiver = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(2500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[types.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   1,
					waitTime: int64(2500 * time.Millisecond),
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[types.Address]*account{
				addressList[0]: {
					pending: types.Interactions{},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[0]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(500 * time.Millisecond),
				},
				addressList[1]: {
					pending: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 1
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 2
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					queued: types.Interactions{
						newTestInteraction(t, 1, func(ixData *types.IxData) {
							ixData.Input.Sender = addressList[1]
							ixData.Input.Nonce = 3
							ixData.Input.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(1000 * time.Millisecond),
				},
			},
			testFn: func(accounts map[types.Address]*account) {
				for addr, account := range accounts {
					ixpool.setIxs(addr, account.pending, account.queued)
					ixpool.setWaitTime(addr, account.waitTime)
				}
			},
			expectedAccInfo: map[types.Address]*expectedResult{
				addressList[0]: {
					pending:  0,
					queued:   2,
					waitTime: int64(500 * time.Millisecond),
				},
				addressList[1]: {
					pending:  2,
					queued:   1,
					waitTime: int64(1000 * time.Millisecond),
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
				require.Equal(t, accInfo.waitTime, response.WaitTime[address.Hex()])

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
		args             *ptypes.IxPoolArgs
		expectedWaitTime int64
		expectedErr      error
	}{
		{
			name: "Invalid address",
			args: &ptypes.IxPoolArgs{
				From: "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: &ptypes.IxPoolArgs{
				From: tests.RandomAddress(t).Hex(),
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Account with state",
			args: &ptypes.IxPoolArgs{
				From: address.Hex(),
			},
			expectedWaitTime: waitTime,
			expectedErr:      nil,
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
			require.Equal(t, testcase.expectedWaitTime, waitTime)
		})
	}
}
