package args

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/hexutil"

	"github.com/sarvalabs/go-moi/common/tests"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
)

// CheckForRPCTesseract validates fields of rpc tesseract
func CheckForRPCTesseract(
	t *testing.T,
	ts *common.Tesseract,
	rpcTS *RPCTesseract,
) {
	t.Helper()

	CheckForRPCParticipantState(t, ts.Participants(), rpcTS.Participants)
	CheckForRPCPoxtData(t, ts.ConsensusInfo(), rpcTS.ConsensusInfo)

	require.Equal(t, ts.InteractionsHash(), rpcTS.InteractionsHash)
	require.Equal(t, ts.ReceiptsHash(), rpcTS.ReceiptsHash)
	require.Equal(t, ts.Epoch(), rpcTS.Epoch.ToInt())
	require.Equal(t, ts.Timestamp(), rpcTS.TimeStamp.ToUint64())
	require.Equal(t, ts.FuelUsed(), rpcTS.FuelUsed.ToUint64())
	require.Equal(t, ts.FuelLimit(), rpcTS.FuelLimit.ToUint64())
	require.Equal(t, ts.Seal(), rpcTS.Seal.Bytes())
	require.Equal(t, ts.Hash(), rpcTS.Hash)

	if ts.ConsensusInfo().View == common.GenesisView {
		for _, ix := range rpcTS.Ixns {
			require.Nil(t, ix)
		}

		return
	}

	require.Equal(t, len(ts.Interactions().IxList()), len(rpcTS.Ixns))

	for i, ixn := range ts.Interactions().IxList() {
		CheckForRPCIxn(t, ixn, ts.Hash(), nil, rpcTS.Ixns[i])
	}

	CheckForRPCCommitInfo(t, ts.CommitInfo(), &rpcTS.CommitInfo)
}

func CheckForRPCCommitInfo(t *testing.T, info *common.CommitInfo, rpcInfo *RPCCommitInfo) {
	t.Helper()

	if info == nil {
		return
	}

	require.Equal(t, info.QC.Type, rpcInfo.QC.Type)
	require.Equal(t, info.QC.ID, rpcInfo.QC.ID)
	require.Equal(t, info.QC.LockType, rpcInfo.QC.LockType)
	require.Equal(t, info.QC.View, rpcInfo.QC.View)
	require.Equal(t, info.QC.TSHash, rpcInfo.QC.TSHash)
	require.Equal(t, info.QC.SignerIndices.String(), rpcInfo.QC.SignerIndices)
	require.Equal(t, info.QC.Signature, rpcInfo.QC.Signature)

	require.Equal(t, info.Operator, rpcInfo.Operator)
	require.Equal(t, info.ClusterID, rpcInfo.ClusterID)
	require.Equal(t, info.View, rpcInfo.View)
	require.Equal(t, info.RandomSet, rpcInfo.RandomSet)
	require.Equal(t, info.RandomSetSizeWithoutDelta, rpcInfo.RandomSetSizeWithoutDelta)
}

