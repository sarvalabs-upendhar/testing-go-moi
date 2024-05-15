package args

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
)

var LatestTesseractHeight int64 = -1

// CreateRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the interaction type, JSON marshalling it, and storing it in the input payload.
func CreateRPCInteraction(
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantStates,
	ixIndex int,
) (*RPCInteraction, error) {
	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	rpcIX := &RPCInteraction{
		TSHash:       tsHash,
		Participants: CreateRPCParticipants(participants),

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

func CreateRPCParticipants(participants common.ParticipantStates) RPCParticipants {
	if len(participants) == 0 {
		return nil
	}

	rpcParticipants := make(RPCParticipants, 0, len(participants))

	for addr, state := range participants {
		rpcParticipants = append(rpcParticipants, RPCState{
			Address:        addr,
			Height:         hexutil.Uint64(state.Height),
			TransitiveLink: state.TransitiveLink,
			PrevContext:    state.PreviousContext,
			LatestContext:  state.LatestContext,
			ContextDelta:   state.ContextDelta,
			StateHash:      state.StateHash,
		})
	}

	rpcParticipants.Sort()

	return rpcParticipants
}

func CreateRPCPoXtData(p common.PoXtData) RPCPoXtData {
	return RPCPoXtData{
		EvidenceHash:    p.EvidenceHash,
		BinaryHash:      p.BinaryHash,
		IdentityHash:    p.IdentityHash,
		ICSHash:         p.ICSHash,
		ClusterID:       p.ClusterID.String(),
		ICSSignature:    p.ICSSignature,
		ICSVoteset:      p.ICSVoteset.String(),
		Round:           hexutil.Uint64(p.Round),
		CommitSignature: p.CommitSignature,
		BFTVoteSet:      p.BFTVoteSet.String(),
	}
}

// CreateRPCTesseract creates rpc tesseract fom tesseract
func CreateRPCTesseract(ts *common.Tesseract) (*RPCTesseract, error) {
	var (
		rpcIxns []*RPCInteraction
		err     error
	)

	if ts.ClusterID() != common.GenesisIdentifier && len(ts.Interactions()) > 0 {
		rpcIxns = make([]*RPCInteraction, len(ts.Interactions()))

		for ixIndex, ixn := range ts.Interactions() {
			// avoid sending participants as they can be found in tesseract
			rpcIxns[ixIndex], err = CreateRPCInteraction(ixn, ts.Hash(), nil, ixIndex)
			if err != nil {
				return nil, err
			}
		}
	}

	return &RPCTesseract{
		Participants:     CreateRPCParticipants(ts.Participants()),
		InteractionsHash: ts.InteractionsHash(),
		ReceiptsHash:     ts.ReceiptsHash(),
		Epoch:            (*hexutil.Big)(ts.Epoch()),
		TimeStamp:        hexutil.Uint64(ts.Timestamp()),
		Operator:         ts.Operator(),
		FuelUsed:         hexutil.Uint64(ts.FuelUsed()),
		FuelLimit:        hexutil.Uint64(ts.FuelLimit()),
		ConsensusInfo:    CreateRPCPoXtData(ts.ConsensusInfo()),
		Seal:             ts.Seal(),

		Hash: ts.Hash(),
		Ixns: rpcIxns,
	}, nil
}

// CreateRPCReceipt creates rpc receipt from receipt, interaction, ts hash, participants, interaction index
func CreateRPCReceipt(
	receipt *common.Receipt,
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantStates,
	ixIndex int,
) *RPCReceipt {
	return &RPCReceipt{
		IxType:       hexutil.Uint64(receipt.IxType),
		IxHash:       receipt.IxHash,
		Status:       receipt.Status,
		FuelUsed:     hexutil.Uint64(receipt.FuelUsed),
		ExtraData:    receipt.ExtraData,
		From:         ix.Sender(),
		To:           ix.Receiver(),
		IXIndex:      hexutil.Uint64(ixIndex),
		TSHash:       tsHash,
		Participants: CreateRPCParticipants(participants),
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
