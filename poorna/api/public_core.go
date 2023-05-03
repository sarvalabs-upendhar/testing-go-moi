package api

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/lattice"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

// PublicCoreAPI is a struct that represents a wrapper for the core public core APIs
type PublicCoreAPI struct {
	// Represents the API backend
	ixpool IxPool
	chain  ChainManager
	sm     StateManager
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(ixpool IxPool, chain ChainManager, sm StateManager) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{ixpool, chain, sm}
}

func getTesseractArgs(address types.Address, options ptypes.TesseractNumberOrHash) *ptypes.TesseractArgs {
	return &ptypes.TesseractArgs{
		Address: address,
		Options: options,
	}
}

// getTesseractByHash returns the tesseract based on the hash given
func (p *PublicCoreAPI) getTesseractByHash(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	return p.chain.GetTesseract(hash, withInteractions)
}

func (p *PublicCoreAPI) getTesseractHashByHeight(address types.Address, height int64) (types.Hash, error) {
	if address.IsNil() {
		return types.NilHash, types.ErrInvalidAddress
	}

	if height == ptypes.LatestTesseractHeight {
		accMetaInfo, err := p.sm.GetAccountMetaInfo(address)
		if err != nil {
			return types.NilHash, err
		}

		return accMetaInfo.TesseractHash, nil
	}

	return p.chain.GetTesseractHeightEntry(address, uint64(height))
}

// getTesseract returns tesseract using arguments.
func (p *PublicCoreAPI) getTesseract(args *ptypes.TesseractArgs) (*types.Tesseract, error) {
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

	if errors.Is(err, types.ErrEmptyHeight) {
		return nil, types.ErrEmptyOptions
	}

	return nil, errors.Wrap(err, "invalid options")
}

