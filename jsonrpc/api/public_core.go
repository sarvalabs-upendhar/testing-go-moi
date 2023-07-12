package api

import (
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// PublicCoreAPI is a struct that represents a wrapper for the core public core APIs
type PublicCoreAPI struct {
	// Represents the API backend
	ixpool IxPool
	chain  ChainManager
	exec   ExecutionManager
	sm     StateManager
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(ixpool IxPool, chain ChainManager, sm StateManager, exec ExecutionManager) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{ixpool, chain, exec, sm}
}

func getTesseractArgs(address common.Address, options rpcargs.TesseractNumberOrHash) *rpcargs.TesseractArgs {
	return &rpcargs.TesseractArgs{
		Address: address,
		Options: options,
	}
}

// getTesseractByHash returns the tesseract based on the hash given
func (p *PublicCoreAPI) getTesseractByHash(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	return p.chain.GetTesseract(hash, withInteractions)
}

func (p *PublicCoreAPI) getTesseractHashByHeight(address common.Address, height int64) (common.Hash, error) {
	if address.IsNil() {
		return common.NilHash, common.ErrInvalidAddress
	}

	if height == rpcargs.LatestTesseractHeight {
		accMetaInfo, err := p.sm.GetAccountMetaInfo(address)
		if err != nil {
			return common.NilHash, err
		}

		return accMetaInfo.TesseractHash, nil
	}

	return p.chain.GetTesseractHeightEntry(address, uint64(height))
}

// getTesseract returns tesseract using arguments.
func (p *PublicCoreAPI) getTesseract(args *rpcargs.TesseractArgs) (*common.Tesseract, error) {
	if err := validateOptions(args.Options); err != nil {
		return nil, err
	}

	if hash, ok := args.Options.Hash(); ok {
		return p.getTesseractByHash(hash, args.WithInteractions)
	}

	height, err := args.Options.Number()
	if err == nil {
		hash, err := p.getTesseractHashByHeight(args.Address, height)
		if err != nil {
			return nil, err
		}

		return p.getTesseractByHash(hash, args.WithInteractions)
	}

	if errors.Is(err, common.ErrEmptyHeight) {
		return nil, common.ErrEmptyOptions
	}

	return nil, errors.Wrap(err, "invalid options")
}

// GetRPCTesseract returns the rpc tesseract using given arguments
func (p *PublicCoreAPI) GetRPCTesseract(args *rpcargs.TesseractArgs) (*rpcargs.RPCTesseract, error) {
	ts, err := p.getTesseract(args)
	if err != nil {
		return nil, err
	}

	return CreateRPCTesseract(ts)
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(args *rpcargs.ContextInfoArgs) ([]string, []string, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, nil, err
	}

	_, behaviourSet, RandomSet, err := p.sm.GetContextByHash(ts.Address(), ts.ContextHash())
	if err != nil {
		return nil, nil, err
	}

	return utils.KramaIDToString(behaviourSet), utils.KramaIDToString(RandomSet), nil
}

// GetBalance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) GetBalance(args *rpcargs.BalArgs) (*hexutil.Big, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	balance, err := p.sm.GetBalance(ts.Address(), args.AssetID, ts.StateHash())
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(balance), nil
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	object, err := p.sm.GetBalances(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	data, _ := object.TDU()

	tdu := make([]rpcargs.TDU, 0, len(data))

	for key, value := range data {
		tdu = append(tdu, rpcargs.TDU{
			AssetID: key,
			Amount:  (*hexutil.Big)(value),
		})
	}

	return tdu, nil
}

func (p *PublicCoreAPI) GetRegistry(args *rpcargs.QueryArgs) ([]rpcargs.RPCRegistry, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	registry, err := p.sm.GetRegistry(args.Address, ts.StateHash())
	if err != nil {
		return nil, err
	}

	entries := make([]rpcargs.RPCRegistry, 0, len(registry))

	for assetID, rawInfo := range registry {
		ad := new(common.AssetDescriptor)
		if err = ad.FromBytes(rawInfo); err != nil {
			return nil, err
		}

		entries = append(entries, rpcargs.RPCRegistry{
			AssetID:   assetID,
			AssetInfo: rpcargs.GetRPCAssetDescriptor(ad),
		})
	}

	return entries, nil
}

// GetInteractionByTesseract returns the interaction for the given tesseract hash
func (p *PublicCoreAPI) GetInteractionByTesseract(args *rpcargs.InteractionByTesseract) (
	*rpcargs.RPCInteraction,
	error,
) {
	if err := validateOptions(args.Options); err != nil {
		return nil, err
	}

	if args.IxIndex == nil {
		return nil, common.ErrIXIndex
	}

	getRPCIX := func(hash common.Hash) (*rpcargs.RPCInteraction, error) {
		ix, parts, err := p.chain.GetInteractionAndPartsByTSHash(hash, int(args.IxIndex.ToUint64()))
		if err != nil {
			return nil, errors.Wrap(err, "interaction not found")
		}

		return createRPCInteraction(ix, parts.Grid, int(args.IxIndex.ToUint64()))
	}

	if hash, ok := args.Options.Hash(); ok {
		return getRPCIX(hash)
	}

	height, err := args.Options.Number()
	if err == nil {
		hash, err := p.getTesseractHashByHeight(args.Address, height)
		if err != nil {
			return nil, errors.Wrap(err, "tesseract hash not found for given address and height")
		}

		return getRPCIX(hash)
	}

	if errors.Is(err, common.ErrEmptyHeight) {
		return nil, common.ErrEmptyOptions
	}

	return nil, errors.Wrap(err, "invalid options")
}

// GetInteractionByHash returns the interaction for the given interaction hash
func (p *PublicCoreAPI) GetInteractionByHash(args *rpcargs.InteractionByHashArgs) (*rpcargs.RPCInteraction, error) {
	if args.Hash.IsNil() {
		return nil, common.ErrInvalidHash
	}

	ix, parts, ixIndex, err := p.chain.GetInteractionAndPartsByIxHash(args.Hash)
	if err != nil && errors.Is(err, common.ErrGridHashNotFound) {
		if pendingIX, found := p.ixpool.GetPendingIx(args.Hash); found {
			return createRPCInteraction(pendingIX, nil, 0)
		}

		return nil, common.ErrFetchingInteraction
	}

	if err != nil {
		return nil, err
	}

	return createRPCInteraction(ix, parts.Grid, ixIndex)
}

// GetInteractionReceipt returns the receipt for the given interaction hash
func (p *PublicCoreAPI) GetInteractionReceipt(args *rpcargs.ReceiptArgs) (*rpcargs.RPCReceipt, error) {
	if args.Hash.IsNil() {
		return nil, common.ErrInvalidHash
	}

	receipt, err := p.chain.GetReceiptByIxHash(args.Hash)
	if err != nil {
		return nil, err
	}

	ix, parts, ixIndex, err := p.chain.GetInteractionAndPartsByIxHash(args.Hash)
	if err != nil {
		return nil, err
	}

	return createRPCReceipt(receipt, ix, parts.Grid, ixIndex), nil
}

// GetInteractionCount returns the number of interactions sent for the given address
func (p *PublicCoreAPI) GetInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	nonce, err := p.sm.GetNonce(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// GetPendingInteractionCount returns the number of interactions sent for the given address.
// Including the pending interactions in IxPool.
func (p *PublicCoreAPI) GetPendingInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.Address.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	interactionCount, err := p.ixpool.GetNonce(args.Address)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&interactionCount), nil
}

