package args

import (
	"encoding/json"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

// CheckForRPCTesseract validates fields of rpc tesseract
func CheckForRPCTesseract(
	t *testing.T,
	ts *common.Tesseract,
	rpcTS *RPCTesseract,
) {
	t.Helper()

	var grid map[common.Address]common.TesseractHeightAndHash

	CheckForRPCHeader(t, ts.Header(), rpcTS.Header)
	CheckForRPCBody(t, ts.Body(), rpcTS.Body)
	require.Equal(t, ts.Seal(), rpcTS.Seal.Bytes())

	require.Equal(t, ts.Hash(), rpcTS.Hash)

	if ts.ClusterID() == common.GenesisIdentifier {
		for _, ix := range rpcTS.Ixns {
			require.Nil(t, ix)
		}

		return
	}

	parts, err := ts.Parts()
	if err == nil {
		grid = parts.Grid
	}

	for i, ixn := range ts.Interactions() {
		CheckForRPCIxn(t, ixn, rpcTS.Ixns[i], grid)
	}
}

func CheckForRPCHeader(t *testing.T, header common.TesseractHeader, rpcHeader RPCHeader) {
	t.Helper()

	require.Equal(t, header.Address, rpcHeader.Address)
	require.Equal(t, header.PrevHash, rpcHeader.PrevHash)
	require.Equal(t, header.Height, rpcHeader.Height.ToUint64())
	require.Equal(t, header.FuelUsed, rpcHeader.FuelUsed.ToUint64())
	require.Equal(t, header.FuelLimit, rpcHeader.FuelLimit.ToUint64())
	require.Equal(t, header.BodyHash, rpcHeader.BodyHash)
	require.Equal(t, header.GroupHash, rpcHeader.GridHash)
	require.Equal(t, header.Operator, rpcHeader.Operator)
	require.Equal(t, header.ClusterID, rpcHeader.ClusterID)
	require.Equal(t, uint64(header.Timestamp), rpcHeader.Timestamp.ToUint64())
	CheckForRPCContextLockInfos(t, header.ContextLock, rpcHeader.ContextLock)

	checkForRPCCommitData(t, header.Extra, rpcHeader.Extra)
}

func CheckForRPCBody(t *testing.T, body common.TesseractBody, rpcBody RPCBody) {
	t.Helper()

	require.Equal(t, body.StateHash, rpcBody.StateHash)
	require.Equal(t, body.ContextHash, rpcBody.ContextHash)
	require.Equal(t, body.InteractionHash, rpcBody.InteractionHash)
	require.Equal(t, body.ReceiptHash, rpcBody.ReceiptHash)
	CheckForRPCDeltaGroups(t, body.ContextDelta, rpcBody.ContextDelta)
	require.Equal(t, body.ConsensusProof, rpcBody.ConsensusProof)
}

// CheckForRPCIxn validates a field from input, compute, trust, and verifies payload.
func CheckForRPCIxn(
	t *testing.T,
	ix *common.Interaction,
	rpcIxn *RPCInteraction,
	grid map[common.Address]common.TesseractHeightAndHash,
) {
	t.Helper()

	if len(grid) != 0 {
		CheckForRPCTesseractParts(t, grid, rpcIxn.Parts)
	}

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
			if assetID == rpcAssetID {
				flag = true

				require.Equal(t, amount, rpcAmount.ToInt())
			}
		}

		require.True(t, flag)
	}

	for assetID, amount := range input.PerceivedValues {
		flag := false

		for rpcAssetID, rpcAmount := range rpcIxn.PerceivedValues {
			if assetID == rpcAssetID {
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

	case common.IxLogicDeploy:
		fallthrough

	case common.IxLogicInvoke:
		logicPayload := new(common.LogicPayload)

		err := logicPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcLogicPayload := &RPCLogicPayload{
			Manifest: (hexutil.Bytes)(logicPayload.Manifest),
			LogicID:  logicPayload.Logic.String(),
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

func CheckForRPCContextLockInfos(
	t *testing.T,
	expectedContextLockInfos map[common.Address]common.ContextLockInfo,
	rpcContextLockInfos RPCContextLockInfos,
) {
	t.Helper()

	if len(expectedContextLockInfos) == 0 {
		require.Nil(t, rpcContextLockInfos)

		return
	}

	require.Equal(t, len(expectedContextLockInfos), len(rpcContextLockInfos))

	for _, rpcContextLockInfo := range rpcContextLockInfos {
		contextLockInfo, ok := expectedContextLockInfos[rpcContextLockInfo.Address]
		require.True(t, ok)

		require.Equal(t, contextLockInfo.ContextHash, rpcContextLockInfo.ContextHash)
		require.Equal(t, contextLockInfo.Height, rpcContextLockInfo.Height.ToUint64())
		require.Equal(t, contextLockInfo.TesseractHash, rpcContextLockInfo.TesseractHash)
	}

	for i := 1; i < len(rpcContextLockInfos); i++ {
		require.True(t, rpcContextLockInfos[i-1].Address.Hex() < rpcContextLockInfos[i].Address.Hex())
	}
}

func checkForRPCCommitData(t *testing.T, commitData common.CommitData, rpcCommitData RPCCommitData) {
	t.Helper()

	require.Equal(t, uint64(commitData.Round), rpcCommitData.Round.ToUint64())
	require.Equal(t, commitData.CommitSignature, rpcCommitData.CommitSignature.Bytes())
	require.Equal(t, commitData.VoteSet.String(), rpcCommitData.VoteSet)
	require.Equal(t, commitData.EvidenceHash, rpcCommitData.EvidenceHash)

	if commitData.GridID != nil {
		CheckForRPCTesseractGridID(t, commitData.GridID, rpcCommitData.GridID)
	}
}

func CheckForRPCDeltaGroups(
	t *testing.T,
	expectedRPCDeltaGroups map[common.Address]*common.DeltaGroup,
	rpcDeltaGroups RPCDeltaGroups,
) {
	t.Helper()

	if len(expectedRPCDeltaGroups) == 0 {
		require.Nil(t, rpcDeltaGroups)

		return
	}

	require.Equal(t, len(expectedRPCDeltaGroups), len(rpcDeltaGroups))

	for _, rpcDeltaGroup := range rpcDeltaGroups {
		deltaGroup, ok := expectedRPCDeltaGroups[rpcDeltaGroup.Address]
		require.True(t, ok)

		require.Equal(t, deltaGroup.Role, rpcDeltaGroup.Role)
		require.Equal(t, deltaGroup.BehaviouralNodes, rpcDeltaGroup.BehaviouralNodes)
		require.Equal(t, deltaGroup.RandomNodes, rpcDeltaGroup.RandomNodes)
		require.Equal(t, deltaGroup.ReplacedNodes, rpcDeltaGroup.ReplacedNodes)
	}

	for i := 1; i < len(rpcDeltaGroups); i++ {
		require.True(t, rpcDeltaGroups[i-1].Address.Hex() < rpcDeltaGroups[i].Address.Hex())
	}
}

func CheckForRPCTesseractParts(
	t *testing.T,
	grid map[common.Address]common.TesseractHeightAndHash,
	rpcParts RPCTesseractParts,
) {
	t.Helper()

	require.Equal(t, len(grid), len(rpcParts))

	for _, rpcPart := range rpcParts {
		heightAndHash, ok := grid[rpcPart.Address]
		require.True(t, ok)

		require.Equal(t, heightAndHash.Hash, rpcPart.Hash)
		require.Equal(t, heightAndHash.Height, rpcPart.Height.ToUint64())
	}

	CheckIfPartsSorted(t, rpcParts)
}

func CheckForRPCTesseractGridID(
	t *testing.T,
	tesseractGridID *common.TesseractGridID,
	rpcTesseractGridID *RPCTesseractGridID,
) {
	t.Helper()

	if tesseractGridID == nil {
		require.Nil(t, rpcTesseractGridID)

		return
	}

	require.Equal(t, tesseractGridID.Hash, rpcTesseractGridID.Hash)

	if tesseractGridID.Parts != nil {
		require.Equal(t, uint64(tesseractGridID.Parts.Total), rpcTesseractGridID.Total.ToUint64())
		CheckForRPCTesseractParts(t, tesseractGridID.Parts.Grid, rpcTesseractGridID.Parts)

		return
	}

	require.Equal(t, 0, int(rpcTesseractGridID.Total))
}

func CheckIfPartsSorted(t *testing.T, parts RPCTesseractParts) {
	t.Helper()

	for i := 1; i < len(parts); i++ {
		require.True(t, parts[i-1].Address.Hex() < parts[i].Address.Hex())
	}
}

func CheckForRPCReceipt(
	t *testing.T,
	grid map[common.Address]common.TesseractHeightAndHash,
	ix *common.Interaction,
	receipt *common.Receipt,
	rpcReceipt *RPCReceipt,
	ixIndex int,
) {
	t.Helper()

	CheckForRPCTesseractParts(t, grid, rpcReceipt.Parts)
	require.Equal(t, uint64(receipt.IxType), rpcReceipt.IxType.ToUint64())
	require.Equal(t, receipt.IxHash, rpcReceipt.IxHash)
	require.Equal(t, receipt.FuelUsed, uint64(rpcReceipt.FuelUsed))
	checkForRPCHashes(t, receipt.Hashes, rpcReceipt.Hashes)
	require.Equal(t, receipt.ExtraData, rpcReceipt.ExtraData)
	require.Equal(t, ix.Sender(), rpcReceipt.From)
	require.Equal(t, ix.Receiver(), rpcReceipt.To)
	require.Equal(t, uint64(ixIndex), rpcReceipt.IXIndex.ToUint64())
}

func checkForRPCHashes(
	t *testing.T,
	expectedRPCHashes common.ReceiptAccHashes,
	rpcHashes RPCHashes,
) {
	t.Helper()

	require.Equal(t, len(expectedRPCHashes), len(rpcHashes))

	for _, rpcHash := range rpcHashes {
		stateHash := expectedRPCHashes.StateHash(rpcHash.Address)
		contextHash := expectedRPCHashes.ContextHash(rpcHash.Address)

		require.Equal(t, stateHash, rpcHash.StateHash)
		require.Equal(t, contextHash, rpcHash.ContextHash)
	}

	for i := 1; i < len(rpcHashes); i++ {
		require.True(t, rpcHashes[i-1].Address.Hex() < rpcHashes[i].Address.Hex())
	}
}

func CreateInteractionWithTestData(t *testing.T, ixType common.IxType, payload []byte) *common.Interaction {
	t.Helper()

	ixData := common.IxData{
		Input:   tests.CreateIXInputWithTestData(t, ixType, payload, []byte{187, 1, 29, 103}),
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ix, err := common.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	return ix
}

func CreateHeaderCallbackWithTestData(t *testing.T) func(header *common.TesseractHeader) {
	t.Helper()

	return func(header *common.TesseractHeader) {
		header.Address = tests.RandomAddress(t)
		header.PrevHash = tests.RandomHash(t)
		header.Height = 4
		header.FuelUsed = 88
		header.FuelLimit = 99
		header.BodyHash = tests.RandomHash(t)
		header.GroupHash = tests.RandomHash(t)
		header.Operator = "operator"
		header.ClusterID = "cluster-id"
		header.Timestamp = 3
		header.ContextLock = make(map[common.Address]common.ContextLockInfo)
		header.ContextLock[tests.RandomAddress(t)] = common.ContextLockInfo{
			Height: 4,
		}
		header.Extra = tests.CreateCommitDataWithTestData(t)
	}
}
