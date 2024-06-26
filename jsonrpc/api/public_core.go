package api

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
)

type FilterManager interface {
	NewTesseractFilter(ws jsonrpc.ConnManager) string
	NewTesseractsByAccountFilter(ws jsonrpc.ConnManager, addr identifiers.Address) string
	NewLogFilter(ws jsonrpc.ConnManager, logQuery *jsonrpc.LogQuery) string
	PendingIxnsFilter(ws jsonrpc.ConnManager) string
	Uninstall(id string) bool
	GetFilterChanges(id string) (interface{}, error)
	GetLogsForQuery(query jsonrpc.LogQuery) ([]*rpcargs.RPCLog, error)
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

func (p *PublicCoreAPI) getAccMetaInfo(args *rpcargs.TesseractArgs) (*common.AccountMetaInfo, error) {
	if err := validateOptions(args.Options); err != nil {
		return nil, err
	}

	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	height, err := args.Options.Number()
	if err == nil && height == rpcargs.LatestTesseractHeight {
		return p.sm.GetAccountMetaInfo(args.Address)
	}

	return nil, nil
}

func (p *PublicCoreAPI) getStateHash(args *rpcargs.TesseractArgs) (common.Hash, error) {
	accMetaInfo, err := p.getAccMetaInfo(args)
	if err != nil {
		return common.NilHash, err
	}

	if accMetaInfo != nil {
		return accMetaInfo.StateHash, nil
	}

	ts, err := p.getTesseract(args)
	if err != nil {
		return common.NilHash, err
	}

	return ts.StateHash(args.Address), nil
}

func (p *PublicCoreAPI) getContextHash(args *rpcargs.TesseractArgs) (common.Hash, error) {
	accMetaInfo, err := p.getAccMetaInfo(args)
	if err != nil {
		return common.NilHash, err
	}

	if accMetaInfo != nil {
		return accMetaInfo.ContextHash, nil
	}

	ts, err := p.getTesseract(args)
	if err != nil {
		return common.NilHash, err
	}

	return ts.LatestContextHash(args.Address), nil
}

// Tesseract returns the rpc tesseract using given arguments
func (p *PublicCoreAPI) Tesseract(args *rpcargs.TesseractArgs) (*rpcargs.RPCTesseract, error) {
	if err := validateOptions(args.Options); err != nil {
		return nil, err
	}

	ts, err := p.getTesseract(args)
	if err != nil {
		return nil, err
	}

	return rpcargs.CreateRPCTesseract(ts)
}

// ContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) ContextInfo(args *rpcargs.ContextInfoArgs) (*rpcargs.ContextResponse, error) {
	contextHash, err := p.getContextHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	_, behaviourSet, RandomSet, err := p.sm.GetContextByHash(args.Address, contextHash)
	if err != nil {
		return nil, err
	}

	return &rpcargs.ContextResponse{
		BehaviourNodes: utils.KramaIDToString(behaviourSet),
		RandomNodes:    utils.KramaIDToString(RandomSet),
		StorageNodes:   make([]string, 0),
	}, nil
}

// Balance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) Balance(args *rpcargs.BalArgs) (*hexutil.Big, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	balance, err := p.sm.GetBalance(args.Address, args.AssetID, stateHash)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(balance), nil
}

