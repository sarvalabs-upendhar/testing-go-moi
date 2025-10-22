package args

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestCreateRPCInteraction(t *testing.T) {
	t.Helper()

	assetCreatePaylaod := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetAction, err := common.GetAssetActionPayload(identifiers.RandomAssetIDv0(), "Transfer", &common.TransferParams{
		Beneficiary: tests.RandomIdentifier(t),
		Amount:      big.NewInt(100),
	})
	require.NoError(t, err)

	logicDeployPayload := &common.LogicPayload{
		Manifest: tests.RandomHash(t).Bytes(),
	}

	logicInvokePayload := &common.LogicPayload{
		LogicID:  identifiers.MustLogicID(tests.GetLogicID(t, tests.RandomIdentifier(t))),
		Callsite: "call site",
	}

	ixData := tests.CreateIXDataWithTestData(t, func(ixData *common.IxData) {
		ixData.IxOps = []common.IxOpRaw{}
	})

	ixWithNilFields, err := common.NewInteraction(ixData, common.Signatures{
		{
			ID:        ixData.Sender.ID,
			Signature: tests.RandomHash(t).Bytes(),
		},
	})
	require.NoError(t, err)

	tsHash := tests.RandomHash(t)
	participants := tests.CreateParticipantWithTestData(t, 1)

	testcases := []struct {
		name          string
		ix            *common.Interaction
		expectedError error
	}{
		{
			name: "create rpc interaction for participant create interaction",
			ix: CreateInteractionWithTestData(t, common.IxParticipantCreate,
				tests.CreateParticipantCreatePayload(t, tests.RandomIdentifier(t))),
		},
		{
			name: "create rpc interaction for asset transfer interaction",
			ix:   CreateInteractionWithTestData(t, common.IxAssetAction, assetAction),
		},
		{
			name: "create rpc interaction for asset creation interaction",
			ix:   CreateInteractionWithTestData(t, common.IxAssetCreate, assetCreatePaylaod),
		},
		{
			name: "create rpc interaction for logic deploy interaction",
			ix:   CreateInteractionWithTestData(t, common.IxLogicDeploy, logicDeployPayload),
		},
		{
			name: "create rpc interaction for logic execute interaction",
			ix:   CreateInteractionWithTestData(t, common.IxLogicInvoke, logicInvokePayload),
		},
		{
			name: "create rpc interaction with nil transfer values,perceived values, perceived proofs, ",
			ix:   ixWithNilFields,
		},
		{
			name: "create rpc interaction with participants data",
			ix:   CreateInteractionWithTestData(t, common.IxAssetAction, assetAction),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := CreateRPCInteraction(test.ix, tsHash, participants, 0)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			CheckForRPCIxn(t, test.ix, tsHash, participants, rpcIxn)
		})
	}
}

func TestCreateRPCParticipants(t *testing.T) {
	testcases := []struct {
		name         string
		participants common.ParticipantsState
	}{
		{
			name:         "create rpc participants",
			participants: tests.CreateParticipantWithTestData(t, 3),
		},
		{
			name:         "empty participants",
			participants: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcParticipants := CreateRPCParticipantStates(test.participants)

			CheckForRPCParticipantState(t, test.participants, rpcParticipants)
		})
	}
}

func TestCreateRPCPoXtData(t *testing.T) {
	testcases := []struct {
		name     string
		poxtData common.PoXtData
	}{
		{
			name:     "create rpc participants",
			poxtData: tests.CreatePoXtWithTestData(t, common.GenesisView),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcPoXt := CreateRPCPoXtData(test.poxtData)

			CheckForRPCPoxtData(t, test.poxtData, rpcPoXt)
		})
	}
}