// CheckForRPCIxn validates a field from input, compute, trust, and verifies payload.
func CheckForRPCIxn(
	t *testing.T,
	ix *common.Interaction,
	tsHash common.Hash,
	participantsState common.ParticipantsState,
	rpcIxn *RPCInteraction,
) {
	t.Helper()

	require.Equal(t, tsHash, rpcIxn.TSHash)
	CheckForRPCParticipantState(t, participantsState, rpcIxn.ParticipantsState)
	CheckForRPCIxParticipants(t, ix.IxParticipants(), rpcIxn.IxParticipants)
	CheckForRPCFunds(t, ix.Funds(), rpcIxn.Funds)

	input := ix.IXData()

	require.Equal(t, ix.Hash(), rpcIxn.Hash)
	require.Equal(t, len(ix.Signatures()), len(rpcIxn.Signatures))

	for i := 0; i < len(ix.Signatures()); i++ {
		require.Equal(t, ix.Signatures()[i].ID, rpcIxn.Signatures[i].ID)
		require.Equal(t, ix.Signatures()[i].KeyID, rpcIxn.Signatures[i].KeyID.ToUint64())
		require.Equal(t, ix.Signatures()[i].Signature, rpcIxn.Signatures[i].Signature.Bytes())
	}

	require.Equal(t, input.Sender.SequenceID, rpcIxn.Sender.SequenceID.ToUint64())

	require.Equal(t, input.Sender.ID, rpcIxn.Sender.ID)
	require.Equal(t, input.Sender.SequenceID, rpcIxn.Sender.SequenceID.ToUint64())
	require.Equal(t, input.Sender.KeyID, rpcIxn.Sender.KeyID.ToUint64())
	require.Equal(t, input.Payer, rpcIxn.Payer)

	require.Equal(t, len(input.IxOps), len(rpcIxn.IxOps))
	require.Equal(t, input.FuelLimit, uint64(rpcIxn.FuelLimit))
	require.Equal(t, input.FuelPrice, rpcIxn.FuelPrice.ToInt())

	for idx, op := range input.IxOps {
		require.Equal(t, op.Type, rpcIxn.IxOps[idx].Type)

		switch op.Type {
		case common.IxParticipantCreate:
			participantCreatePayload := new(common.ParticipantCreatePayload)
			err := participantCreatePayload.FromBytes(op.Payload)
			require.NoError(t, err)

			rpcAssetActionPayload := RPCParticipantCreate{
				ID:     participantCreatePayload.ID,
				Amount: (*hexutil.Big)(participantCreatePayload.Amount),
			}

			expectedPayload, err := json.Marshal(rpcAssetActionPayload)
			require.NoError(t, err)

			require.Equal(t, expectedPayload, []byte(rpcIxn.IxOps[idx].Payload))

		case common.IxAssetTransfer:
			assetCreationPayload := new(common.AssetActionPayload)
			err := assetCreationPayload.FromBytes(op.Payload)
			require.NoError(t, err)

			rpcAssetActionPayload := RPCAssetAction{
				Beneficiary: assetCreationPayload.Beneficiary,
				AssetID:     assetCreationPayload.AssetID,
				Amount:      (*hexutil.Big)(assetCreationPayload.Amount),
				Timestamp:   (*hexutil.Uint64)(&assetCreationPayload.Timestamp),
			}

			expectedPayload, err := json.Marshal(rpcAssetActionPayload)
			require.NoError(t, err)

			require.Equal(t, expectedPayload, []byte(rpcIxn.IxOps[idx].Payload))

		case common.IxAssetCreate:
			assetCreationPayload := new(common.AssetCreatePayload)
			err := assetCreationPayload.FromBytes(op.Payload)
			require.NoError(t, err)

			rpcAssetCreationPayload := RPCAssetCreation{
				Symbol:     assetCreationPayload.Symbol,
				Supply:     (*hexutil.Big)(assetCreationPayload.Supply),
				Dimension:  (*hexutil.Uint8)(&assetCreationPayload.Dimension),
				Standard:   (*hexutil.Uint16)(&assetCreationPayload.Standard),
				IsLogical:  assetCreationPayload.IsLogical,
				IsStateful: assetCreationPayload.IsStateFul,

				Logic: RPCLogicPayloadFromLogicPayload(assetCreationPayload.LogicPayload),
			}

			expectedPayload, err := json.Marshal(rpcAssetCreationPayload)
			require.NoError(t, err)

			require.Equal(t, expectedPayload, []byte(rpcIxn.IxOps[idx].Payload))

		case common.IxLogicDeploy, common.IxLogicInvoke, common.IxLogicEnlist:
			logicPayload := new(common.LogicPayload)

			err := logicPayload.FromBytes(op.Payload)
			require.NoError(t, err)

			rpcLogicPayload := &RPCLogicPayload{
				Manifest: (hexutil.Bytes)(logicPayload.Manifest),
				LogicID:  logicPayload.Logic.String(),
				Callsite: logicPayload.Callsite,
				Calldata: (hexutil.Bytes)(logicPayload.Calldata),
			}

			expectedPayload, err := json.Marshal(rpcLogicPayload)
			require.NoError(t, err)

			require.Equal(t, expectedPayload, []byte(rpcIxn.IxOps[idx].Payload))
		default:
			require.FailNow(t, "invalid ix type")
		}
	}
}

func CheckForRPCIxParticipants(t *testing.T, participants []common.IxParticipant, rpcParticipants RPCIxParticipants) {
	t.Helper()

	require.Equal(t, len(participants), len(rpcParticipants))

	for idx, participant := range participants {
		require.Equal(t, participant.ID, rpcParticipants[idx].ID)
		require.Equal(t, participant.LockType, rpcParticipants[idx].LockType)
	}
}

func CheckForRPCFunds(t *testing.T, funds []common.IxFund, rpcFunds []RPCIxFund) {
	t.Helper()

	require.Equal(t, len(funds), len(rpcFunds))

	for idx, fund := range funds {
		require.Equal(t, fund.AssetID, rpcFunds[idx].AssetID)
		require.Equal(t, fund.Amount, rpcFunds[idx].Amount.ToInt())
	}
}

func CheckForRPCParticipantState(
	t *testing.T,
	participants common.ParticipantsState,
	rpcParticipants RPCParticipantsStates,
) {
	t.Helper()

	if len(participants) == 0 {
		require.Nil(t, rpcParticipants)
	}

	require.Equal(t, len(participants), len(rpcParticipants))

	for _, rpcParticipant := range rpcParticipants {
		participant, ok := participants[rpcParticipant.ID]
		require.True(t, ok)

		require.Equal(t, participant.Height, rpcParticipant.Height.ToUint64())
		require.Equal(t, participant.TransitiveLink, rpcParticipant.TransitiveLink)
		require.Equal(t, participant.LockedContext, rpcParticipant.LockedContext)
		require.Equal(t, participant.ContextDelta, rpcParticipant.ContextDelta)
		require.Equal(t, participant.StateHash, rpcParticipant.StateHash)
	}
	// check if participants are sorted
	for i := 1; i < len(rpcParticipants); i++ {
		require.True(t, rpcParticipants[i-1].ID.Hex() < rpcParticipants[i].ID.Hex())
	}
}

