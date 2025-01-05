package args

import (
	"encoding/json"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
)

const (
	defaultTesseractRangeLimit = 10
	defaultBatchLengthLimit    = 20
)

var (
	LatestTesseractHeight int64 = -1
	MockJSONRPCConfig           = config.JSONRPCConfig{
		TesseractRangeLimit: defaultTesseractRangeLimit,
		BatchLengthLimit:    defaultBatchLengthLimit,
	}
)

// CreateRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the operation type, JSON marshalling it, and storing it in the input payload.
func CreateRPCInteraction(
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantsState,
	ixIndex int,
) (*RPCInteraction, error) {
	data := ix.IXData()

	ops := make([]RPCIxOp, len(ix.IXData().IxOps))

	for idx, op := range ix.Ops() {
		var rawPayload []byte

		switch op.Type() {
		case common.IxParticipantCreate:
			payload, err := op.GetParticipantCreatePayload()
			if err != nil {
				return nil, err
			}

			rpcPayload := RPCParticipantCreate{
				Address: payload.Address,
				Amount:  (*hexutil.Big)(payload.Amount),
			}

			rawPayload, err = json.Marshal(rpcPayload)
			if err != nil {
				return nil, err
			}

		case common.IxAssetCreate:
			payload, err := op.GetAssetCreatePayload()
			if err != nil {
				return nil, err
			}

			rpcPayload := RPCAssetCreation{
				Symbol:    payload.Symbol,
				Supply:    (*hexutil.Big)(payload.Supply),
				Dimension: (*hexutil.Uint8)(&payload.Dimension),
				Standard:  (*hexutil.Uint16)(&payload.Standard),

				IsLogical:  payload.IsLogical,
				IsStateful: payload.IsStateFul,
				Logic:      RPCLogicPayloadFromLogicPayload(payload.LogicPayload),
			}

			rawPayload, err = json.Marshal(rpcPayload)
			if err != nil {
				return nil, err
			}
		case common.IxAssetTransfer, common.IxAssetApprove, common.IxAssetRevoke,
			common.IxAssetLockup, common.IxAssetRelease:
			payload, err := op.GetAssetActionPayload()
			if err != nil {
				return nil, err
			}

			rpcPayload := RPCAssetAction{
				Benefactor:  payload.Benefactor,
				Beneficiary: payload.Beneficiary,
				AssetID:     payload.AssetID,
				Amount:      (*hexutil.Big)(payload.Amount),
				Timestamp:   (*hexutil.Uint64)(&payload.Timestamp),
			}

			rawPayload, err = json.Marshal(rpcPayload)
			if err != nil {
				return nil, err
			}
		case common.IxAssetMint, common.IxAssetBurn:
			payload, err := op.GetAssetSupplyPayload()
			if err != nil {
				return nil, err
			}

			rpcPayload := RPCAssetSupply{
				AssetID: payload.AssetID,
				Amount:  (*hexutil.Big)(payload.Amount),
			}

			rawPayload, err = json.Marshal(rpcPayload)
			if err != nil {
				return nil, err
			}
		case common.IxLogicDeploy, common.IxLogicEnlist, common.IxLogicInvoke:
			payload, err := op.GetLogicPayload()
			if err != nil {
				return nil, err
			}

			rawPayload, err = json.Marshal(RPCLogicPayloadFromLogicPayload(payload))
			if err != nil {
				return nil, err
			}
		default:
			return nil, common.ErrInvalidInteractionType
		}

		ops[idx] = RPCIxOp{
			Type:    op.Type(),
			Payload: rawPayload,
		}
	}

	return &RPCInteraction{
		TSHash: tsHash,

		ParticipantsState: CreateRPCParticipantStates(participants),
		IxParticipants:    CreateRPCIxParticipants(ix.IxParticipants()),

		IxIndex: hexutil.Uint64(ixIndex),
		Nonce:   hexutil.Uint64(data.Nonce),

		Sender: data.Sender,
		Payer:  data.Payer,

		FuelPrice: (*hexutil.Big)(data.FuelPrice),
		FuelLimit: hexutil.Uint64(data.FuelLimit),
		Funds:     CreateRPCIxFunds(ix.Funds()),

		IxOps: ops,

		Hash:      ix.Hash(),
		Signature: ix.Signature(),
	}, nil
}

func CreateRPCPeersScore(peers map[peer.ID]*pubsub.PeerScoreSnapshot) RPCPeersScore {
	peersScore := make(RPCPeersScore, 0, len(peers))

	for id, score := range peers {
		rpcTopicScores := make(RPCTopicScores, 0, len(score.Topics))

		for name, s := range score.Topics {
			rpcTopicScores = append(rpcTopicScores, RPCTopicScore{
				Name:                     name,
				TimeInMesh:               uint64(s.TimeInMesh.Milliseconds()),
				FirstMessageDeliveries:   s.FirstMessageDeliveries,
				MeshMessageDeliveries:    s.MeshMessageDeliveries,
				InvalidMessageDeliveries: s.InvalidMessageDeliveries,
			})
		}

		rpcTopicScores.Sort()

		peersScore = append(peersScore, RPCPeerScore{
			ID:                 id,
			TopicScores:        rpcTopicScores,
			AppSpecificScore:   score.AppSpecificScore,
			GossipScore:        score.Score,
			IPColocationFactor: score.IPColocationFactor,
			BehaviourPenalty:   score.BehaviourPenalty,
		})
	}

	peersScore.Sort()

	return peersScore
}

