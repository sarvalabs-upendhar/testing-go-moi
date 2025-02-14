package api

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
)

type FilterManager interface {
	NewTesseractFilter(ws jsonrpc.ConnManager) string
	NewTesseractsByAccountFilter(ws jsonrpc.ConnManager, id identifiers.Identifier) string
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

func getTesseractArgs(id identifiers.Identifier, options rpcargs.TesseractNumberOrHash) *rpcargs.TesseractArgs {
	return &rpcargs.TesseractArgs{
		ID:      id,
		Options: options,
	}
}

// getTesseractByHash returns the tesseract based on the hash given
func (p *PublicCoreAPI) getTesseractByHash(
	hash common.Hash,
	withInteractions bool,
	withCommitInfo bool,
) (*common.Tesseract, error) {
	return p.chain.GetTesseract(hash, withInteractions, withCommitInfo)
}

func (p *PublicCoreAPI) getTesseractHashByHeight(id identifiers.Identifier, height int64) (common.Hash, error) {
	if id.IsNil() {
		return common.NilHash, common.ErrInvalidIdentifier
	}

	if height == rpcargs.LatestTesseractHeight {
		accMetaInfo, err := p.sm.GetAccountMetaInfo(id)
		if err != nil {
			return common.NilHash, err
		}

		return accMetaInfo.TesseractHash, nil
	}

	return p.chain.GetTesseractHeightEntry(id, uint64(height))
}

// getTesseract returns tesseract using arguments.
func (p *PublicCoreAPI) getTesseract(args *rpcargs.TesseractArgs) (*common.Tesseract, error) {
	if hash, ok := args.Options.Hash(); ok {
		return p.getTesseractByHash(hash, args.WithInteractions, args.WithCommitInfo)
	}

	height, err := args.Options.Number()
	if err == nil {
		hash, err := p.getTesseractHashByHeight(args.ID, height)
		if err != nil {
			return nil, err
		}

		return p.getTesseractByHash(hash, args.WithInteractions, args.WithCommitInfo)
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

	if args.ID.IsNil() {
		return nil, common.ErrEmptyID
	}

	height, err := args.Options.Number()
	if err == nil && height == rpcargs.LatestTesseractHeight {
		return p.sm.GetAccountMetaInfo(args.ID)
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

	return ts.StateHash(args.ID), nil
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

	return ts.LatestContextHash(args.ID), nil
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

// ConsensusNodes will fetch the context associated with the given ids
func (p *PublicCoreAPI) ConsensusNodes(args *rpcargs.ContextInfoArgs) (*rpcargs.ContextResponse, error) {
	contextHash, err := p.getContextHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	consensusNodes, err := p.sm.GetConsensusNodesByHash(args.ID, contextHash)
	if err != nil {
		return nil, err
	}

	return &rpcargs.ContextResponse{
		ConsensusNodes: utils.KramaIDToString(consensusNodes),
		StorageNodes:   make([]string, 0),
	}, nil
}

// Balance is a method of PublicCoreAPI for retrieving the balance of an ids.
// Accepts the ids and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) Balance(args *rpcargs.BalArgs) (*hexutil.Big, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	balance, err := p.sm.GetBalance(args.ID, args.AssetID, stateHash)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(balance), nil
}

// TDU will return the total digital utility associated with ids
func (p *PublicCoreAPI) TDU(args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	data, err := p.sm.GetBalances(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	tdu := make([]rpcargs.TDU, 0, len(data))

	for key, value := range data {
		tdu = append(tdu, rpcargs.TDU{
			AssetID: key,
			Amount:  (*hexutil.Big)(value),
		})
	}

	return tdu, nil
}

func (p *PublicCoreAPI) Deeds(args *rpcargs.QueryArgs) ([]rpcargs.RPCDeeds, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	deeds, err := p.sm.GetDeeds(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	entries := make([]rpcargs.RPCDeeds, 0, len(deeds))

	for assetID, assetInfo := range deeds {
		entries = append(entries, rpcargs.RPCDeeds{
			AssetID:   assetID.String(),
			AssetInfo: *rpcargs.GetRPCAssetDescriptor(assetInfo),
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
		hash, err := p.getTesseractHashByHeight(args.ID, height)
		if err != nil {
			return nil, errors.Wrap(err, "tesseract hash not found for given ids and height")
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

// InteractionCount returns the number of interactions sent for the given ids
func (p *PublicCoreAPI) InteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	sequenceID, err := p.sm.GetSequenceID(args.ID, args.KeyID, stateHash)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&sequenceID), nil
}

// PendingInteractionCount returns the number of interactions sent for the given ids.
// Including the pending interactions in IxPool.
func (p *PublicCoreAPI) PendingInteractionCount(args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	if args.ID.IsNil() {
		return nil, common.ErrEmptyID
	}

	interactionCount, err := p.ixpool.GetSequenceID(args.ID, args.KeyID)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&interactionCount), nil
}

// AccountState returns the account state of the given ids
func (p *PublicCoreAPI) AccountState(args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccount, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	account, err := p.sm.GetAccountState(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	return &rpcargs.RPCAccount{
		AccType:     account.AccType,
		AssetDeeds:  account.AssetDeeds,
		ContextHash: account.ContextHash,
		StorageRoot: account.StorageRoot,
		AssetRoot:   account.AssetRoot,
		LogicRoot:   account.LogicRoot,
		FileRoot:    account.FileRoot,
		KeysHash:    account.KeysHash,
	}, nil
}

func (p *PublicCoreAPI) AccountKeys(args *rpcargs.GetAccountKeysArgs) ([]*rpcargs.RPCAccountKey, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	accountKeys, err := p.sm.GetAccountKeys(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	return rpcargs.CreateRPCAccountKeys(accountKeys), nil
}

func (p *PublicCoreAPI) LogicEnlisted(args *rpcargs.LogicEnlistedArgs) (bool, error) {
	obj, err := p.sm.GetLatestStateObject(args.ID)
	if err != nil {
		return false, err
	}

	return obj.HasStorageTree(args.LogicID)
}

// LogicManifest returns the manifest associated with the given logic id
func (p *PublicCoreAPI) LogicManifest(args *rpcargs.LogicManifestArgs) (hexutil.Bytes, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.LogicID.AsIdentifier(), args.Options))
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
	id := args.ID
	if args.ID.IsNil() {
		id = args.LogicID.AsIdentifier()
	}

	stateHash, err := p.getStateHash(getTesseractArgs(id, args.Options))
	if err != nil {
		return nil, err
	}

	if args.ID.IsNil() {
		return p.sm.GetPersistentStorageEntry(args.LogicID, args.StorageKey, stateHash)
	}

	return p.sm.GetEphemeralStorageEntry(args.ID, args.LogicID, args.StorageKey, stateHash)
}

// Mandates retrieves and returns a list of asset mandates associated with the specified ids.
func (p *PublicCoreAPI) Mandates(args *rpcargs.GetAssetMandateOrLockupArgs) ([]rpcargs.RPCMandateOrLockup, error) {
	if args.ID.IsNil() {
		return nil, common.ErrEmptyID
	}

	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	mandates, err := p.sm.GetMandates(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	entries := make([]rpcargs.RPCMandateOrLockup, 0, len(mandates))

	for _, mandate := range mandates {
		entries = append(entries, rpcargs.RPCMandateOrLockup{
			AssetID: mandate.AssetID.String(),
			ID:      mandate.ID,
			Amount:  (*hexutil.Big)(mandate.Amount),
		})
	}

	return entries, nil
}

// Lockups retrieves and returns a list of asset lockups associated with the specified ids.
func (p *PublicCoreAPI) Lockups(args *rpcargs.GetAssetMandateOrLockupArgs) ([]rpcargs.RPCMandateOrLockup, error) {
	if args.ID.IsNil() {
		return nil, common.ErrEmptyID
	}

	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	lockups, err := p.sm.GetLockups(args.ID, stateHash)
	if err != nil {
		return nil, err
	}

	entries := make([]rpcargs.RPCMandateOrLockup, 0, len(lockups))

	for _, lockup := range lockups {
		entries = append(entries, rpcargs.RPCMandateOrLockup{
			AssetID: lockup.AssetID.String(),
			ID:      lockup.ID,
			Amount:  (*hexutil.Big)(lockup.Amount),
		})
	}

	return entries, nil
}

// LogicIDs will fetch the logic IDs from the logic tree
func (p *PublicCoreAPI) LogicIDs(args *rpcargs.GetAccountArgs) ([]identifiers.LogicID, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.ID, args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetLogicIDs(args.ID, stateHash)
}

// AssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) AssetInfoByAssetID(args *rpcargs.GetAssetInfoArgs) (*rpcargs.RPCAssetDescriptor, error) {
	stateHash, err := p.getStateHash(getTesseractArgs(args.AssetID.AsIdentifier(), args.Options))
	if err != nil {
		return nil, err
	}

	info, err := p.sm.GetAssetInfo(args.AssetID, stateHash)
	if err != nil {
		return nil, err
	}

	return rpcargs.GetRPCAssetDescriptor(info), nil
}

// AccountMetaInfo returns the account meta info associated with the given ids
func (p *PublicCoreAPI) AccountMetaInfo(args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccountMetaInfo, error) {
	if args.ID.IsNil() {
		return nil, common.ErrInvalidIdentifier
	}

	accMetaInfo, err := p.sm.GetAccountMetaInfo(args.ID)
	if err != nil {
		return nil, err
	}

	return &rpcargs.RPCAccountMetaInfo{
		Type:          accMetaInfo.Type,
		ID:            accMetaInfo.ID,
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

	ixData := createIxData(args.IxArgs)

	if err = validateIxData(ixData, false); err != nil {
		return nil, err
	}

	ix, err := constructIxn(p.sm, ixData)
	if err != nil {
		return nil, err
	}

	ctx := &common.ExecutionContext{
		CtxDelta: nil,
		Cluster:  "moi.FuelEstimate",
		Time:     uint64(time.Now().Unix()),
	}

	transition, err := p.sm.FetchIxStateObjects(common.NewInteractionsWithLeaderCheck(false, ix), stateHashes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch transition objects")
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, transition)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Big)(new(big.Int).SetUint64(receipt.FuelUsed)), nil
}

// Syncing returns the sync status of an account if ids is given else returns the node sync status
func (p *PublicCoreAPI) Syncing(args *rpcargs.SyncStatusRequest) (*rpcargs.SyncStatusResponse, error) {
	if args.ID.IsNil() {
		nodeSyncStatus := p.syncer.GetNodeSyncStatus(args.PendingAccounts)

		return &rpcargs.SyncStatusResponse{
			NodeSyncResp: nodeSyncStatus,
		}, nil
	}

	accSyncStatus, err := p.syncer.GetAccountSyncStatus(args.ID)
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

	ixData := createIxData(args.IxArgs)

	if err := validateIxData(ixData, false); err != nil {
		return nil, err
	}

	ix, err := constructIxn(p.sm, ixData)
	if err != nil {
		return nil, err
	}

	ctx := &common.ExecutionContext{
		CtxDelta: nil,
		Cluster:  "moi.Call",
		Time:     uint64(time.Now().Unix()),
	}

	transition, err := p.sm.FetchIxStateObjects(common.NewInteractionsWithLeaderCheck(false, ix), stateHashes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch transition objects")
	}

	receipt, err := p.exec.InteractionCall(ctx, ix, transition)
	if err != nil {
		return nil, err
	}

	return createCallReceipt(ix.SenderID(), receipt), nil
}

func (p *PublicCoreAPI) normalizeOptions(
	options map[identifiers.Identifier]*rpcargs.TesseractNumberOrHash,
) (map[identifiers.Identifier]common.Hash, error) {
	stateHashes := make(map[identifiers.Identifier]common.Hash)

	for id, value := range options {
		stateHash, err := p.getStateHash(getTesseractArgs(id, *value))
		if err != nil {
			return nil, err
		}

		stateHashes[id] = stateHash
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
	if args.ID.IsNil() {
		return nil, common.ErrInvalidIdentifier
	}

	id := p.filterManager.NewTesseractsByAccountFilter(nil, args.ID)

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
		ID:          query.ID,
		Topics:      query.Topics,
	}

	return p.filterManager.GetLogsForQuery(filterQuery)
}

func createCallReceipt(
	sender identifiers.Identifier,
	receipt *common.Receipt,
) *rpcargs.RPCReceipt {
	return &rpcargs.RPCReceipt{
		From:     sender,
		IxHash:   receipt.IxHash,
		Status:   receipt.Status,
		FuelUsed: hexutil.Uint64(receipt.FuelUsed),
		IxOps: func() []*rpcargs.RPCIxOpResult {
			results := make([]*rpcargs.RPCIxOpResult, len(receipt.IxOps))

			for idx, op := range receipt.IxOps {
				results[idx] = &rpcargs.RPCIxOpResult{
					TxType: hexutil.Uint64(op.IxType),
					Status: op.Status,
					Data:   op.Data,
				}
			}

			return results
		}(),
	}
}

func validateOptions(options rpcargs.TesseractNumberOrHash) error {
	if options.TesseractHash != nil && options.TesseractNumber != nil {
		return errors.New("can not use both tesseract number and tesseract hash")
	}

	return nil
}
