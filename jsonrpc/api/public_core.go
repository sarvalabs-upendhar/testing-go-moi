package api

import (
	"math/big"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/jsonrpc/websocket"
)

type FilterManager interface {
	NewTesseractFilter(ws websocket.ConnManager) string
	NewTesseractsByAccountFilter(ws websocket.ConnManager, addr identifiers.Address) string
	NewLogFilter(ws websocket.ConnManager, logQuery *websocket.LogQuery) string
	PendingIxnsFilter(ws websocket.ConnManager) string
	Uninstall(id string) bool
	GetFilterChanges(id string) (interface{}, error)
	GetLogsForQuery(query websocket.LogQuery) ([]*rpcargs.RPCLog, error)
}

// PublicCoreAPI is a struct that represents a wrapper for the core public core APIs
type PublicCoreAPI struct {
	// Represents the API backend
	ixpool        backend.IxPool
	chain         backend.ChainManager
	sm            backend.StateManager
	exec          backend.ExecutionManager
	syncer        backend.Syncer
	filterManager FilterManager
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(
	ixpool backend.IxPool,
	chain backend.ChainManager,
	sm backend.StateManager,
	exec backend.ExecutionManager,
	syncer backend.Syncer,
	filterMan FilterManager,
) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{
		ixpool:        ixpool,
		chain:         chain,
		sm:            sm,
		exec:          exec,
		syncer:        syncer,
		filterManager: filterMan,
	}
}

func getTesseractArgs(address identifiers.Address, options rpcargs.TesseractNumberOrHash) *rpcargs.TesseractArgs {
	return &rpcargs.TesseractArgs{
		Address: address,
		Options: options,
	}
}

// getTesseractByHash returns the tesseract based on the hash given
func (p *PublicCoreAPI) getTesseractByHash(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
	return p.chain.GetTesseract(hash, withInteractions)
}

func (p *PublicCoreAPI) getTesseractHashByHeight(address identifiers.Address, height int64) (common.Hash, error) {
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

	return rpcargs.CreateRPCTesseract(ts)
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(args *rpcargs.ContextInfoArgs) ([]string, []string, error) {
	if args.Address.IsNil() {
		return nil, nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, nil, err
	}

	_, behaviourSet, RandomSet, err := p.sm.GetContextByHash(args.Address, ts.LatestContextHash(args.Address))
	if err != nil {
		return nil, nil, err
	}

	return utils.KramaIDToString(behaviourSet), utils.KramaIDToString(RandomSet), nil
}

// GetBalance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) GetBalance(args *rpcargs.BalArgs) (*hexutil.Big, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	balance, err := p.sm.GetBalance(args.Address, args.AssetID, ts.StateHash(args.Address))
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(balance), nil
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	object, err := p.sm.GetBalances(args.Address, ts.StateHash(args.Address))
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
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	registry, err := p.sm.GetRegistry(args.Address, ts.StateHash(args.Address))
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
			AssetID:   identifiers.AssetID(assetID).String(),
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
		ix, participants, err := p.chain.GetInteractionAndParticipantsByTSHash(hash, int(args.IxIndex.ToUint64()))
		if err != nil {
			return nil, errors.Wrap(err, "interaction not found")
		}

		return rpcargs.CreateRPCInteraction(ix, hash, participants, int(args.IxIndex.ToUint64()))
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

	ix, hash, participants, ixIndex, err := p.chain.GetInteractionAndParticipantsByIxHash(args.Hash)
	if err != nil && errors.Is(err, common.ErrTSHashNotFound) {
		if pendingIX, found := p.ixpool.GetPendingIx(args.Hash); found {
			return rpcargs.CreateRPCInteraction(pendingIX, common.NilHash, nil, 0)
		}

		return nil, common.ErrFetchingInteraction
	}

	if err != nil {
		return nil, err
	}

	return rpcargs.CreateRPCInteraction(ix, hash, participants, ixIndex)
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

	ix, hash, participants, ixIndex, err := p.chain.GetInteractionAndParticipantsByIxHash(args.Hash)
	if err != nil {
		return nil, err
	}

	return rpcargs.CreateRPCReceipt(receipt, ix, hash, participants, ixIndex), nil
}

