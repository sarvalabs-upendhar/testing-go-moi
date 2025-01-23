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
	pending  []*common.Interaction
	queued   []*common.Interaction
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
	ids := tests.GetRandomIDs(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Identifier]*account
		testFn          func(accounts map[identifiers.Identifier]*account)
		expectedIxQueue map[identifiers.Identifier]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued:  []*common.Interaction{},
				},
			},
			expectedIxQueue: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending: 0,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending: 1,
					queued:  0,
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending: 0,
					queued:  1,
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
				ids[1]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 3
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
				}
			},
			expectedIxQueue: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending: 0,
					queued:  2,
				},
				ids[1]: {
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

			for id, ixQueue := range testcase.expectedIxQueue {
				require.Equal(t, ixQueue.pending, len(response.Pending[id]))
				require.Equal(t, ixQueue.queued, len(response.Queued[id]))
			}
		})
	}
}

//nolint:dupl
func TestPublicIXPoolAPI_ContentFrom(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	ids := tests.GetRandomIDs(t, 2)

	testcases := []struct {
		name            string
		args            *rpcargs.IxPoolArgs
		accounts        map[identifiers.Identifier]*account
		testFn          func(accounts map[identifiers.Identifier]*account)
		expectedIxQueue *expectedResult
		expectedErr     error
	}{
		{
			name: "nil id",
			args: &rpcargs.IxPoolArgs{
				ID: identifiers.Nil,
			},
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "Ix pool with no interactions",
			args: &rpcargs.IxPoolArgs{
				ID: ids[0],
			},
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued:  []*common.Interaction{},
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
				ID: ids[0],
			},
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
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
				ID: ids[0],
			},
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
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
				ID: ids[1],
			},
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
				ids[1]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 3
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
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
	ids := tests.GetRandomIDs(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Identifier]*account
		testFn          func(accounts map[identifiers.Identifier]*account)
		expectedIxQueue *expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued:  []*common.Interaction{},
				},
			},
			expectedIxQueue: &expectedResult{
				pending: 0,
				queued:  0,
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
				ids[1]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 3
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
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
	ids := tests.GetRandomIDs(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Identifier]*account
		testFn          func(accounts map[identifiers.Identifier]*account)
		expectedAccInfo map[identifiers.Identifier]*expectedResult
	}{
		{
			name: "Ix pool with no interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending:  []*common.Interaction{},
					queued:   []*common.Interaction{},
					waitTime: 0,
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setWaitTime(id, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending:  0,
					queued:   0,
					waitTime: 0,
					expired:  true,
				},
			},
		},
		{
			name: "Ix pool with one pending interaction",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
							ixData.IxOps = []common.IxOpRaw{
								{
									Type:    common.IxAssetCreate,
									Payload: tests.CreateRawAssetCreatePayload(t),
								},
							}
							ixData.Participants = []common.IxParticipant{
								{
									ID:       ids[0],
									LockType: common.MutateLock,
								},
							}
						}),
					},
					queued:   []*common.Interaction{},
					waitTime: int64(1500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
					ixpool.setWaitTime(id, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending:  1,
					queued:   0,
					waitTime: int64(1500 * time.Millisecond),
					expired:  false,
				},
			},
		},
		{
			name: "Ix pool with one queued interaction",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
							ixData.IxOps = []common.IxOpRaw{
								{
									Type:    common.IxAssetCreate,
									Payload: tests.CreateRawAssetCreatePayload(t),
								},
							}
						}),
					},
					waitTime: int64(2500 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
					ixpool.setWaitTime(id, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending:  0,
					queued:   1,
					waitTime: int64(2500 * time.Millisecond),
					expired:  false,
				},
			},
		},
		{
			name: "Ix pool with multiple pending and queued interactions",
			accounts: map[identifiers.Identifier]*account{
				ids[0]: {
					pending: []*common.Interaction{},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[0]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(500 * time.Millisecond),
				},
				ids[1]: {
					pending: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 1
							ixData.FuelPrice = big.NewInt(100)
						}),
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 2
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					queued: []*common.Interaction{
						newTestInteraction(t, common.IxAssetCreate, func(ixData *common.IxData) {
							ixData.Sender.ID = ids[1]
							ixData.Sender.SequenceID = 3
							ixData.FuelPrice = big.NewInt(100)
						}),
					},
					waitTime: int64(1000 * time.Millisecond),
				},
			},
			testFn: func(accounts map[identifiers.Identifier]*account) {
				for id, account := range accounts {
					ixpool.setIxs(id, account.pending, account.queued)
					ixpool.setWaitTime(id, account.waitTime)
				}
			},
			expectedAccInfo: map[identifiers.Identifier]*expectedResult{
				ids[0]: {
					pending:  0,
					queued:   2,
					waitTime: int64(500 * time.Millisecond),
					expired:  false,
				},
				ids[1]: {
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

			for id, accInfo := range testcase.expectedAccInfo {
				require.Equal(t, accInfo.pending, len(response.Pending[id.Hex()]))
				require.Equal(t, accInfo.queued, len(response.Queued[id.Hex()]))
				require.Equal(t, big.NewInt(accInfo.waitTime), response.WaitTime[id.Hex()].Time.ToInt())
				require.Equal(t, accInfo.expired, response.WaitTime[id.Hex()].Expired)

				if len(testcase.accounts[id].pending) > 0 {
					interactionInfo := response.Pending[id.Hex()]
					require.NotNil(t, interactionInfo)

					for _, ix := range testcase.accounts[id].pending {
						require.NotNil(t, interactionInfo[strconv.Itoa(int(ix.SequenceID()))])
					}
				}

				if len(testcase.accounts[id].queued) > 0 {
					interactionInfo := response.Queued[id.Hex()]
					require.NotNil(t, interactionInfo)

					for _, ix := range testcase.accounts[id].queued {
						require.NotNil(t, interactionInfo[strconv.Itoa(int(ix.SequenceID()))])
					}
				}
			}
		})
	}
}

func TestPublicIXPoolAPI_WaitTime(t *testing.T) {
	ixpool := NewMockIxPool(t)
	ixPoolAPI := NewPublicIXPoolAPI(ixpool)
	id := tests.RandomIdentifier(t)
	waitTime := int64(3500 * time.Millisecond)

	ixpool.setWaitTime(id, waitTime)

	testcases := []struct {
		name             string
		args             *rpcargs.IxPoolArgs
		expectedWaitTime int64
		expectedErr      error
	}{
		{
			name: "nil id",
			args: &rpcargs.IxPoolArgs{
				ID: identifiers.Nil,
			},
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "Account without state",
			args: &rpcargs.IxPoolArgs{
				ID: tests.RandomIdentifier(t),
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "Account with state",
			args: &rpcargs.IxPoolArgs{
				ID: id,
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