// TDU will return the total digital utility associated with address
func (p *PublicCoreAPI) TDU(args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	object, err := p.sm.GetBalances(args.Address, stateHash)
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

func (p *PublicCoreAPI) Registry(args *rpcargs.QueryArgs) ([]rpcargs.RPCRegistry, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	registry, err := p.sm.GetRegistry(args.Address, stateHash)
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

// InteractionByTesseract returns the interaction for the given tesseract hash
func (p *PublicCoreAPI) InteractionByTesseract(args *rpcargs.InteractionByTesseract) (
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

// InteractionByHash returns the interaction for the given interaction hash
func (p *PublicCoreAPI) InteractionByHash(args *rpcargs.InteractionByHashArgs) (*rpcargs.RPCInteraction, error) {
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

// InteractionReceipt returns the receipt for the given interaction hash
func (p *PublicCoreAPI) InteractionReceipt(args *rpcargs.ReceiptArgs) (*rpcargs.RPCReceipt, error) {
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

// InteractionCount returns the number of interactions sent for the given address
func (p *PublicCoreAPI) InteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	nonce, err := p.sm.GetNonce(args.Address, stateHash)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// PendingInteractionCount returns the number of interactions sent for the given address.
// Including the pending interactions in IxPool.
func (p *PublicCoreAPI) PendingInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.Address.IsNil() {
		return nil, common.ErrEmptyAddress
	}

	interactionCount, err := p.ixpool.GetNonce(args.Address)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&interactionCount), nil
}

// AccountState returns the account state of the given address
func (p *PublicCoreAPI) AccountState(args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccount, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	account, err := p.sm.GetAccountState(args.Address, stateHash)
	if err != nil {
		return nil, err
	}

	return &rpcargs.RPCAccount{
		Nonce:          hexutil.Uint64(account.Nonce),
		AccType:        account.AccType,
		Balance:        account.Balance,
		AssetRegistry:  account.AssetRegistry,
		AssetApprovals: account.AssetApprovals,
		ContextHash:    account.ContextHash,
		StorageRoot:    account.StorageRoot,
		LogicRoot:      account.LogicRoot,
		FileRoot:       account.FileRoot,
	}, nil
}

func (p *PublicCoreAPI) LogicEnlisted(args *rpcargs.LogicEnlistedArgs) (bool, error) {
	obj, err := p.sm.GetLatestStateObject(args.Address)
	if err != nil {
		return false, err
	}

	return obj.HasStorageTree(args.LogicID)
}

// LogicManifest returns the manifest associated with the given logic id
func (p *PublicCoreAPI) LogicManifest(args *rpcargs.LogicManifestArgs) (hexutil.Bytes, error) {
	if args.LogicID == "" {
		return nil, common.ErrEmptyLogicID
	}

	stateHash, err := p.getStateHash(getTesseractArgs(args.LogicID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	logicManifest, err := p.sm.GetLogicManifest(args.LogicID, stateHash)
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

// LogicStorage returns the data associated with the given storage slot
func (p *PublicCoreAPI) LogicStorage(args *rpcargs.GetLogicStorageArgs) (hexutil.Bytes, error) {
	if args.LogicID == "" {
		return nil, common.ErrEmptyLogicID
	}

	address := args.Address
	if args.Address.IsNil() {
		address = args.LogicID.Address()
	}

	stateHash, err := p.getStateHash(getTesseractArgs(address, args.Options))
	if err != nil {
		return nil, err
	}

	if args.Address.IsNil() {
		return p.sm.GetPersistentStorageEntry(args.LogicID, args.StorageKey, stateHash)
	}

	return p.sm.GetEphemeralStorageEntry(args.Address, args.LogicID, args.StorageKey, stateHash)
}

// LogicIDs will fetch the logic IDs from the logic tree
func (p *PublicCoreAPI) LogicIDs(args *rpcargs.GetAccountArgs) ([]identifiers.LogicID, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetLogicIDs(args.Address, stateHash)
}

// AssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) AssetInfoByAssetID(args *rpcargs.GetAssetInfoArgs) (*rpcargs.RPCAssetDescriptor, error) {
	if args.AssetID == "" {
		return nil, common.ErrEmptyAssetID
	}

	stateHash, err := p.getStateHash(getTesseractArgs(args.AssetID.Address(), args.Options))
	if err != nil {
		return nil, err
	}

	info, err := p.sm.GetAssetInfo(args.AssetID, stateHash)
	if err != nil {
		return nil, err
	}

	return &rpcargs.RPCAssetDescriptor{
		Symbol:     info.Symbol,
		Operator:   info.Operator,
		Supply:     *(*hexutil.Big)(info.Supply),
		Standard:   hexutil.Uint16(info.Standard),
		Dimension:  hexutil.Uint8(info.Dimension),
		IsLogical:  info.IsLogical,
		IsStateFul: info.IsStateFul,
		LogicID:    info.LogicID,
	}, nil
}

// AccountMetaInfo returns the account meta info associated with the given address
func (p *PublicCoreAPI) AccountMetaInfo(args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccountMetaInfo, error) {
	if args.Address.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	accMetaInfo, err := p.sm.GetAccountMetaInfo(args.Address)
	if err != nil {
		return nil, err
	}

	return &rpcargs.RPCAccountMetaInfo{
		Type:          accMetaInfo.Type,
		Address:       accMetaInfo.Address,
		Height:        hexutil.Uint64(accMetaInfo.Height),
		TesseractHash: accMetaInfo.TesseractHash,
	}, nil
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

	transition, err := p.sm.FetchIxStateObjects(common.Interactions{ix}, stateHashes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch transition objects")
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, transition)
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

	transition, err := p.sm.FetchIxStateObjects(common.Interactions{ix}, stateHashes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch transition objects")
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, transition)
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
		stateHash, err := p.getStateHash(getTesseractArgs(addr, *value))
		if err != nil {
			return nil, err
		}

		stateHashes[addr] = stateHash
	}

	return stateHashes, nil
}

// NewTesseractFilter subscribes to all new tesseract events
func (p *PublicCoreAPI) NewTesseractFilter() (*rpcargs.FilterResponse, error) {
	id := p.filterManager.NewTesseractFilter(nil)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}, nil
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
func (p *PublicCoreAPI) NewLogFilter(query *jsonrpc.LogQuery) (*rpcargs.FilterResponse, error) {
	id := p.filterManager.NewLogFilter(nil, query)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}, nil
}

// PendingIxnsFilter subscribes to all new pending interactions.
func (p *PublicCoreAPI) PendingIxnsFilter() (*rpcargs.FilterResponse, error) {
	id := p.filterManager.PendingIxnsFilter(nil)

	return &rpcargs.FilterResponse{
		FilterID: id,
	}, nil
}

// RemoveFilter uninstalls a filter for given filter ID.
func (p *PublicCoreAPI) RemoveFilter(
	args *rpcargs.FilterArgs,
) (*rpcargs.FilterUninstallResponse, error) {
	status := p.filterManager.Uninstall(args.FilterID)

	return &rpcargs.FilterUninstallResponse{
		Status: status,
	}, nil
}

// GetFilterChanges is a polling method for a filter using a filter ID,
// which returns an array of events which occurred since last poll.
func (p *PublicCoreAPI) GetFilterChanges(args *rpcargs.FilterArgs) (interface{}, error) {
	return p.filterManager.GetFilterChanges(args.FilterID)
}

// GetLogs returns an array of logs matching the LogQuery
func (p *PublicCoreAPI) GetLogs(query *rpcargs.FilterQueryArgs) ([]*rpcargs.RPCLog, error) {
	filterQuery := jsonrpc.LogQuery{
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