// GetInteractionCount returns the number of interactions sent for the given address
func (p *PublicCoreAPI) GetInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	nonce, err := p.sm.GetNonce(args.Address, ts.StateHash(args.Address))
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// GetPendingInteractionCount returns the number of interactions sent for the given address.
// Including the pending interactions in IxPool.
func (p *PublicCoreAPI) GetPendingInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	interactionCount, err := p.ixpool.GetNonce(args.Address)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&interactionCount), nil
}

// GetAccountState returns the account state of the given address
func (p *PublicCoreAPI) GetAccountState(args *rpcargs.GetAccountArgs) (map[string]interface{}, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	account, err := p.sm.GetAccountState(args.Address, ts.StateHash(args.Address))
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

	logicManifest, err := p.sm.GetLogicManifest(args.LogicID, ts.StateHash(args.LogicID.Address()))
	if err != nil {
		return nil, err
	}

	switch args.Encoding {
	case "POLO", "":
		return logicManifest, nil
	case "JSON":
		depolorizedManifest, err := engineio.NewManifest(logicManifest, common.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(common.JSON)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	case "YAML":
		depolorizedManifest, err := engineio.NewManifest(logicManifest, common.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(common.YAML)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	default:
		return nil, errors.New("invalid encoding type")
	}
}

// GetLogicStorage returns the data associated with the given storage slot
func (p *PublicCoreAPI) GetLogicStorage(args *rpcargs.GetLogicStorageArgs) (hexutil.Bytes, error) {
	if args.LogicID == "" {
		return nil, common.ErrEmptyLogicID
	}

	ts, err := p.getTesseract(getTesseractArgs(args.LogicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetStorageEntry(args.LogicID, args.StorageKey, ts.StateHash(args.LogicID.Address()))
}

// GetLogicIDs will fetch the logic IDs from the logic tree
func (p *PublicCoreAPI) GetLogicIDs(args *rpcargs.GetAccountArgs) ([]identifiers.LogicID, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	logicIDs, err := p.sm.GetLogicIDs(args.Address, ts.StateHash(args.Address))
	if err != nil {
		return nil, err
	}

	return logicIDs, nil
}

// GetAssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) GetAssetInfoByAssetID(args *rpcargs.GetAssetInfoArgs) (map[string]interface{}, error) {
	if args.AssetID == "" {
		return nil, common.ErrEmptyAssetID
	}

	ts, err := p.getTesseract(getTesseractArgs(args.AssetID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	info, err := p.sm.GetAssetInfo(args.AssetID, ts.StateHash(args.AssetID.Address()))
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

	if string(info.LogicID) != "" {
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
	}

	return rpcAccMetaInfo, nil
}

// FuelEstimate returns an estimate of the fuel that is required for executing an interaction
func (p *PublicCoreAPI) FuelEstimate(args *rpcargs.CallArgs) (*hexutil.Big, error) {
	stateHashes, err := p.normalizeOptions(args.Options)
	if err != nil {
		return nil, err
	}

	sendIXArgs, err := createSendIXArgs(args.IxArgs)
	if err != nil {
		return nil, err
	}

	ix, err := constructIxn(p.sm, sendIXArgs, nil)
	if err != nil {
		return nil, err
	}

	ctx := &common.ExecutionContext{
		CtxDelta: nil,
		Cluster:  "moi.FuelEstimate",
		Time:     uint64(time.Now().Unix()),
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, stateHashes)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(new(big.Int).SetUint64(receipt.FuelUsed)), nil
}

// Syncing returns the sync status of an account if address is given else returns the node sync status
func (p *PublicCoreAPI) Syncing(args *rpcargs.SyncStatusRequest) (*rpcargs.SyncStatusResponse, error) {
	if args.Address.IsNil() {
		nodeSyncStatus := p.syncer.GetNodeSyncStatus(args.PendingAccounts)

		return &rpcargs.SyncStatusResponse{
			NodeSyncResp: nodeSyncStatus,
		}, nil
	}

	accSyncStatus, err := p.syncer.GetAccountSyncStatus(args.Address)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching account sync status")
	}

	return &rpcargs.SyncStatusResponse{
		AccSyncResp: accSyncStatus,
	}, nil
}

// Call is a method of PublicCoreAPI that is a stateless version of an interaction submit
func (p *PublicCoreAPI) Call(args *rpcargs.CallArgs) (*rpcargs.RPCReceipt, error) {
	stateHashes, err := p.normalizeOptions(args.Options)
	if err != nil {
		return nil, err
	}

	sendIXArgs, err := createSendIXArgs(args.IxArgs)
	if err != nil {
		return nil, err
	}

	ix, err := constructIxn(p.sm, sendIXArgs, nil)
	if err != nil {
		return nil, err
	}

	ctx := &common.ExecutionContext{
		CtxDelta: nil,
		Cluster:  "moi.Call",
		Time:     uint64(time.Now().Unix()),
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, stateHashes)
	if err != nil {
		return nil, err
	}

	return createCallReceipt(receipt, ix), nil
}

func (p *PublicCoreAPI) normalizeOptions(
	options map[identifiers.Address]*rpcargs.TesseractNumberOrHash,
) (map[identifiers.Address]common.Hash, error) {
	stateHashes := make(map[identifiers.Address]common.Hash)

	for addr, value := range options {
		if addr.IsNil() {
			return nil, common.ErrEmptyAddress
		}

		ts, err := p.getTesseract(getTesseractArgs(addr, *value))
		if err != nil {
			return nil, err
		}

		stateHashes[addr] = ts.StateHash(addr)
	}

	return stateHashes, nil
}

// NewTesseractFilter subscribes to all new tesseract events
func (p *PublicCoreAPI) NewTesseractFilter() *rpcargs.FilterResponse {
	id := p.filterManager.NewTesseractFilter(nil)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}
}

// NewTesseractsByAccountFilter subscribes to all new tesseract events for a given account
func (p *PublicCoreAPI) NewTesseractsByAccountFilter(
	args *rpcargs.TesseractByAccountFilterArgs,
) (*rpcargs.FilterResponse, error) {
	if args.Addr.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	id := p.filterManager.NewTesseractsByAccountFilter(nil, args.Addr)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}, nil
}

// NewLogFilter subscribes to all new tesseract log events for a given filter
func (p *PublicCoreAPI) NewLogFilter(query *websocket.LogQuery) *rpcargs.FilterResponse {
	id := p.filterManager.NewLogFilter(nil, query)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}
}

// PendingIxnsFilter subscribes to all new pending interactions.
func (p *PublicCoreAPI) PendingIxnsFilter() *rpcargs.FilterResponse {
	id := p.filterManager.PendingIxnsFilter(nil)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}
}

