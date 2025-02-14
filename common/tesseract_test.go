package common_test

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
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
				&expectedState.ContextDelta.ConsensusNodes[0] == &copiedState.ContextDelta.ConsensusNodes[0],
			)
		})
	}
}

func TestCopyParticipant(t *testing.T) {
	id := tests.RandomIdentifier(t)

	testcases := []struct {
		name         string
		participants common.ParticipantsState
	}{
		{
			name: "copy participants",
			participants: common.ParticipantsState{
				id: tests.CreateStateWithTestData(t),
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
				reflect.ValueOf(expectedParticipants[id].ContextDelta.ConsensusNodes).Pointer(),
				reflect.ValueOf(copiedParticipants[id].ContextDelta.ConsensusNodes).Pointer(),
			)
		})
	}
}

func TestNewTesseract(t *testing.T) {
	var (
		id       = tests.RandomIdentifier(t)
		ixParams = tests.GetIxParamsMapWithIDs(
			t,
			[]identifiers.Identifier{tests.RandomIdentifier(t)},
			[]identifiers.Identifier{tests.RandomIdentifier(t)},
		)
	)

	testcases := []struct {
		name             string
		participants     common.ParticipantsState
		interactionsHash common.Hash
		receiptHash      common.Hash
		epoch            *big.Int
		timestamp        uint64
		fuelUsed         uint64
		fuelLimit        uint64
		consensusInfo    common.PoXtData
		seal             []byte
		sealBy           kramaid.KramaID
		ixns             common.Interactions
		receipts         common.Receipts
		commitInfo       common.CommitInfo
	}{
		{
			name: "create new tesseract",
			participants: common.ParticipantsState{
				id: tests.CreateStateWithTestData(t),
			},
			interactionsHash: tests.RandomHash(t),
			receiptHash:      tests.RandomHash(t),
			epoch:            big.NewInt(3),
			timestamp:        44,
			fuelUsed:         34,
			fuelLimit:        33,
			consensusInfo:    tests.CreatePoXtWithTestData(t, 1),
			seal:             []byte{1, 2, 3},
			sealBy:           tests.RandomKramaIDs(t, 1)[0],
			ixns: common.NewInteractionsWithLeaderCheck(false,
				tests.CreateIxns(t, 1, ixParams)...),
			receipts:   tests.CreateReceiptsWithTestData(t, tests.RandomHash(t)),
			commitInfo: tests.CreateCommitInfoWithTestData(t),
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
				test.fuelUsed,
				test.fuelLimit,
				test.consensusInfo,
				test.seal,
				test.sealBy,
				test.ixns,
				test.receipts,
				&test.commitInfo,
			)

			require.Equal(t, test.participants, tesseract.Participants())
			require.Equal(t, test.interactionsHash, tesseract.InteractionsHash())
			require.Equal(t, test.receiptHash, tesseract.ReceiptsHash())
			require.Equal(t, test.epoch, tesseract.Epoch())
			require.Equal(t, test.timestamp, tesseract.Timestamp())
			require.Equal(t, test.fuelUsed, tesseract.FuelUsed())
			require.Equal(t, test.fuelLimit, tesseract.FuelLimit())
			require.Equal(t, test.consensusInfo, tesseract.ConsensusInfo())
			require.Equal(t, test.seal, tesseract.Seal())
			require.Equal(t, test.sealBy, tesseract.SealBy())
			require.Equal(t, test.ixns, tesseract.Interactions())
			require.Equal(t, test.receipts, tesseract.Receipts())
			require.Equal(t, test.commitInfo, *tesseract.CommitInfo())

			// modifying values is the only way to check if values are copied, as methods on copied value
			// always return copy regardless of whether value copied or not in new tesseract
			// make sure consensus info is copied
			// make sure epoch is copied
			test.epoch = big.NewInt(100)
			require.NotEqual(t,
				test.epoch,
				tesseract.Epoch(),
			)

			// make sure participants doesn't match
			test.participants[id].ContextDelta.ConsensusNodes[0] = tests.RandomKramaIDs(t, 1)[0]
			require.NotEqual(t,
				test.participants[id].ContextDelta.ConsensusNodes,
				tesseract.Participants()[id].ContextDelta.ConsensusNodes,
			)

			// make sure seal is copied
			test.seal[0] = 99
			require.NotEqual(t,
				test.seal,
				tesseract.Seal(),
			)
		})
	}
}