func TestCreateRPCTesseract(t *testing.T) {
	participants := tests.CreateParticipantWithTestData(t, 2)

	// make sure to fill at least one field of every field of tesseract so that we can verify that every field is copied
	createTesseractParams := func(view uint64) *tests.CreateTesseractParams {
		return &tests.CreateTesseractParams{
			Participants: participants,
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.InteractionsHash = tests.RandomHash(t)
				ts.ReceiptsHash = tests.RandomHash(t)
				ts.Epoch = big.NewInt(33)
				ts.Timestamp = 443
				ts.Operator = "guardian"
				ts.FuelUsed = 55
				ts.FuelLimit = 88
				ts.ConsensusInfo = tests.CreatePoXtWithTestData(t, view)
				ts.Seal = []byte{2, 3, 4}
				ts.SealBy = tests.RandomKramaIDs(t, 1)[0]
			},
			CommitInfo: &common.CommitInfo{
				QC: &common.Qc{
					Type:     3,
					ID:       tests.RandomIdentifier(t),
					LockType: 2,
					View:     334,
					TSHash:   tests.RandomHash(t),
					SignerIndices: &common.ArrayOfBits{
						Size:     1,                 // represents node tsCount
						Elements: make([]uint64, 1), // each element holds eight votes
					},
					Signature: []byte{0, 1},
				},
				Operator:                  tests.RandomKramaID(t, 1),
				ClusterID:                 "12342",
				View:                      9,
				RandomSet:                 []common.ValidatorIndex{0, 1},
				RandomSetSizeWithoutDelta: 3,
			},
		}
	}

	testcases := []struct {
		name          string
		ixParams      *tests.CreateIxParams
		tsParams      *tests.CreateTesseractParams
		expectedError error
	}{
		{
			name:     "created rpc tesseract for non-genesis tesseract",
			ixParams: tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t)),
			tsParams: createTesseractParams(1),
		},
		{
			name:     "create rpc tesseract for genesis tesseract",
			tsParams: createTesseractParams(common.GenesisView),
		},
		{
			name:     "nil interactions",
			ixParams: nil,
			tsParams: createTesseractParams(common.GenesisView),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.ixParams != nil {
				test.tsParams.Ixns = common.NewInteractionsWithLeaderCheck(false, tests.CreateIX(t, test.ixParams))
			}

			ts := tests.CreateTesseract(t, test.tsParams)

			rpcTS, err := CreateRPCTesseract(ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			CheckForRPCTesseract(t, ts, rpcTS)
		})
	}
}

func TestCreateRPCReceipt(t *testing.T) {
	ixParams := tests.GetIxParamsForTransfer(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t))
	testcases := []struct {
		name         string
		tsHash       common.Hash
		receipt      *common.Receipt
		ix           *common.Interaction
		participants common.ParticipantsState
		ixIndex      int
	}{
		{
			name:         "create rpc receipt",
			tsHash:       tests.RandomHash(t),
			receipt:      tests.CreateReceiptWithTestData(t),
			ix:           tests.CreateIX(t, ixParams),
			participants: tests.CreateParticipantWithTestData(t, 1),
			ixIndex:      8,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt := CreateRPCReceipt(test.receipt, test.ix, test.tsHash, test.participants, test.ixIndex)

			CheckForRPCReceipt(t, test.tsHash, test.participants, test.ix, test.receipt, receipt, test.ixIndex)
		})
	}
}

func TestCreateRPCPeersScore(t *testing.T) {
	peersScore := map[peer.ID]*pubsub.PeerScoreSnapshot{
		tests.RandomPeerID(t): {
			Score: 3,
			Topics: map[string]*pubsub.TopicScoreSnapshot{
				tests.GetRandomUpperCaseString(t, 4): {
					TimeInMesh:               19,
					FirstMessageDeliveries:   21,
					MeshMessageDeliveries:    23,
					InvalidMessageDeliveries: 27,
				},
				tests.GetRandomUpperCaseString(t, 4): {
					TimeInMesh:               31,
					FirstMessageDeliveries:   33,
					MeshMessageDeliveries:    37,
					InvalidMessageDeliveries: 40,
				},
			},
			AppSpecificScore:   22,
			IPColocationFactor: 12,
			BehaviourPenalty:   33,
		},
		tests.RandomPeerID(t): {
			Score: 3,
			Topics: map[string]*pubsub.TopicScoreSnapshot{
				tests.GetRandomUpperCaseString(t, 4): {
					TimeInMesh:               81,
					FirstMessageDeliveries:   83,
					MeshMessageDeliveries:    84,
					InvalidMessageDeliveries: 89,
				},
				tests.GetRandomUpperCaseString(t, 4): {
					TimeInMesh:               91,
					FirstMessageDeliveries:   32,
					MeshMessageDeliveries:    11,
					InvalidMessageDeliveries: 31,
				},
			},
			AppSpecificScore:   62,
			IPColocationFactor: 72,
			BehaviourPenalty:   88,
		},
	}

	testcases := []struct {
		name  string
		peers map[peer.ID]*pubsub.PeerScoreSnapshot
	}{
		{
			name:  "create rpc peer scores",
			peers: peersScore,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcPeersScore := CreateRPCPeersScore(test.peers)
			require.Equal(t, len(rpcPeersScore), len(peersScore))

			// check peersScore sorted by peer id
			for i := 1; i < len(rpcPeersScore); i++ {
				require.True(t, rpcPeersScore[i-1].ID < rpcPeersScore[i].ID)
			}

			// check if topics are sorted
			for i := 0; i < len(rpcPeersScore); i++ {
				for j := 1; j < len(rpcPeersScore[i].TopicScores); j++ {
					require.True(t, rpcPeersScore[i].TopicScores[j-1].Name < rpcPeersScore[i].TopicScores[j].Name)
				}
			}

			checkForRPCPeersScore(t, rpcPeersScore, peersScore)
		})
	}
}