func CheckForRPCPoxtData(t *testing.T, poxt common.PoXtData, rpcPoxt RPCPoXtData) {
	t.Helper()

	require.Equal(t, poxt.Proposer, rpcPoxt.Proposer)
	require.Equal(t, poxt.EvidenceHash, rpcPoxt.EvidenceHash)
	require.Equal(t, poxt.BinaryHash, rpcPoxt.BinaryHash)
	require.Equal(t, poxt.IdentityHash, rpcPoxt.IdentityHash)
	require.Equal(t, poxt.View, rpcPoxt.View.ToUint64())
	require.Equal(t, poxt.LastCommit, rpcPoxt.LastCommit)
	require.Equal(t, poxt.AccountLocks, rpcPoxt.AccountLocks)
	require.Equal(t, poxt.ICSSeed, rpcPoxt.ICSSeed)
	require.Equal(t, poxt.ICSProof, rpcPoxt.ICSProof.Bytes())
}

func CheckForRPCReceipt(
	t *testing.T,
	tsHash common.Hash,
	participants common.ParticipantsState,
	ix *common.Interaction,
	receipt *common.Receipt,
	rpcReceipt *RPCReceipt,
	ixIndex int,
) {
	t.Helper()

	require.Equal(t, tsHash, rpcReceipt.TSHash)
	CheckForRPCParticipantState(t, participants, rpcReceipt.Participants)
	require.Equal(t, receipt.IxHash, rpcReceipt.IxHash)
	require.Equal(t, receipt.FuelUsed, uint64(rpcReceipt.FuelUsed))
	require.Equal(t, ix.SenderID(), rpcReceipt.From)
	require.Equal(t, uint64(ixIndex), rpcReceipt.IXIndex.ToUint64())
	require.Len(t, rpcReceipt.IxOps, len(receipt.IxOps))

	for idx, op := range receipt.IxOps {
		require.Equal(t, uint64(op.IxType), rpcReceipt.IxOps[idx].TxType.ToUint64())
		require.Equal(t, op.Status, rpcReceipt.IxOps[idx].Status)
		require.Equal(t, op.Data, rpcReceipt.IxOps[idx].Data)
	}
}

func CreateInteractionWithTestData(t *testing.T, ixType common.IxOpType, payload []byte) *common.Interaction {
	t.Helper()

	ixData := common.IxData{
		Sender: common.Sender{
			ID:         tests.RandomIdentifier(t),
			SequenceID: 2,
		},
		Payer:     tests.RandomIdentifier(t),
		FuelLimit: 1043,
		FuelPrice: new(big.Int).SetUint64(1),
		IxOps: []common.IxOpRaw{
			{
				Type:    ixType,
				Payload: payload,
			},
		},
	}

	tests.AppendParticipantsInIxData(t, &ixData)

	ix, err := common.NewInteraction(ixData, common.Signatures{
		{
			ID:        tests.RandomIdentifier(t),
			KeyID:     2,
			Signature: tests.RandomHash(t).Bytes(),
		},
		{
			ID:        tests.RandomIdentifier(t),
			KeyID:     3,
			Signature: tests.RandomHash(t).Bytes(),
		},
	})
	require.NoError(t, err)

	return ix
}

func checkForRPCPeersScore(t *testing.T, rpcPeersScore RPCPeersScore, peers map[peer.ID]*pubsub.PeerScoreSnapshot) {
	t.Helper()

	for _, rpcScore := range rpcPeersScore {
		score, ok := peers[rpcScore.ID]
		require.True(t, ok)

		require.Equal(t, score.Score, rpcScore.GossipScore)
		require.Equal(t, score.AppSpecificScore, rpcScore.AppSpecificScore)
		require.Equal(t, score.IPColocationFactor, rpcScore.IPColocationFactor)
		require.Equal(t, score.BehaviourPenalty, rpcScore.BehaviourPenalty)

		require.Equal(t, len(score.Topics), len(rpcScore.TopicScores))

		for _, rpcTopicScore := range rpcScore.TopicScores {
			topicScore, ok := score.Topics[rpcTopicScore.Name]
			require.True(t, ok)

			require.Equal(t, uint64(topicScore.TimeInMesh.Milliseconds()), rpcTopicScore.TimeInMesh)
			require.Equal(t, topicScore.FirstMessageDeliveries, rpcTopicScore.FirstMessageDeliveries)
			require.Equal(t, topicScore.MeshMessageDeliveries, rpcTopicScore.MeshMessageDeliveries)
			require.Equal(t, topicScore.InvalidMessageDeliveries, rpcTopicScore.InvalidMessageDeliveries)
		}
	}
}
