package args

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
)

var LatestTesseractHeight int64 = -1

// CreateRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the interaction type, JSON marshalling it, and storing it in the input payload.
func CreateRPCInteraction(
	ix *common.Interaction,
	grid map[identifiers.Address]common.TesseractHeightAndHash,
	ixIndex int,
) (*RPCInteraction, error) {
	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	rpcIX := &RPCInteraction{
		Parts:   GetRPCTesseractPartsFromGrid(grid),
		IxIndex: hexutil.Uint64(ixIndex),
		Type:    input.Type,
		Nonce:   hexutil.Uint64(input.Nonce),

		Sender:   input.Sender,
		Receiver: input.Receiver,
		Payer:    input.Payer,

		FuelPrice: (*hexutil.Big)(input.FuelPrice),
		FuelLimit: hexutil.Uint64(input.FuelLimit),

		Mode:         hexutil.Uint64(compute.Mode),
		ComputeHash:  compute.Hash,
		ComputeNodes: compute.ComputeNodes,

		MTQ:        hexutil.Uint64(trust.MTQ),
		TrustNodes: trust.TrustNodes,

		Hash:      ix.Hash(),
		Signature: ix.Signature(),
	}

	if len(input.TransferValues) > 0 {
		rpcIX.TransferValues = make(map[string]*hexutil.Big)
		for asset, amount := range input.TransferValues {
			rpcIX.TransferValues[asset.String()] = (*hexutil.Big)(amount)
		}
	}

	if len(input.PerceivedValues) > 0 {
		rpcIX.PerceivedValues = make(map[string]*hexutil.Big)
		for asset, amount := range input.PerceivedValues {
			rpcIX.PerceivedValues[asset.String()] = (*hexutil.Big)(amount)
		}
	}

	if len(input.PerceivedProofs) > 0 {
		rpcIX.PerceivedProofs = input.PerceivedProofs
	}

	var err error

	switch ix.Type() {
	case common.IxValueTransfer:
		break

	case common.IxAssetBurn, common.IxAssetMint:
		assetPayload := new(common.AssetMintOrBurnPayload)
		if err = assetPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcPayload := RPCAssetMintOrBurn{
			AssetID: assetPayload.Asset,
			Amount:  (*hexutil.Big)(assetPayload.Amount),
		}

		rpcIX.Payload, err = json.Marshal(rpcPayload)
		if err != nil {
			return nil, err
		}

	case common.IxAssetCreate:
		assetPayload := new(common.AssetCreatePayload)
		if err = assetPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcAssetPayload := RPCAssetCreation{
			Symbol:    assetPayload.Symbol,
			Supply:    (*hexutil.Big)(assetPayload.Supply),
			Dimension: (*hexutil.Uint8)(&assetPayload.Dimension),
			Standard:  (*hexutil.Uint16)(&assetPayload.Standard),

			IsLogical:  assetPayload.IsLogical,
			IsStateful: assetPayload.IsStateFul,
			Logic:      RPClogicPayloadFromLogicPayload(assetPayload.LogicPayload),
		}

		rpcIX.Payload, err = json.Marshal(rpcAssetPayload)
		if err != nil {
			return nil, err
		}

	case common.IxLogicInvoke, common.IxLogicDeploy:
		logicPayload := new(common.LogicPayload)

		if err = logicPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcIX.Payload, err = json.Marshal(RPClogicPayloadFromLogicPayload(logicPayload))
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return rpcIX, nil
}

func GetRPCTesseractPartsFromGrid(grid map[identifiers.Address]common.TesseractHeightAndHash) RPCTesseractParts {
	if len(grid) == 0 {
		return nil
	}

	parts := make(RPCTesseractParts, 0, len(grid))

	for address, heightAndHash := range grid {
		parts = append(
			parts,
			RPCTesseractPart{
				Address: address,
				Height:  hexutil.Uint64(heightAndHash.Height),
				Hash:    heightAndHash.Hash,
			},
		)
	}

	parts.Sort()

	return parts
}

func CreateRPCTesseractGridID(tesseractGridID *common.TesseractGridID) *RPCTesseractGridID {
	if tesseractGridID == nil {
		return nil
	}

	newGrid := &RPCTesseractGridID{
		Hash: tesseractGridID.Hash,
	}

	if tesseractGridID.Parts != nil {
		newGrid.Total = hexutil.Uint64(tesseractGridID.Parts.Total)
		newGrid.Parts = GetRPCTesseractPartsFromGrid(tesseractGridID.Parts.Grid)
	}

	return newGrid
}

func CreateRPCContextLockInfos(contextLockInfos map[identifiers.Address]common.ContextLockInfo) RPCContextLockInfos {
	if len(contextLockInfos) == 0 {
		return nil
	}

	rpcContextLockInfos := make(RPCContextLockInfos, 0, len(contextLockInfos))

	for address, contextLockInfo := range contextLockInfos {
		rpcContextLockInfos = append(
			rpcContextLockInfos,
			RPCContextLockInfo{
				Address:       address,
				ContextHash:   contextLockInfo.ContextHash,
				Height:        hexutil.Uint64(contextLockInfo.Height),
				TesseractHash: contextLockInfo.TesseractHash,
			},
		)
	}

	rpcContextLockInfos.Sort()

	return rpcContextLockInfos
}

// CreateRPCHeader creates rpc header from header
func CreateRPCHeader(h common.TesseractHeader) RPCHeader {
	rpcHeader := RPCHeader{
		Address:  h.Address,
		PrevHash: h.PrevHash,

		Height:    hexutil.Uint64(h.Height),
		FuelUsed:  hexutil.Uint64(h.FuelUsed),
		FuelLimit: hexutil.Uint64(h.FuelLimit),

		BodyHash:    h.BodyHash,
		GridHash:    h.GroupHash,
		Operator:    h.Operator,
		ClusterID:   h.ClusterID,
		Timestamp:   hexutil.Uint64(h.Timestamp),
		ContextLock: CreateRPCContextLockInfos(h.ContextLock),

		Extra: RPCCommitData{
			Round:           hexutil.Uint64(h.Extra.Round),
			CommitSignature: h.Extra.CommitSignature,
			VoteSet:         h.Extra.VoteSet.String(),
			EvidenceHash:    h.Extra.EvidenceHash,
		},
	}

	rpcHeader.Extra.GridID = CreateRPCTesseractGridID(h.Extra.GridID)

	return rpcHeader
}

func CreateRPCDeltaGroups(deltaGroups map[identifiers.Address]*common.DeltaGroup) RPCDeltaGroups {
	if len(deltaGroups) == 0 {
		return nil
	}

	rpcDeltaGroups := make(RPCDeltaGroups, 0, len(deltaGroups))

	for address, deltaGroup := range deltaGroups {
		rpcDeltaGroups = append(
			rpcDeltaGroups,
			RPCDeltaGroup{
				Address:          address,
				Role:             deltaGroup.Role,
				BehaviouralNodes: deltaGroup.BehaviouralNodes,
				RandomNodes:      deltaGroup.RandomNodes,
				ReplacedNodes:    deltaGroup.ReplacedNodes,
			},
		)
	}

	rpcDeltaGroups.Sort()

	return rpcDeltaGroups
}

func CreateRPCBody(body common.TesseractBody) RPCBody {
	return RPCBody{
		StateHash:       body.StateHash,
		ContextHash:     body.ContextHash,
		InteractionHash: body.InteractionHash,
		ReceiptHash:     body.ReceiptHash,
		ContextDelta:    CreateRPCDeltaGroups(body.ContextDelta),
		ConsensusProof:  body.ConsensusProof,
	}
}

// CreateRPCTesseract creates rpc tesseract from tesseract
func CreateRPCTesseract(ts *common.Tesseract) (*RPCTesseract, error) {
	var rpcIxns []*RPCInteraction

	if ts.ClusterID() != common.GenesisIdentifier && len(ts.Interactions()) > 0 {
		rpcIxns = make([]*RPCInteraction, len(ts.Interactions()))

		parts, err := ts.Parts()
		if err != nil {
			return nil, err
		}

		for ixIndex, ixn := range ts.Interactions() {
			rpcIxns[ixIndex], err = CreateRPCInteraction(ixn, parts.Grid, ixIndex)
			if err != nil {
				return nil, err
			}
		}
	}

	return &RPCTesseract{
		Header: CreateRPCHeader(ts.Header()),
		Body:   CreateRPCBody(ts.Body()),
		Ixns:   rpcIxns,
		Seal:   ts.Seal(),
		Hash:   ts.Hash(),
	}, nil
}

func createRPCHashes(hashes common.ReceiptAccHashes) RPCHashes {
	if len(hashes) == 0 {
		return nil
	}

	rpcHashes := make(RPCHashes, 0, len(hashes))

	for addr, hash := range hashes {
		rpcHashes = append(rpcHashes, Hashes{
			Address:     addr,
			StateHash:   hash.StateHash,
			ContextHash: hash.ContextHash,
		})
	}

	rpcHashes.Sort()

	return rpcHashes
}

// CreateRPCReceipt creates rpc receipt from receipt, interaction, grid, interaction index
func CreateRPCReceipt(
	receipt *common.Receipt,
	ix *common.Interaction,
	grid map[identifiers.Address]common.TesseractHeightAndHash,
	ixIndex int,
) *RPCReceipt {
	return &RPCReceipt{
		IxType:    hexutil.Uint64(receipt.IxType),
		IxHash:    receipt.IxHash,
		Status:    receipt.Status,
		FuelUsed:  hexutil.Uint64(receipt.FuelUsed),
		Hashes:    createRPCHashes(receipt.Hashes),
		ExtraData: receipt.ExtraData,
		From:      ix.Sender(),
		To:        ix.Receiver(),
		IXIndex:   hexutil.Uint64(ixIndex),
		Parts:     GetRPCTesseractPartsFromGrid(grid),
	}
}

func UnmarshalTopic(topics []interface{}) ([][]common.Hash, error) {
	resTopics := [][]common.Hash{}
	// decode topics, either "" or ["", ""] or null
	for _, item := range topics {
		switch raw := item.(type) {
		case string:
			// ""
			if err := addTopic(&resTopics, raw); err != nil {
				return resTopics, err
			}

		case []interface{}:
			// ["", ""]
			res := []string{}

			for _, i := range raw {
				item, ok := i.(string)
				if !ok {
					return resTopics, fmt.Errorf("hash expected")
				}

				res = append(res, item)
			}

			if err := addTopic(&resTopics, res...); err != nil {
				return resTopics, err
			}

		case nil:
			// null
			if err := addTopic(&resTopics); err != nil {
				return resTopics, err
			}

		default:
			return resTopics, fmt.Errorf("failed to decode topics. Expected '' or [''] or null")
		}
	}

	return resTopics, nil
}

// addTopic adds specific topics to the log subscription topics
func addTopic(queryTopic *[][]common.Hash, set ...string) error {
	res := []common.Hash{}

	for _, i := range set {
		item := common.Hash{}
		if err := item.UnmarshalText([]byte(i)); err != nil {
			return err
		}

		res = append(res, item)
	}

	*queryTopic = append(*queryTopic, res)

	return nil
}

// GetIxParamsWithInputComputeTrust returns ixparams initialized with ixType, payload, mtq, and mode
// We initialize at least one field in input, compute, and trust.
func GetIxParamsWithInputComputeTrust(
	ixType common.IxType,
	payload json.RawMessage,
	mtq uint,
	mode uint64,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input.Type = ixType
			ix.Input.Payload = payload
			ix.Compute.Mode = mode
			ix.Trust.MTQ = mtq
		},
	}
}