// GetAccountState returns the account state of the given address
func (p *PublicCoreAPI) GetAccountState(args *rpcargs.GetAccountArgs) (map[string]interface{}, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	account, err := p.sm.GetAccountState(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	rpcAccount := map[string]interface{}{
		"nonce":           hexutil.Uint64(account.Nonce),
		"acc_type":        account.AccType,
		"balance":         account.Balance,
		"asset_registry":  account.AssetRegistry,
		"asset_approvals": account.AssetApprovals,
		"context_hash":    account.ContextHash,
		"storage_root":    account.StorageRoot,
		"logic_root":      account.LogicRoot,
		"file_root":       account.FileRoot,
	}

	return rpcAccount, nil
}

// GetLogicManifest returns the manifest associated with the given logic id
func (p *PublicCoreAPI) GetLogicManifest(args *rpcargs.LogicManifestArgs) (hexutil.Bytes, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.LogicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	logicManifest, err := p.sm.GetLogicManifest(args.LogicID, ts.StateHash())
	if err != nil {
		return nil, err
	}

	switch args.Encoding {
	case "POLO", "":
		return logicManifest, nil
	case "JSON":
		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(engineio.JSON)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	case "YAML":
		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(engineio.YAML)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	default:
		return nil, errors.New("invalid encoding type")
	}
}

// GetStorageAt returns the data associated with the given storage slot
func (p *PublicCoreAPI) GetStorageAt(args *rpcargs.GetStorageArgs) (hexutil.Bytes, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.LogicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetStorageEntry(args.LogicID, args.StorageKey, ts.StateHash())
}

// GetLogicIDs will fetch the logic IDs from the logic tree
func (p *PublicCoreAPI) GetLogicIDs(args *rpcargs.GetAccountArgs) ([]common.LogicID, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	logicIDs, err := p.sm.GetLogicIDs(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	return logicIDs, nil
}

// GetAssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) GetAssetInfoByAssetID(args *rpcargs.GetAssetInfoArgs) (map[string]interface{}, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.AssetID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	info, err := p.sm.GetAssetInfo(args.AssetID, ts.StateHash())
	if err != nil {
		return nil, err
	}

	rpcAssetInfo := map[string]interface{}{
		"symbol":      info.Symbol,
		"operator":    info.Operator,
		"supply":      (*hexutil.Big)(info.Supply),
		"standard":    hexutil.Uint16(info.Standard),
		"dimension":   hexutil.Uint8(info.Dimension),
		"is_logical":  info.IsLogical,
		"is_stateful": info.IsStateFul,
	}

	if info.LogicID.String() != "" {
		rpcAssetInfo["logic_id"] = info.LogicID
	}

	return rpcAssetInfo, nil
}

// AccountMetaInfo returns the account meta info associated with the given address
func (p *PublicCoreAPI) AccountMetaInfo(args *rpcargs.GetAccountArgs) (map[string]interface{}, error) {
	if args.Address.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	accMetaInfo, err := p.sm.GetAccountMetaInfo(args.Address)
	if err != nil {
		return nil, err
	}

	rpcAccMetaInfo := map[string]interface{}{
		"type":           accMetaInfo.Type,
		"address":        accMetaInfo.Address,
		"height":         hexutil.Uint64(accMetaInfo.Height),
		"tesseract_hash": accMetaInfo.TesseractHash,
		"lattice_exists": accMetaInfo.LatticeExists,
		"state_exists":   accMetaInfo.StateExists,
	}

	return rpcAccMetaInfo, nil
}

// LogicCall supports call to logics that do not transition state
func (p *PublicCoreAPI) LogicCall(args *rpcargs.LogicCallArgs) (*rpcargs.LogicCallResult, error) {
	consumed, receipt, err := p.exec.LogicCall(args.LogicID, args.Invoker, args.Callsite, args.Calldata)
	if err != nil {
		return nil, err
	}

	logicCallResult := &rpcargs.LogicCallResult{
		Consumed: (hexutil.Big)(*consumed),
		Outputs:  receipt.Outputs,
		Error:    receipt.Error,
	}

	return logicCallResult, nil
}

// createRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the interaction type, JSON marshalling it, and storing it in the input payload.
func createRPCInteraction(
	ix *common.Interaction,
	grid map[common.Address]common.TesseractHeightAndHash,
	ixIndex int,
) (*rpcargs.RPCInteraction, error) {
	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	rpcIX := &rpcargs.RPCInteraction{
		Parts:   getRPCTesseractPartsFromGrid(grid),
		IxIndex: hexutil.Uint64(ixIndex),
		Type:    input.Type,
		Nonce:   hexutil.Uint64(input.Nonce),

		Sender:   input.Sender,
		Receiver: input.Receiver,
		Payer:    input.Payer,

		FuelPrice: (*hexutil.Big)(input.FuelPrice),
		FuelLimit: (*hexutil.Big)(input.FuelLimit),

		Mode:         hexutil.Uint64(compute.Mode),
		ComputeHash:  compute.Hash,
		ComputeNodes: compute.ComputeNodes,

		MTQ:        hexutil.Uint64(trust.MTQ),
		TrustNodes: trust.TrustNodes,

		Hash:      ix.Hash(),
		Signature: ix.Signature(),
	}

	if len(input.TransferValues) > 0 {
		rpcIX.TransferValues = make(map[common.AssetID]*hexutil.Big)
		for asset, amount := range input.TransferValues {
			rpcIX.TransferValues[asset] = (*hexutil.Big)(amount)
		}
	}

	if len(input.PerceivedValues) > 0 {
		rpcIX.PerceivedValues = make(map[common.AssetID]*hexutil.Big)
		for asset, amount := range input.PerceivedValues {
			rpcIX.PerceivedValues[asset] = (*hexutil.Big)(amount)
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

		rpcPayload := rpcargs.RPCAssetMintOrBurn{
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

		rpcAssetPayload := rpcargs.RPCAssetCreation{
			Symbol:    assetPayload.Symbol,
			Supply:    (*hexutil.Big)(assetPayload.Supply),
			Dimension: (*hexutil.Uint8)(&assetPayload.Dimension),
			Standard:  (*hexutil.Uint16)(&assetPayload.Standard),

			IsLogical:  assetPayload.IsLogical,
			IsStateful: assetPayload.IsStateFul,
			Logic:      rpcargs.RPClogicPayloadFromLogicPayload(assetPayload.LogicPayload),
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

		rpcIX.Payload, err = json.Marshal(rpcargs.RPClogicPayloadFromLogicPayload(logicPayload))
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return rpcIX, nil
}

func getRPCTesseractPartsFromGrid(grid map[common.Address]common.TesseractHeightAndHash) rpcargs.RPCTesseractParts {
	if len(grid) == 0 {
		return nil
	}

	parts := make(rpcargs.RPCTesseractParts, 0, len(grid))

	for address, heightAndHash := range grid {
		parts = append(
			parts,
			rpcargs.RPCTesseractPart{
				Address: address,
				Height:  hexutil.Uint64(heightAndHash.Height),
				Hash:    heightAndHash.Hash,
			},
		)
	}

	parts.Sort()

	return parts
}

func createRPCTesseractGridID(tesseractGridID *common.TesseractGridID) *rpcargs.RPCTesseractGridID {
	if tesseractGridID == nil {
		return nil
	}

	newGrid := &rpcargs.RPCTesseractGridID{
		Hash: tesseractGridID.Hash,
	}

	if tesseractGridID.Parts != nil {
		newGrid.Total = hexutil.Uint64(tesseractGridID.Parts.Total)
		newGrid.Parts = getRPCTesseractPartsFromGrid(tesseractGridID.Parts.Grid)
	}

	return newGrid
}

func createRPCContextLockInfos(contextLockInfos map[common.Address]common.ContextLockInfo) rpcargs.RPCContextLockInfos {
	if len(contextLockInfos) == 0 {
		return nil
	}

	rpcContextLockInfos := make(rpcargs.RPCContextLockInfos, 0, len(contextLockInfos))

	for address, contextLockInfo := range contextLockInfos {
		rpcContextLockInfos = append(
			rpcContextLockInfos,
			rpcargs.RPCContextLockInfo{
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

// createRPCHeader creates rpc header from header
func createRPCHeader(h common.TesseractHeader) rpcargs.RPCHeader {
	rpcHeader := rpcargs.RPCHeader{
		Address:  h.Address,
		PrevHash: h.PrevHash,
		Height:   hexutil.Uint64(h.Height),
		FuelUsed: func() hexutil.Big {
			if h.FuelUsed == nil {
				return hexutil.Big(*big.NewInt(0))
			}

			return hexutil.Big(*h.FuelUsed)
		}(),
		FuelLimit: func() hexutil.Big {
			if h.FuelLimit == nil {
				return hexutil.Big(*big.NewInt(0))
			}

			return hexutil.Big(*h.FuelLimit)
		}(),
		BodyHash:    h.BodyHash,
		GridHash:    h.GroupHash,
		Operator:    h.Operator,
		ClusterID:   h.ClusterID,
		Timestamp:   hexutil.Uint64(h.Timestamp),
		ContextLock: createRPCContextLockInfos(h.ContextLock),
		Extra: rpcargs.RPCCommitData{
			Round:           hexutil.Uint64(h.Extra.Round),
			CommitSignature: h.Extra.CommitSignature,
			VoteSet:         h.Extra.VoteSet.String(),
			EvidenceHash:    h.Extra.EvidenceHash,
		},
	}

	rpcHeader.Extra.GridID = createRPCTesseractGridID(h.Extra.GridID)

	return rpcHeader
}

func createRPCDeltaGroups(deltaGroups map[common.Address]*common.DeltaGroup) rpcargs.RPCDeltaGroups {
	if len(deltaGroups) == 0 {
		return nil
	}

	rpcDeltaGroups := make(rpcargs.RPCDeltaGroups, 0, len(deltaGroups))

	for address, deltaGroup := range deltaGroups {
		rpcDeltaGroups = append(
			rpcDeltaGroups,
			rpcargs.RPCDeltaGroup{
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

func createRPCBody(body common.TesseractBody) rpcargs.RPCBody {
	return rpcargs.RPCBody{
		StateHash:       body.StateHash,
		ContextHash:     body.ContextHash,
		InteractionHash: body.InteractionHash,
		ReceiptHash:     body.ReceiptHash,
		ContextDelta:    createRPCDeltaGroups(body.ContextDelta),
		ConsensusProof:  body.ConsensusProof,
	}
}

// CreateRPCTesseract creates rpc tesseract from tesseract
func CreateRPCTesseract(ts *common.Tesseract) (*rpcargs.RPCTesseract, error) {
	var rpcIxns []*rpcargs.RPCInteraction

	if ts.ClusterID() != common.GenesisIdentifier && len(ts.Interactions()) > 0 {
		rpcIxns = make([]*rpcargs.RPCInteraction, len(ts.Interactions()))

		parts, err := ts.Parts()
		if err != nil {
			return nil, err
		}

		for ixIndex, ixn := range ts.Interactions() {
			rpcIxns[ixIndex], err = createRPCInteraction(ixn, parts.Grid, ixIndex)
			if err != nil {
				return nil, err
			}
		}
	}

	return &rpcargs.RPCTesseract{
		Header: createRPCHeader(ts.Header()),
		Body:   createRPCBody(ts.Body()),
		Ixns:   rpcIxns,
		Seal:   ts.Seal(),
		Hash:   ts.Hash(),
	}, nil
}

func createRPCHashes(hashes common.ReceiptAccHashes) rpcargs.RPCHashes {
	if len(hashes) == 0 {
		return nil
	}

	rpcHashes := make(rpcargs.RPCHashes, 0, len(hashes))

	for addr, hash := range hashes {
		rpcHashes = append(rpcHashes, rpcargs.Hashes{
			Address:     addr,
			StateHash:   hash.StateHash,
			ContextHash: hash.ContextHash,
		})
	}

	rpcHashes.Sort()

	return rpcHashes
}

// createRPCReceipt creates rpc receipt from receipt, interaction, grid, interaction index
func createRPCReceipt(
	receipt *common.Receipt,
	ix *common.Interaction,
	grid map[common.Address]common.TesseractHeightAndHash,
	ixIndex int,
) *rpcargs.RPCReceipt {
	return &rpcargs.RPCReceipt{
		IxType:    hexutil.Uint64(receipt.IxType),
		IxHash:    receipt.IxHash,
		Status:    receipt.Status,
		FuelUsed:  hexutil.Big(*receipt.FuelUsed),
		Hashes:    createRPCHashes(receipt.Hashes),
		ExtraData: receipt.ExtraData,
		From:      ix.Sender(),
		To:        ix.Receiver(),
		IXIndex:   hexutil.Uint64(ixIndex),
		Parts:     getRPCTesseractPartsFromGrid(grid),
	}
}

func validateOptions(options rpcargs.TesseractNumberOrHash) error {
	if options.TesseractHash != nil && options.TesseractNumber != nil {
		return errors.New("can not use both tesseract number and tesseract hash")
	}

	return nil
}
