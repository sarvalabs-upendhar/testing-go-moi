package args

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
)

// CheckForRPCTesseract validates fields of rpc tesseract
func CheckForRPCTesseract(
	t *testing.T,
	ts *common.Tesseract,
	rpcTS *RPCTesseract,
) {
	t.Helper()

	CheckForRPCParticipants(t, ts.Participants(), rpcTS.Participants)
	CheckForRPCPoxtData(t, ts.ConsensusInfo(), rpcTS.ConsensusInfo)
	require.Equal(t, ts.InteractionsHash(), rpcTS.InteractionsHash)
	require.Equal(t, ts.ReceiptsHash(), rpcTS.ReceiptsHash)
	require.Equal(t, ts.Epoch(), rpcTS.Epoch.ToInt())
	require.Equal(t, ts.Timestamp(), rpcTS.TimeStamp.ToUint64())
	require.Equal(t, ts.Operator(), rpcTS.Operator)
	require.Equal(t, ts.FuelUsed(), rpcTS.FuelUsed.ToUint64())
	require.Equal(t, ts.FuelLimit(), rpcTS.FuelLimit.ToUint64())
	require.Equal(t, ts.Seal(), rpcTS.Seal.Bytes())
	require.Equal(t, ts.Hash(), rpcTS.Hash)

	if ts.ClusterID() == common.GenesisIdentifier {
		for _, ix := range rpcTS.Ixns {
			require.Nil(t, ix)
		}

		return
	}

	require.Equal(t, len(ts.Interactions()), len(rpcTS.Ixns))

	for i, ixn := range ts.Interactions() {
		CheckForRPCIxn(t, ixn, ts.Hash(), nil, rpcTS.Ixns[i])
	}
}

// CheckForRPCIxn validates a field from input, compute, trust, and verifies payload.
func CheckForRPCIxn(
	t *testing.T,
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantsState,
	rpcIxn *RPCInteraction,
) {
	t.Helper()

	require.Equal(t, tsHash, rpcIxn.TSHash)
	CheckForRPCParticipants(t, participants, rpcIxn.Participants)

	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	require.Equal(t, ix.Hash(), rpcIxn.Hash)
	require.Equal(t, ix.Signature(), rpcIxn.Signature.Bytes())

	require.Equal(t, input.Type, rpcIxn.Type)
	require.Equal(t, input.Nonce, rpcIxn.Nonce.ToUint64())

	require.Equal(t, input.Sender, rpcIxn.Sender)
	require.Equal(t, input.Receiver, rpcIxn.Receiver)
	require.Equal(t, input.Payer, rpcIxn.Payer)

	require.Equal(t, len(input.TransferValues), len(rpcIxn.TransferValues))
	require.Equal(t, len(input.PerceivedValues), len(rpcIxn.PerceivedValues))

	for assetID, amount := range input.TransferValues {
		flag := false

		for rpcAssetID, rpcAmount := range rpcIxn.TransferValues {
			if assetID.String() == rpcAssetID {
				flag = true

				require.Equal(t, amount, rpcAmount.ToInt())
			}
		}

		require.True(t, flag)
	}

	for assetID, amount := range input.PerceivedValues {
		flag := false

		for rpcAssetID, rpcAmount := range rpcIxn.PerceivedValues {
			if assetID.String() == rpcAssetID {
				flag = true

				require.Equal(t, amount, rpcAmount.ToInt())
			}
		}

		require.True(t, flag)
	}

	require.Equal(t, input.PerceivedProofs, rpcIxn.PerceivedProofs.Bytes())

	require.Equal(t, input.FuelLimit, uint64(rpcIxn.FuelLimit))
	require.Equal(t, input.FuelPrice, rpcIxn.FuelPrice.ToInt())

	require.Equal(t, compute.Mode, rpcIxn.Mode.ToUint64())
	require.Equal(t, compute.Hash, rpcIxn.ComputeHash)
	require.Equal(t, compute.ComputeNodes, rpcIxn.ComputeNodes)

	require.Equal(t, trust.MTQ, uint(rpcIxn.MTQ.ToUint64()))
	require.Equal(t, trust.TrustNodes, rpcIxn.TrustNodes)

	switch ix.Type() {
	case common.IxValueTransfer:
		require.Equal(t, json.RawMessage(nil), rpcIxn.Payload)

	case common.IxAssetCreate:
		assetCreationPayload := new(common.AssetCreatePayload)
		err := assetCreationPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcAssetCreationPayload := RPCAssetCreation{
			Symbol:     assetCreationPayload.Symbol,
			Supply:     (*hexutil.Big)(assetCreationPayload.Supply),
			Dimension:  (*hexutil.Uint8)(&assetCreationPayload.Dimension),
			Standard:   (*hexutil.Uint16)(&assetCreationPayload.Standard),
			IsLogical:  assetCreationPayload.IsLogical,
			IsStateful: assetCreationPayload.IsStateFul,

			Logic: RPClogicPayloadFromLogicPayload(assetCreationPayload.LogicPayload),
		}

		expectedPayload, err := json.Marshal(rpcAssetCreationPayload)
		require.NoError(t, err)

		require.Equal(t, expectedPayload, []byte(rpcIxn.Payload))

	case common.IxLogicDeploy, common.IxLogicInvoke, common.IxLogicEnlist:
		logicPayload := new(common.LogicPayload)

		err := logicPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcLogicPayload := &RPCLogicPayload{
			Manifest: (hexutil.Bytes)(logicPayload.Manifest),
			LogicID:  string(logicPayload.Logic),
			Callsite: logicPayload.Callsite,
			Calldata: (hexutil.Bytes)(logicPayload.Calldata),
		}

		expectedPayload, err := json.Marshal(rpcLogicPayload)
		require.NoError(t, err)

		require.Equal(t, expectedPayload, []byte(rpcIxn.Payload))
	default:
		require.FailNow(t, "invalid ix type")
	}
}

