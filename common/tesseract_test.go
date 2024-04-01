package common_test

import (
	"math/big"
	"reflect"
	"testing"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestCopyState(t *testing.T) {
	testcases := []struct {
		name  string
		state common.State
	}{
		{
			name:  "copy state",
			state: tests.CreateStateWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedState := test.state

			copiedState := test.state.Copy()

			require.Equal(t, expectedState, copiedState)
			require.False(
				t,
				&expectedState.ContextDelta.BehaviouralNodes[0] == &copiedState.ContextDelta.BehaviouralNodes[0],
			)
		})
	}
}

func TestCopyParticipant(t *testing.T) {
	address := tests.RandomAddress(t)

	testcases := []struct {
		name         string
		participants common.Participants
	}{
		{
			name: "copy participants",
			participants: common.Participants{
				address: tests.CreateStateWithTestData(t),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedParticipants := test.participants

			copiedParticipants := test.participants.Copy()

			require.Equal(t, expectedParticipants, copiedParticipants)
			require.NotEqual(t,
				reflect.ValueOf(expectedParticipants).Pointer(),
				reflect.ValueOf(copiedParticipants).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(expectedParticipants[address].ContextDelta.BehaviouralNodes).Pointer(),
				reflect.ValueOf(copiedParticipants[address].ContextDelta.BehaviouralNodes).Pointer(),
			)
		})
	}
}

func TestCopyPoXtData(t *testing.T) {
	testcases := []struct {
		name string
		poxt common.PoXtData
	}{
		{
			name: "copy tesseract poxt data",
			poxt: tests.CreatePoXtWithTestData(t),
		},
		{
			name: "empty signatures and votesets",
			poxt: common.PoXtData{
				ClusterID: "cluster",
				Round:     5,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedPoXt := test.poxt

			copiedPoXt := test.poxt.Copy()

			require.Equal(t, expectedPoXt, copiedPoXt)

			if expectedPoXt.BFTVoteSet != nil {
				require.False(t, &expectedPoXt.BFTVoteSet.Elements[0] == &copiedPoXt.BFTVoteSet.Elements[0])
			}

			if expectedPoXt.ICSVoteset != nil {
				require.False(t, &expectedPoXt.ICSVoteset.Elements[0] == &copiedPoXt.ICSVoteset.Elements[0])
			}

			if len(expectedPoXt.CommitSignature) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(expectedPoXt.CommitSignature).Pointer(),
					reflect.ValueOf(copiedPoXt.CommitSignature).Pointer(),
				)
			}

			if len(expectedPoXt.ICSSignature) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(expectedPoXt.ICSSignature).Pointer(),
					reflect.ValueOf(copiedPoXt.ICSSignature).Pointer(),
				)
			}
		})
	}
}

func TestNewTesseract(t *testing.T) {
	var (
		address  = tests.RandomAddress(t)
		ixParams = tests.GetIxParamsMapWithAddresses(
			[]identifiers.Address{tests.RandomAddress(t)},
			[]identifiers.Address{tests.RandomAddress(t)},
		)
	)

	testcases := []struct {
		name             string
		participants     common.Participants
		interactionsHash common.Hash
		receiptHash      common.Hash
		epoch            *big.Int
		timestamp        int64
		operator         string
		fuelUsed         uint64
		fuelLimit        uint64
		consensusInfo    common.PoXtData
		seal             []byte
		sealBy           kramaid.KramaID
		ixns             common.Interactions
		receipts         common.Receipts
	}{
		{
			name: "create new tesseract",
			participants: common.Participants{
				address: tests.CreateStateWithTestData(t),
			},
			interactionsHash: tests.RandomHash(t),
			receiptHash:      tests.RandomHash(t),
			epoch:            big.NewInt(3),
			timestamp:        44,
			operator:         "operator",
			fuelUsed:         34,
			fuelLimit:        33,
			consensusInfo:    tests.CreatePoXtWithTestData(t),
			seal:             []byte{1, 2, 3},
			sealBy:           tests.RandomKramaIDs(t, 1)[0],
			ixns:             tests.CreateIxns(t, 1, ixParams),
			receipts:         tests.CreateReceiptsWithTestData(t, tests.RandomHash(t)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tesseract := common.NewTesseract(
				test.participants,
				test.interactionsHash,
				test.receiptHash,
				test.epoch,
				test.timestamp,
				test.operator,
				test.fuelUsed,
				test.fuelLimit,
				test.consensusInfo,
				test.seal,
				test.sealBy,
				test.ixns,
				test.receipts,
			)

			require.Equal(t, test.participants, tesseract.Participants())
			require.Equal(t, test.interactionsHash, tesseract.InteractionsHash())
			require.Equal(t, test.receiptHash, tesseract.ReceiptsHash())
			require.Equal(t, test.epoch, tesseract.Epoch())
			require.Equal(t, test.timestamp, tesseract.Timestamp())
			require.Equal(t, test.operator, tesseract.Operator())
			require.Equal(t, test.fuelUsed, tesseract.FuelUsed())
			require.Equal(t, test.fuelLimit, tesseract.FuelLimit())
			require.Equal(t, test.consensusInfo, tesseract.ConsensusInfo())
			require.Equal(t, test.seal, tesseract.Seal())
			require.Equal(t, test.sealBy, tesseract.SealBy())
			require.Equal(t, test.ixns, tesseract.Interactions())
			require.Equal(t, test.receipts, tesseract.Receipts())

			// modifying values is the only way to check if values are copied, as methods on copied value
			// always return copy regardless of whether value copied or not in new tesseract
			// make sure consensus info is copied
			test.consensusInfo.CommitSignature[0] = 22
			require.NotEqual(t,
				test.consensusInfo.CommitSignature,
				tesseract.ConsensusInfo().CommitSignature,
			)

			// make sure epoch is copied
			test.epoch = big.NewInt(100)
			require.NotEqual(t,
				test.epoch,
				tesseract.Epoch(),
			)

			// make sure participants doesn't match
			test.participants[address].ContextDelta.BehaviouralNodes[0] = tests.RandomKramaIDs(t, 1)[0]
			require.NotEqual(t,
				test.participants[address].ContextDelta.BehaviouralNodes,
				tesseract.Participants()[address].ContextDelta.BehaviouralNodes,
			)

			// make sure seal is copied
			test.seal[0] = 99
			require.NotEqual(t,
				test.seal,
				tesseract.Seal(),
			)

			// make sure receipts copied
			test.receipts[tests.RandomHash(t)] = &common.Receipt{}
			require.NotEqual(t,
				test.receipts,
				tesseract.Receipts(),
			)
		})
	}
}