func CreateRPCIxFunds(funds []common.IxFund) []RPCIxFund {
	rpcFunds := make([]RPCIxFund, len(funds))
	for index, v := range funds {
		rpcFunds[index] = RPCIxFund{
			AssetID: v.AssetID,
			Amount:  (*hexutil.Big)(v.Amount),
		}
	}

	return rpcFunds
}

func CreateRPCIxParticipants(ixParticipants []common.IxParticipant) RPCIxParticipants {
	rpcIxParticipants := make(RPCIxParticipants, len(ixParticipants))
	for i, participant := range ixParticipants {
		rpcIxParticipants[i] = RPCIxParticipant{
			Address:  participant.Address,
			LockType: participant.LockType,
		}
	}

	return rpcIxParticipants
}

func CreateRPCParticipantStates(participants common.ParticipantsState) RPCParticipantsStates {
	if len(participants) == 0 {
		return nil
	}

	rpcParticipants := make(RPCParticipantsStates, 0, len(participants))

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

// TODO: Update more fields here
func CreateRPCPoXtData(p common.PoXtData) RPCPoXtData {
	return RPCPoXtData{
		Proposer:     p.Proposer,
		EvidenceHash: p.EvidenceHash,
		BinaryHash:   p.BinaryHash,
		IdentityHash: p.IdentityHash,
		View:         hexutil.Uint64(p.View),
		LastCommit:   p.LastCommit,
		AccountLocks: p.AccountLocks,
		ICSSeed:      p.ICSSeed,
		ICSProof:     p.ICSProof,
	}
}

// CreateRPCTesseract creates rpc tesseract fom tesseract
func CreateRPCTesseract(ts *common.Tesseract) (*RPCTesseract, error) {
	var (
		rpcIxns []*RPCInteraction
		err     error
	)

	if ts.ConsensusInfo().View != common.GenesisView && len(ts.Interactions().IxList()) > 0 {
		rpcIxns = make([]*RPCInteraction, len(ts.Interactions().IxList()))

		for ixIndex, ixn := range ts.Interactions().IxList() {
			// avoid sending participants as they can be found in tesseract
			rpcIxns[ixIndex], err = CreateRPCInteraction(ixn, ts.Hash(), nil, ixIndex)
			if err != nil {
				return nil, err
			}
		}
	}

	return &RPCTesseract{
		Participants:     CreateRPCParticipantStates(ts.Participants()),
		InteractionsHash: ts.InteractionsHash(),
		ReceiptsHash:     ts.ReceiptsHash(),
		Epoch:            (*hexutil.Big)(ts.Epoch()),
		TimeStamp:        hexutil.Uint64(ts.Timestamp()),
		FuelUsed:         hexutil.Uint64(ts.FuelUsed()),
		FuelLimit:        hexutil.Uint64(ts.FuelLimit()),
		ConsensusInfo:    CreateRPCPoXtData(ts.ConsensusInfo()),
		Seal:             ts.Seal(),

		Hash:       ts.Hash(),
		Ixns:       rpcIxns,
		CommitInfo: CreateRPCCommitInfo(ts.CommitInfo()),
	}, nil
}

// CreateRPCReceipt creates rpc receipt from receipt, interaction, ts hash, participants, interaction index
func CreateRPCReceipt(
	receipt *common.Receipt,
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantsState,
	ixIndex int,
) *RPCReceipt {
	return &RPCReceipt{
		IxHash:   receipt.IxHash,
		Status:   receipt.Status,
		FuelUsed: hexutil.Uint64(receipt.FuelUsed),
		IxOps: func() []*RPCIxOpResult {
			opResults := make([]*RPCIxOpResult, len(receipt.IxOps))

			for idx, op := range receipt.IxOps {
				opResults[idx] = &RPCIxOpResult{
					TxType: hexutil.Uint64(op.IxType),
					Status: op.Status,
					Data:   op.Data,
				}
			}

			return opResults
		}(),
		From:         ix.Sender(),
		IXIndex:      hexutil.Uint64(ixIndex),
		TSHash:       tsHash,
		Participants: CreateRPCParticipantStates(participants),
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

// GetIxParamsWithIxData returns ixparams initialized with ixType, payload.
// We initialize at least one field in input, compute, and trust.
func GetIxParamsWithIxData(
	ixType common.IxOpType,
	payload json.RawMessage,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    ixType,
					Payload: payload,
				},
			}
		},
	}
}

func MockConfig() *config.Config {
	return &config.Config{
		JSONRPC: &config.JSONRPCConfig{
			TesseractRangeLimit: 10,
			BatchLengthLimit:    20,
		},
	}
}

type MockMethodData struct {
	ID   uint8
	Name string
}

type MockInputArgs struct{}

func MockRegisterValidMethod() *MockMethodData {
	return &MockMethodData{
		ID:   1,
		Name: "mockMethodData",
	}
}

// Method should have 2 return params to register service
// 1. Response
// 2. Error

// MockMethodWithResp and MockMethodWithError are used in dispatcher_test.go
func (h *MockMethodData) MockMethodWithResp(args *MockInputArgs) (*MockMethodData, error) {
	return h, nil
}

func (h *MockMethodData) MockMethodWithError() (*MockMethodData, error) {
	return nil, errors.New("mock error")
}

type MockInvalidMethodData struct {
	ID   uint8
	Name string
}

func MockRegisterInvalidMethod() *MockInvalidMethodData {
	return &MockInvalidMethodData{
		ID:   1,
		Name: "mockMethodData",
	}
}

func (h *MockInvalidMethodData) MockMethodWithOnlyResp() *MockInvalidMethodData {
	return h
}

func (h *MockInvalidMethodData) MockMethodWithNoError() (*MockInvalidMethodData, *MockInvalidMethodData) {
	return h, h
}