func CheckForRPCParticipants(t *testing.T, participants common.ParticipantsState, rpcParticipants RPCParticipants) {
	t.Helper()

	if len(participants) == 0 {
		require.Nil(t, rpcParticipants)
	}

	require.Equal(t, len(participants), len(rpcParticipants))

	for _, rpcParticipant := range rpcParticipants {
		participant, ok := participants[rpcParticipant.Address]
		require.True(t, ok)

		require.Equal(t, participant.Height, rpcParticipant.Height.ToUint64())
		require.Equal(t, participant.TransitiveLink, rpcParticipant.TransitiveLink)
		require.Equal(t, participant.PreviousContext, rpcParticipant.PrevContext)
		require.Equal(t, participant.LatestContext, rpcParticipant.LatestContext)
		require.Equal(t, participant.ContextDelta, rpcParticipant.ContextDelta)
		require.Equal(t, participant.StateHash, rpcParticipant.StateHash)
	}
	// check if participants are sorted
	for i := 1; i < len(rpcParticipants); i++ {
		require.True(t, rpcParticipants[i-1].Address.Hex() < rpcParticipants[i].Address.Hex())
	}
}

func CheckForRPCPoxtData(t *testing.T, poxt common.PoXtData, rpcPoxt RPCPoXtData) {
	t.Helper()

	require.Equal(t, poxt.EvidenceHash, rpcPoxt.EvidenceHash)
	require.Equal(t, poxt.BinaryHash, rpcPoxt.BinaryHash)
	require.Equal(t, poxt.IdentityHash, rpcPoxt.IdentityHash)
	require.Equal(t, poxt.ICSHash, rpcPoxt.ICSHash)
	require.Equal(t, string(poxt.ClusterID), rpcPoxt.ClusterID)
	require.Equal(t, poxt.ICSSignature, rpcPoxt.ICSSignature.Bytes())
	require.Equal(t, poxt.ICSVoteset.String(), rpcPoxt.ICSVoteset)
	require.Equal(t, uint64(poxt.Round), rpcPoxt.Round.ToUint64())
	require.Equal(t, poxt.CommitSignature, rpcPoxt.CommitSignature.Bytes())
	require.Equal(t, poxt.BFTVoteSet.String(), rpcPoxt.BFTVoteSet)
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
	CheckForRPCParticipants(t, participants, rpcReceipt.Participants)
	require.Equal(t, uint64(receipt.IxType), rpcReceipt.IxType.ToUint64())
	require.Equal(t, receipt.IxHash, rpcReceipt.IxHash)
	require.Equal(t, receipt.FuelUsed, uint64(rpcReceipt.FuelUsed))
	require.Equal(t, receipt.ExtraData, rpcReceipt.ExtraData)
	require.Equal(t, ix.Sender(), rpcReceipt.From)
	require.Equal(t, ix.Receiver(), rpcReceipt.To)
	require.Equal(t, uint64(ixIndex), rpcReceipt.IXIndex.ToUint64())
}

func CreateInteractionWithTestData(t *testing.T, ixType common.IxType, payload []byte) *common.Interaction {
	t.Helper()

	ixData := common.IxData{
		Input:   tests.CreateIXInputWithTestData(t, ixType, payload, []byte{187, 1, 29, 103}),
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.RandomKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ix, err := common.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	return ix
}