// RemoveFilter uninstalls a filter for given filter ID.
func (p *PublicCoreAPI) RemoveFilter(
	args *rpcargs.FilterArgs,
) *rpcargs.FilterUninstallResponse {
	status := p.filterManager.Uninstall(args.FilterID)

	return &rpcargs.FilterUninstallResponse{
		Status: status,
	}
}

// GetFilterChanges is a polling method for a filter using a filter ID,
// which returns an array of events which occurred since last poll.
func (p *PublicCoreAPI) GetFilterChanges(args *rpcargs.FilterArgs) (interface{}, error) {
	return p.filterManager.GetFilterChanges(args.FilterID)
}

// GetLogs returns an array of logs matching the LogQuery
func (p *PublicCoreAPI) GetLogs(query *rpcargs.FilterQueryArgs) ([]*rpcargs.RPCLog, error) {
	filterQuery := websocket.LogQuery{
		StartHeight: *query.StartHeight,
		EndHeight:   *query.EndHeight,
		Address:     query.Address,
		Topics:      query.Topics,
	}

	return p.filterManager.GetLogsForQuery(filterQuery)
}

func createCallReceipt(
	receipt *common.Receipt,
	ix *common.Interaction,
) *rpcargs.RPCReceipt {
	return &rpcargs.RPCReceipt{
		IxType:    hexutil.Uint64(receipt.IxType),
		IxHash:    receipt.IxHash,
		Status:    receipt.Status,
		FuelUsed:  hexutil.Uint64(receipt.FuelUsed),
		ExtraData: receipt.ExtraData,
		From:      ix.Sender(),
		To:        ix.Receiver(),
	}
}

func validateOptions(options rpcargs.TesseractNumberOrHash) error {
	if options.TesseractHash != nil && options.TesseractNumber != nil {
		return errors.New("can not use both tesseract number and tesseract hash")
	}

	return nil
}