// GetRPCTesseract returns the rpc tesseract using given arguments
func (p *PublicCoreAPI) GetRPCTesseract(args *ptypes.TesseractArgs) (*ptypes.RPCTesseract, error) {
	ts, err := p.getTesseract(args)
	if err != nil {
		return nil, err
	}

	return CreateRPCTesseract(ts)
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(args *ptypes.ContextInfoArgs) ([]string, []string, error) {
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
func (p *PublicCoreAPI) GetBalance(args *ptypes.BalArgs) (*hexutil.Big, error) {
	assetID, err := utils.ValidateAssetID(args.AssetID)
	if err != nil {
		return nil, types.ErrInvalidAssetID
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	balance, err := p.sm.GetBalance(ts.Address(), assetID, ts.StateHash())
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(balance), nil
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *ptypes.TesseractArgs) (map[types.AssetID]*hexutil.Big, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	object, err := p.sm.GetBalances(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	data, _ := object.TDU()

	rpcAssetMap := make(map[types.AssetID]*hexutil.Big)

	for key, value := range data {
		rpcAssetMap[key] = (*hexutil.Big)(value)
	}

	return rpcAssetMap, nil
}

// GetInteractionByTesseract returns the interaction for the given tesseract hash
func (p *PublicCoreAPI) GetInteractionByTesseract(args *ptypes.InteractionByTesseract) (*ptypes.RPCInteraction, error) {
	if err := validateOptions(args.Options); err != nil {
		return nil, err
	}

	getRPCIX := func(hash types.Hash, ixIndex hexutil.Uint64) (*ptypes.RPCInteraction, error) {
		ix, parts, err := p.chain.GetInteractionAndPartsByTSHash(hash, int(args.IxIndex))
		if err != nil {
			return nil, errors.Wrap(err, "interaction not found")
		}

		return createRPCInteraction(ix, parts.Grid, int(ixIndex))
	}

	if hash, ok := args.Options.Hash(); ok {
		return getRPCIX(hash, args.IxIndex)
	}

	height, err := args.Options.Number()
	if err == nil {
		hash, err := p.getTesseractHashByHeight(args.Address, height)
		if err != nil {
			return nil, errors.Wrap(err, "tesseract hash not found for given address and height")
		}

		return getRPCIX(hash, args.IxIndex)
	}

	if errors.Is(err, types.ErrEmptyHeight) {
		return nil, types.ErrEmptyOptions
	}

	return nil, errors.Wrap(err, "invalid options")
}

// GetInteractionByHash returns the interaction for the given interaction hash
func (p *PublicCoreAPI) GetInteractionByHash(args *ptypes.InteractionByHashArgs) (*ptypes.RPCInteraction, error) {
	if args.Hash.IsNil() {
		return nil, types.ErrInvalidHash
	}

	ix, parts, ixIndex, err := p.chain.GetInteractionAndPartsByIxHash(args.Hash)
	if err != nil && errors.Is(err, types.ErrGridHashNotFound) {
		if pendingIX, found := p.ixpool.GetPendingIx(args.Hash); found {
			return createRPCInteraction(pendingIX, nil, 0)
		}

		return nil, types.ErrFetchingInteraction
	}

	if err != nil {
		return nil, err
	}

	return createRPCInteraction(ix, parts.Grid, ixIndex)
}

// GetInteractionReceipt returns the receipt for the given interaction hash
func (p *PublicCoreAPI) GetInteractionReceipt(args *ptypes.ReceiptArgs) (*ptypes.RPCReceipt, error) {
	if args.Hash.IsNil() {
		return nil, types.ErrInvalidHash
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
func (p *PublicCoreAPI) GetInteractionCount(args *ptypes.InteractionCountArgs) (*hexutil.Uint64, error) {
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
func (p *PublicCoreAPI) GetPendingInteractionCount(args *ptypes.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.Address.IsNil() {
		return nil, types.ErrInvalidAddress
	}

	interactionCount, err := p.ixpool.GetNonce(args.Address)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&interactionCount), nil
}

// GetAccountState returns the account state of the given address
func (p *PublicCoreAPI) GetAccountState(args *ptypes.GetAccountArgs) (map[string]interface{}, error) {
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
		"asset_approvals": account.AssetApprovals,
		"context_hash":    account.ContextHash,
		"storage_root":    account.StorageRoot,
		"logic_root":      account.LogicRoot,
		"file_root":       account.FileRoot,
	}

	return rpcAccount, nil
}

// GetLogicManifest returns the manifest associated with the given logic id
func (p *PublicCoreAPI) GetLogicManifest(args *ptypes.LogicManifestArgs) (hexutil.Bytes, error) {
	logicID, err := utils.ValidateLogicID(args.LogicID)
	if err != nil {
		return nil, err
	}

	ts, err := p.getTesseract(getTesseractArgs(logicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	logicManifest, err := p.sm.GetLogicManifest(logicID, ts.StateHash())
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
func (p *PublicCoreAPI) GetStorageAt(args *ptypes.GetStorageArgs) (hexutil.Bytes, error) {
	logicID, err := utils.ValidateLogicID(args.LogicID)
	if err != nil {
		return nil, err
	}

	ts, err := p.getTesseract(getTesseractArgs(logicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetStorageEntry(logicID, types.FromHex(args.StorageKey), ts.StateHash())
}

// GetAssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) GetAssetInfoByAssetID(id string) (map[string]interface{}, error) {
	_, err := utils.ValidateAssetID(id)
	if err != nil {
		return nil, err
	}

	aID, err := hex.DecodeString(id)
	if err != nil {
		return nil, err
	}

	assetInfo := parseAssetMetaInfo(aID)

	assetData, err := p.chain.GetAssetDataByAssetHash(aID[2:])
	if err != nil {
		return nil, err
	}

	assetInfo.Symbol = assetData.Symbol
	assetInfo.Supply = assetData.Supply
	assetInfo.Owner = assetData.Owner
	assetInfo.LogicID = assetData.LogicID

	rpcAssetInfo := map[string]interface{}{
		"type":            assetInfo.Type,
		"symbol":          assetInfo.Symbol,
		"owner":           assetInfo.Owner,
		"supply":          (*hexutil.Big)(assetInfo.Supply),
		"dimension":       hexutil.Uint8(assetInfo.Dimension),
		"decimals":        hexutil.Uint8(assetInfo.Decimals),
		"is_fungible":     assetInfo.IsFungible,
		"is_mintable":     assetInfo.IsMintable,
		"is_transferable": assetInfo.IsTransferable,
		"logic_id":        assetInfo.LogicID,
	}

	return rpcAssetInfo, nil
}

// AccountMetaInfo returns the account meta info associated with the given address
func (p *PublicCoreAPI) AccountMetaInfo(args *ptypes.GetAccountArgs) (map[string]interface{}, error) {
	if args.Address.IsNil() {
		return nil, types.ErrInvalidAddress
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

// helper functions
func parseAssetMetaInfo(aID []byte) *types.AssetDescriptor {
	var dimension, info uint8

	assetInfo := new(types.AssetDescriptor)

	dimension = uint8(big.NewInt(0).SetBytes(aID[:1]).Uint64())
	info = uint8(big.NewInt(0).SetBytes(aID[1:2]).Uint64())

	// extract most significant bit
	if 0x80&info == 0x80 {
		assetInfo.IsFungible = true
	} else {
		assetInfo.IsFungible = false
	}

	// extract least significant bit
	if 0x01&info == 1 {
		assetInfo.IsMintable = true
	} else {
		assetInfo.IsMintable = false
	}

	assetInfo.Dimension = dimension

	return assetInfo
}

// createRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the interaction type, JSON marshalling it, and storing it in the input payload.
func createRPCInteraction(
	ix *types.Interaction,
	grid map[types.Address]types.TesseractHeightAndHash,
	ixIndex int,
) (*ptypes.RPCInteraction, error) {
	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	rpcIX := &ptypes.RPCInteraction{
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
		rpcIX.TransferValues = make(map[types.AssetID]*hexutil.Big)
		for asset, amount := range input.TransferValues {
			rpcIX.TransferValues[asset] = (*hexutil.Big)(amount)
		}
	}

	if len(input.PerceivedValues) > 0 {
		rpcIX.PerceivedValues = make(map[types.AssetID]*hexutil.Big)
		for asset, amount := range input.PerceivedValues {
			rpcIX.PerceivedValues[asset] = (*hexutil.Big)(amount)
		}
	}

	if len(input.PerceivedProofs) > 0 {
		rpcIX.PerceivedProofs = input.PerceivedProofs
	}

	var err error

	switch ix.Type() {
	case types.IxValueTransfer:
		break
	case types.IxAssetCreate:
		assetPayload := new(types.AssetPayload)
		if err = assetPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcAssetPayload := ptypes.RPCAssetCreation{
			Type:   assetPayload.Create.Type,
			Symbol: assetPayload.Create.Symbol,
			Supply: (*hexutil.Big)(assetPayload.Create.Supply),

			Dimension: hexutil.Uint8(assetPayload.Create.Dimension),
			Decimals:  hexutil.Uint8(assetPayload.Create.Decimals),

			IsFungible:     assetPayload.Create.IsFungible,
			IsMintable:     assetPayload.Create.IsMintable,
			IsTransferable: assetPayload.Create.IsTransferable,

			LogicID: types.BytesToHex(assetPayload.Create.LogicID),
		}

		rpcIX.Payload, err = json.Marshal(rpcAssetPayload)
		if err != nil {
			return nil, err
		}

	case types.IxLogicInvoke, types.IxLogicDeploy:
		logicPayload := new(types.LogicPayload)

		if err = logicPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcLogicPayload := &ptypes.RPCLogicPayload{
			Manifest: (hexutil.Bytes)(logicPayload.Manifest),
			LogicID:  types.BytesToHex(logicPayload.Logic),
			Callsite: logicPayload.Callsite,
			Calldata: (hexutil.Bytes)(logicPayload.Calldata),
		}

		rpcIX.Payload, err = json.Marshal(rpcLogicPayload)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return rpcIX, nil
}

func getRPCTesseractPartsFromGrid(grid map[types.Address]types.TesseractHeightAndHash) ptypes.RPCTesseractParts {
	if len(grid) == 0 {
		return nil
	}

	parts := make(ptypes.RPCTesseractParts, 0, len(grid))

	for address, heightAndHash := range grid {
		parts = append(
			parts,
			ptypes.RPCTesseractPart{
				Address: address,
				Height:  hexutil.Uint64(heightAndHash.Height),
				Hash:    heightAndHash.Hash,
			},
		)
	}

	parts.Sort()

	return parts
}

func createRPCTesseractGridID(tesseractGridID *types.TesseractGridID) *ptypes.RPCTesseractGridID {
	if tesseractGridID == nil {
		return nil
	}

	newGrid := &ptypes.RPCTesseractGridID{
		Hash: tesseractGridID.Hash,
	}

	if tesseractGridID.Parts != nil {
		newGrid.Total = hexutil.Uint64(tesseractGridID.Parts.Total)
		newGrid.Parts = getRPCTesseractPartsFromGrid(tesseractGridID.Parts.Grid)
	}

	return newGrid
}

func createRPCContextLockInfos(contextLockInfos map[types.Address]types.ContextLockInfo) ptypes.RPCContextLockInfos {
	if len(contextLockInfos) == 0 {
		return nil
	}

	rpcContextLockInfos := make(ptypes.RPCContextLockInfos, 0, len(contextLockInfos))

	for address, contextLockInfo := range contextLockInfos {
		rpcContextLockInfos = append(
			rpcContextLockInfos,
			ptypes.RPCContextLockInfo{
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
func createRPCHeader(h types.TesseractHeader) ptypes.RPCHeader {
	rpcHeader := ptypes.RPCHeader{
		Address:     h.Address,
		PrevHash:    h.PrevHash,
		Height:      hexutil.Uint64(h.Height),
		FuelUsed:    hexutil.Uint64(h.FuelUsed),
		FuelLimit:   hexutil.Uint64(h.FuelLimit),
		BodyHash:    h.BodyHash,
		GridHash:    h.GroupHash,
		Operator:    h.Operator,
		ClusterID:   h.ClusterID,
		Timestamp:   hexutil.Uint64(h.Timestamp),
		ContextLock: createRPCContextLockInfos(h.ContextLock),
		Extra: ptypes.RPCCommitData{
			Round:           hexutil.Uint64(h.Extra.Round),
			CommitSignature: h.Extra.CommitSignature,
			VoteSet:         h.Extra.VoteSet.String(),
			EvidenceHash:    h.Extra.EvidenceHash,
		},
	}

	rpcHeader.Extra.GridID = createRPCTesseractGridID(h.Extra.GridID)

	return rpcHeader
}

func createRPCDeltaGroups(deltaGroups map[types.Address]*types.DeltaGroup) ptypes.RPCDeltaGroups {
	if len(deltaGroups) == 0 {
		return nil
	}

	rpcDeltaGroups := make(ptypes.RPCDeltaGroups, 0, len(deltaGroups))

	for address, deltaGroup := range deltaGroups {
		rpcDeltaGroups = append(
			rpcDeltaGroups,
			ptypes.RPCDeltaGroup{
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

func createRPCBody(body types.TesseractBody) ptypes.RPCBody {
	return ptypes.RPCBody{
		StateHash:       body.StateHash,
		ContextHash:     body.ContextHash,
		InteractionHash: body.InteractionHash,
		ReceiptHash:     body.ReceiptHash,
		ContextDelta:    createRPCDeltaGroups(body.ContextDelta),
		ConsensusProof:  body.ConsensusProof,
	}
}

// CreateRPCTesseract creates rpc tesseract from tesseract
func CreateRPCTesseract(ts *types.Tesseract) (*ptypes.RPCTesseract, error) {
	var rpcIxns []*ptypes.RPCInteraction

	if ts.ClusterID() != lattice.GenesisIdentifier && len(ts.Interactions()) > 0 {
		rpcIxns = make([]*ptypes.RPCInteraction, len(ts.Interactions()))

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

	return &ptypes.RPCTesseract{
		Header: createRPCHeader(ts.Header()),
		Body:   createRPCBody(ts.Body()),
		Ixns:   rpcIxns,
		Seal:   ts.Seal(),
		Hash:   ts.Hash(),
	}, nil
}

func createRPCStateHashes(stateHashes map[types.Address]types.Hash) ptypes.RPCStateHashes {
	if len(stateHashes) == 0 {
		return nil
	}

	rpcStateHashes := make(ptypes.RPCStateHashes, 0, len(stateHashes))

	for address, hash := range stateHashes {
		rpcStateHashes = append(
			rpcStateHashes,
			ptypes.RPCStateHash{
				Address: address,
				Hash:    hash,
			},
		)
	}

	rpcStateHashes.Sort()

	return rpcStateHashes
}

func createRPCContextHashes(contextHashes map[types.Address]types.Hash) ptypes.RPCContextHashes {
	if len(contextHashes) == 0 {
		return nil
	}

	rpcContextHashes := make(ptypes.RPCContextHashes, 0, len(contextHashes))

	for address, hash := range contextHashes {
		rpcContextHashes = append(
			rpcContextHashes,
			ptypes.RPCContextHash{
				Address: address,
				Hash:    hash,
			},
		)
	}

	rpcContextHashes.Sort()

	return rpcContextHashes
}

// createRPCReceipt creates rpc receipt from receipt, interaction, grid, interaction index
func createRPCReceipt(
	receipt *types.Receipt,
	ix *types.Interaction,
	grid map[types.Address]types.TesseractHeightAndHash,
	ixIndex int,
) *ptypes.RPCReceipt {
	return &ptypes.RPCReceipt{
		IxType:        hexutil.Uint64(receipt.IxType),
		IxHash:        receipt.IxHash,
		Status:        receipt.Status,
		FuelUsed:      hexutil.Uint64(receipt.FuelUsed),
		StateHashes:   createRPCStateHashes(receipt.StateHashes),
		ContextHashes: createRPCContextHashes(receipt.ContextHashes),
		ExtraData:     receipt.ExtraData,
		From:          ix.Sender(),
		To:            ix.Receiver(),
		IXIndex:       hexutil.Uint64(ixIndex),
		Parts:         getRPCTesseractPartsFromGrid(grid),
	}
}

func validateOptions(options ptypes.TesseractNumberOrHash) error {
	if options.TesseractHash != nil && options.TesseractNumber != nil {
		return errors.New("can not use both tesseract number and tesseract hash")
	}

	return nil
}
