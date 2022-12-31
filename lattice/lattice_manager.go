package lattice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	ktypes "github.com/sarvalabs/moichain/krama/types"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const (
	TesseractTopic = "MOI_PUBSUB_TESSERACT"
)

type db interface {
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id types.Address,
		height *big.Int,
		tesseractHash types.Hash,
		accType types.AccountType,
		latticeExists bool,
		tesseractExists bool,
	) (int32, bool, error)
	GetTesseract(hash types.Hash) ([]byte, error)
	SetTesseract(hash types.Hash, data []byte) error
	HasTesseract(hash types.Hash) (bool, error)
	SetInteractions(hash types.Hash, data []byte) error
	GetTesseractHeightEntry(addr types.Address, height uint64) ([]byte, error)
	SetTesseractHeightEntry(addr types.Address, height uint64, hash types.Hash) error
}

type reputationEngine interface {
	UpdateInclusivity(key id.KramaID, delta int64) error
}

type stateManager interface {
	CreateDirtyObject(addr types.Address, accType types.AccountType) *guna.StateObject
	FlushDirtyObject(addrs types.Address) error
	GetAccTypeUsingStateObject(address types.Address) (types.AccountType, error)
	GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error)
	GetContextByHash(addr types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(id ...id.KramaID) (keys [][]byte, err error)
	Cleanup(addrs types.Address)
	IsAccountRegistered(addr types.Address) (bool, error)
	IsAccountRegisteredAt(addr types.Address, tesseractHash types.Hash) (bool, error)
	FetchContextLock(ts *types.Tesseract) (*ktypes.ICSNodes, error)
	FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
	SetupNewAccount(info *gtypes.AccountSetupArgs) (types.Hash, types.Hash, error)
	SetupSargaAccount(
		sargaAcc *gtypes.AccountSetupArgs,
		otherAccounts []*gtypes.AccountSetupArgs,
	) (types.Hash, types.Hash, error)
}

type server interface {
	GetKramaID() id.KramaID
	Broadcast(topic string, data []byte) error
	Unsubscribe(topic string) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}

type ixpool interface {
	ResetWithHeaders(ts *types.Tesseract)
}

type executor interface {
	ExecuteInteractions(types.ClusterID, types.Interactions, types.ContextDelta) (types.Receipts, error)
	Revert(types.ClusterID) error
}

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type ChainManager struct {
	ctx                 context.Context
	cfg                 *common.ChainConfig
	db                  db
	mux                 *utils.TypeMux
	ixpool              ixpool
	tesseracts          *lru.Cache
	orphanTesseracts    *lru.Cache
	gridsCache          *GridCache
	sm                  stateManager
	validatedTesseracts sync.Map
	latticeLocks        *locker.Locker
	logger              hclog.Logger
	knownTesseracts     *KnownCache
	senatus             reputationEngine
	network             server
	exec                executor
	metrics             *Metrics
	signatureVerifier   AggregatedSignatureVerifier
}

func NewChainManager(
	ctx context.Context,
	cfg *common.ChainConfig,
	db db,
	sm stateManager,
	logger hclog.Logger,
	mux *utils.TypeMux,
	network server,
	ix ixpool,
	cache *lru.Cache,
	exec executor,
	senatus reputationEngine,
	metrics *Metrics,
	verifier AggregatedSignatureVerifier,
) (*ChainManager, error) {
	orphansCache, err := lru.New(25)
	if err != nil {
		return nil, errors.New("failed to initiate orphan tesseracts cache")
	}

	c := &ChainManager{
		ctx:               ctx,
		cfg:               cfg,
		db:                db,
		mux:               mux,
		ixpool:            ix,
		sm:                sm,
		tesseracts:        cache,
		exec:              exec,
		network:           network,
		orphanTesseracts:  orphansCache,
		gridsCache:        NewGridCache(),
		knownTesseracts:   NewKnownCache(150),
		latticeLocks:      locker.New(),
		logger:            logger.Named("Chain-Manager"),
		senatus:           senatus,
		metrics:           metrics,
		signatureVerifier: verifier,
	}

	return c, nil
}

func (c *ChainManager) hasTesseract(hash types.Hash) bool {
	exists, err := c.db.HasTesseract(hash)
	if err != nil {
		c.logger.Error("Failed to fetch hash from db", "error", err)

		return exists
	}

	return exists
}

func (c *ChainManager) fetchContextForAgora(ts types.Tesseract) ([]id.KramaID, error) {
	var (
		address = ts.Address()
		peers   = make([]id.KramaID, 0)
	)

	for {
		if len(peers) >= 10 {
			break
		}

		// fetch the context delta
		deltaGroup := ts.Body.ContextDelta[address]
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(address, ts.Header.ContextLock[address].ContextHash)
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.PreviousHash().IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.PreviousHash(), false)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}

		ts = *t
	}

	return peers, nil
}

func (c *ChainManager) fetchICSNodeSet(
	ts *types.Tesseract,
	info *ptypes.ICSClusterInfo,
) (*ktypes.ICSNodes, error) {
	nodeSets, err := c.sm.FetchContextLock(ts)
	if err != nil {
		return nil, err
	}

	randomKeys, err := c.sm.GetPublicKeys(utils.KramaIDFromString(info.RandomSet)...)
	if err != nil {
		return nil, err
	}

	nodeSets.UpdateNodeSet(ktypes.RandomSet, ktypes.NewNodeSet(utils.KramaIDFromString(info.RandomSet), randomKeys))

	return nodeSets, nil
}

func (c *ChainManager) AddKnownHashes(tesseracts []*types.Tesseract) {
	for _, v := range tesseracts {
		c.knownTesseracts.Add(v.Hash())
	}
}

func (c *ChainManager) UpdateNodeInclusivity(delta types.ContextDelta) error {
	for _, deltaGroup := range delta {
		for _, kramaID := range deltaGroup.BehaviouralNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.RandomNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.ReplacedNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, -1); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *ChainManager) GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	if withInteractions {
		return c.sm.FetchTesseractFromDB(hash, withInteractions)
	}

	tesseractData, isCached := c.tesseracts.Get(hash)
	if !isCached {
		tesseract, err := c.sm.FetchTesseractFromDB(hash, withInteractions)
		if err != nil {
			return nil, err
		}

		c.tesseracts.Add(hash, tesseract)

		return tesseract, nil
	}

	tesseract, ok := tesseractData.(*types.Tesseract)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return tesseract, nil
}

func (c *ChainManager) GetTesseractByHeight(
	address types.Address,
	height uint64,
	withInteractions bool,
) (*types.Tesseract, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return c.GetTesseract(types.BytesToHash(tesseractHash), withInteractions)
}

func (c *ChainManager) GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetObject, error) {
	rawData, err := c.db.ReadEntry(assetHash)
	if err != nil {
		return nil, err
	}

	assetData := new(gtypes.AssetObject)

	if err = assetData.FromBytes(rawData); err != nil {
		return nil, err
	}

	return assetData, nil
}

func (c *ChainManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	if addr.IsNil() {
		return nil, types.ErrInvalidAddress
	}

	return c.sm.GetLatestTesseract(addr, withInteractions)
}

func (c *ChainManager) getReceipt(ixHash, receiptRoot types.Hash) (*types.Receipt, error) {
	rawData, err := c.db.ReadEntry(receiptRoot.Bytes())
	if err != nil {
		c.logger.Error("Error fetching receipt root", "error", err.Error(), receiptRoot, ixHash)

		return nil, err
	}

	receipts := new(types.Receipts)

	if err = receipts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceiptByIxHash(addr types.Address, ixHash types.Hash) (*types.Receipt, error) {
	ts, err := c.GetLatestTesseract(addr, true)
	if err != nil {
		return nil, errors.Wrap(types.ErrReceiptNotFound, err.Error())
	}

	for {
		for _, ix := range ts.Interactions() {
			if hash := ix.Hash(); hash == ixHash {
				return c.getReceipt(hash, ts.Body.ReceiptHash)
			}
		}

		if ts.Header.PrevHash.IsNil() {
			return nil, types.ErrReceiptNotFound
		}

		previousTesseract, err := c.GetTesseract(ts.Header.PrevHash, true)
		if err != nil {
			return nil, errors.Wrap(types.ErrReceiptNotFound, err.Error())
		}

		ts = previousTesseract
	}
}

func (c *ChainManager) isSealValid(ts *types.Tesseract, id id.KramaID) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(id)
	if err != nil {
		c.logger.Error("Error fetching public key", "err", err)

		return false, err
	}

	rawData, err := ts.Bytes()
	if err != nil {
		return false, err
	}

	return mudra.Verify(rawData, ts.Seal, publicKey[0])
}

func (c *ChainManager) verifySignatures(ts *types.Tesseract, ics *ktypes.ICSNodes) (bool, error) {
	var (
		verificationInitTime = time.Now()
		publicKeys           = make([][]byte, 0, ts.Header.Extra.VoteSet.TrueIndicesSize())
		votesCounter         = make([]int, 5) // Only 5 because we don't consider observer nodes vote
	)

	for _, index := range ts.Header.Extra.VoteSet.GetTrueIndices() {
		slots, _, _, publicKey := ics.GetKramaID(int32(index))
		if slots != nil { // ts.Header.Extra.VoteSet.GetIndex(index)
			publicKeys = append(publicKeys, publicKey)

			for _, slotID := range slots {
				votesCounter[slotID]++
			}
		} else {
			c.logger.Debug("Error fetching validator address", "index", index)
		}
	}

	senderContextSize := votesCounter[0] + votesCounter[1]
	receiverContextSize := votesCounter[2] + votesCounter[3]
	randomContextSize := votesCounter[4]

	if senderContextSize < ics.SenderQuorumSize() ||
		receiverContextSize < ics.ReceiverQuorumSize() ||
		randomContextSize < ics.RandomQuorumSize() {
		return false, types.ErrQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   ktypes.PRECOMMIT,
		Round:  ts.Header.Extra.Round,
		GridID: ts.Header.Extra.GridID,
	}

	rawData, err := vote.Bytes()
	if err != nil {
		return false, err
	}

	verified, err := c.signatureVerifier(rawData, ts.Header.Extra.CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	c.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (c *ChainManager) verifyHeaders(ts *types.Tesseract) error {
	var (
		accountRegistered bool
		err               error
	)

	c.logger.Trace("Verifying headers", "addr", ts.Header.Address.Hex(), ts.Header.ContextLock)

	if ts.Header.ClusterID == "genesis" {
		return nil
	}

	if info, ok := ts.Header.ContextLock[guna.SargaAddress]; !ok {
		accountRegistered, err = c.sm.IsAccountRegistered(ts.Header.Address)
	} else {
		accountRegistered, err = c.sm.IsAccountRegisteredAt(ts.Header.Address, info.TesseractHash)
	}

	if err != nil {
		return errors.Wrap(err, "Sarga account not found")
	}

	if accountRegistered {
		parent, err := c.GetTesseract(ts.Header.PrevHash, false)
		if err != nil {
			c.logger.Error("Failed to fetch parent tesseract", "error", err, ts.Address())

			return types.ErrFetchingTesseract
		}

		// Check Heights
		if parent.Header.Height != ts.Header.Height-1 {
			return types.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Header.Timestamp < parent.Header.Timestamp {
			return types.ErrInvalidBlockTime
		}
	}

	return nil
}

func (c *ChainManager) addTesseract(
	cache bool,
	addr types.Address,
	t *types.Tesseract,
	stateExists,
	tesseractExists bool,
) error {
	var (
		accType       types.AccountType
		err           error
		latticeExists = true
	)

	tesseractHash, err := t.Hash()
	if err != nil {
		return err
	}

	if exists, err := c.db.HasTesseract(t.Header.PrevHash); !exists || err != nil {
		latticeExists = false
	}

	if stateExists {
		if err = c.sm.FlushDirtyObject(addr); err != nil {
			return err
		}

		accType, err = c.sm.GetAccTypeUsingStateObject(addr)
		if err != nil {
			return errors.Wrap(err, "failed to fetch account type")
		}
	}

	tsRawData, err := t.Canonical().Bytes()
	if err != nil {
		return err
	}

	if err = c.db.SetTesseract(tesseractHash, tsRawData); err != nil {
		return errors.Wrap(err, "error writing tesseract to db")
	}

	ixRawData, err := t.Interactions().Bytes()
	if err != nil {
		return err
	}

	if err = c.db.SetInteractions(t.InteractionHash(), ixRawData); err != nil {
		return errors.Wrap(err, "error writing interactions to db")
	}

	if err = c.db.SetTesseractHeightEntry(addr, t.Height(), tesseractHash); err != nil {
		return errors.Wrap(err, "failed to write tesseract height entry")
	}

	bucketNo, isBucketCountIncremented, err := c.db.UpdateAccMetaInfo(
		addr,
		new(big.Int).SetUint64(t.Header.Height),
		tesseractHash,
		accType,
		latticeExists,
		tesseractExists,
	)
	if err != nil {
		return errors.Wrap(err, "account meta info update failed")
	}

	if err = c.mux.Post(utils.TesseractAddedEvent{Tesseract: t}); err != nil {
		c.logger.Error("error sending tesseract added event", "err", err)
	}

	// update peer occupancy metrics
	if err = c.UpdateNodeInclusivity(t.ContextDelta()); err != nil {
		return errors.Wrap(err, types.ErrUpdatingInclusivity.Error())
	}

	if isBucketCountIncremented && bucketNo != -1 {
		if err = c.mux.Post(utils.SyncStatusUpdate{BucketID: bucketNo, Count: 1}); err != nil {
			return errors.Wrap(err, "failed to post sync event")
		}
	}

	if cache {
		c.tesseracts.Add(addr, tesseractHash)
		c.tesseracts.Add(tesseractHash, t.GetTesseractWithoutIxns())
	}

	c.logger.Info("!!!!!.... tesseract  added ....!!!!!", addr, tesseractHash)

	c.sm.Cleanup(t.Header.Address)

	c.ixpool.ResetWithHeaders(t)

	return nil
}

func (c *ChainManager) addTesseractsWithState(
	dirtyStorage map[types.Hash][]byte,
	tesseracts ...*types.Tesseract,
) error {
	tsAdditionInitTime := time.Now()

	if len(tesseracts) == 0 {
		return errors.New("empty tesseracts")
	}

	if _, ok := dirtyStorage[tesseracts[0].GetICSHash()]; !ok {
		return errors.New("cluster info not found")
	}

	for _, ts := range tesseracts {
		if err := func(addr types.Address, ts *types.Tesseract) error {
			tsHash, err := ts.Hash()
			if err != nil {
				return err
			}

			c.latticeLocks.Lock(tsHash.Hex())

			defer func() {
				if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
					c.logger.Error("failed to unlock lattice", "error", err, "addr", addr)
				}

				c.validatedTesseracts.Delete(tsHash)
			}()

			if c.hasTesseract(tsHash) {
				return nil
			}

			if err = c.addTesseract(true, addr, ts, true, true); err != nil {
				return err
			}

			return nil
		}(ts.Address(), ts); err != nil {
			return err
		}
	}

	for key, value := range dirtyStorage {
		if err := c.db.CreateEntry(key.Bytes(), value); err != nil {
			return errors.Wrap(err, "failed to write dirty keys")
		}
	}

	c.metrics.captureStatefulTesseractCounter(1)
	c.metrics.captureStatefulTesseractAdditionTime(tsAdditionInitTime)

	return nil
}

func (c *ChainManager) AddSyncedTesseract(
	clusterInfo *ptypes.ICSClusterInfo,
	tesseracts ...*types.Tesseract,
) error {
	if clusterInfo == nil {
		return errors.New("nil cluster info")
	}

	dirtyStorage := make(map[types.Hash][]byte)

	rawData, err := clusterInfo.Bytes()
	if err != nil {
		return err
	}

	dirtyStorage[tesseracts[0].GetICSHash()] = rawData

	return c.addTesseractsWithState(dirtyStorage, tesseracts...)
}

func (c *ChainManager) validateTesseract(sender id.KramaID, ts *types.Tesseract, ics *ktypes.ICSNodes) error {
	tsHash, err := ts.Hash()
	if err != nil {
		return err
	}

	c.latticeLocks.Lock(tsHash.Hex())
	defer func() {
		if err = c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
			c.logger.Error("failed to unlock lattice", "error", err, "addr", ts.Address())
		}
	}()

	_, ok := c.validatedTesseracts.Load(tsHash)
	if ok || c.hasTesseract(tsHash) {
		return types.ErrAlreadyKnown
	}

	validSeal, err := c.isSealValid(ts, sender)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal ", "err", err)

		return types.ErrInvalidSeal
	}

	if err = c.verifyHeaders(ts); err != nil {
		switch {
		case errors.Is(err, types.ErrFetchingTesseract):
			c.orphanTesseracts.Add(tsHash, ts)
		default:
			return err
		}
	}

	verified, err := c.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.Wrap(err, "failed to verify signatures")
	}

	return nil
}

func (c *ChainManager) executeAndAdd(ts *types.Tesseract, clusterInfo *ptypes.ICSClusterInfo) error {
	if clusterInfo == nil {
		return errors.New("empty cluster info")
	}

	if isGridComplete, err := c.gridsCache.AddTesseract(ts); !isGridComplete {
		if err != nil {
			c.logger.Error("Failed to add the tesseract to grids cache", "error", err)
		}

		return nil
	}

	var tesseractGrid []*types.Tesseract

	if ts.GridLength() == 1 {
		tesseractGrid = []*types.Tesseract{ts}
	} else {
		tesseractGrid = c.gridsCache.CleanupGrid(ts.GridHash())
	}

	if tesseractGrid == nil {
		return errors.New("nil grid")
	}

	dirtyStorage, err := c.executeAndValidate(tesseractGrid)
	if err != nil {
		return err
	}

	rawData, err := clusterInfo.Bytes()
	if err != nil {
		return err
	}

	dirtyStorage[types.GetHash(rawData)] = rawData

	return c.addTesseractsWithState(dirtyStorage, tesseractGrid...)
}

func (c *ChainManager) AddTesseractWithOutState(
	ts *types.Tesseract,
	sender id.KramaID,
	clusterInfo *ptypes.ICSClusterInfo,
) error {
	tsHash, err := ts.Hash()
	if err != nil {
		return err
	}

	c.logger.Debug("Adding gossiped tesseract", tsHash, ts.Address())

	if c.hasTesseract(tsHash) {
		return types.ErrAlreadyKnown
	}

	ics, err := c.fetchICSNodeSet(ts, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to fetch ICSNodeSet")
	}

	if err = c.validateTesseract(sender, ts, ics); err != nil {
		return err
	}

	index, _ := ics.GetIndex(c.network.GetKramaID())

	if c.cfg.ShouldExecute && index == -1 { // TODO: Should execute if validator responded false to operator request
		return c.executeAndAdd(ts, clusterInfo)
	}

	c.sendTesseractSyncRequest(ts, clusterInfo)

	return nil
}

func (c *ChainManager) AddTesseracts(tesseracts []*types.Tesseract, dirtyStorage map[types.Hash][]byte) error {
	if len(tesseracts) == 0 {
		return errors.New("nil grid")
	}

	if len(dirtyStorage) == 0 {
		return errors.New("empty dirty storage")
	}

	gridHash := tesseracts[0].GridHash()
	for _, ts := range tesseracts {
		if ts.GridHash() != gridHash {
			return errors.New("grid id mismatch")
		}
	}

	return c.addTesseractsWithState(dirtyStorage, tesseracts...)
}

func validateAssetDetails(info *AssetInfo) (*types.AssetDescriptor, error) {
	var err error

	assetDescriptor := new(types.AssetDescriptor)
	assetType := types.AssetKind(info.Type)

	switch assetType {
	case types.AssetKindValue, types.AssetKindFile, types.AssetKindLogic, types.AssetKindContext:
		assetDescriptor.Type = assetType
	default:
		return nil, types.ErrInvalidAssetKind
	}

	assetDescriptor.Owner, err = utils.ValidateAddress(info.Owner)
	if err != nil {
		return nil, err
	}

	assetDescriptor.Symbol = info.Symbol
	assetDescriptor.Supply = big.NewInt(int64(info.TotalSupply))
	assetDescriptor.Dimension = info.Dimension
	assetDescriptor.Decimals = info.Decimals
	assetDescriptor.IsFungible = info.IsFungible
	assetDescriptor.IsMintable = info.IsMintable
	assetDescriptor.IsTransferable = info.IsTransferable
	assetDescriptor.LogicID = types.Hex2Bytes(info.LogicID)

	return assetDescriptor, nil
}

func (c *ChainManager) validateAccountCreationInfo(acc AccountInfo) (*gtypes.AccountSetupArgs, error) {
	// check for address validity
	var (
		accArgs = new(gtypes.AccountSetupArgs)
		err     error
	)

	accArgs.Balances = make(map[types.AssetID]*big.Int)
	accArgs.Assets = make([]*types.AssetDescriptor, len(acc.AssetDetails))

	accArgs.Address, err = utils.ValidateAddress(acc.Address)
	if err != nil {
		return nil, err
	}

	accArgs.AccType, err = utils.ValidateAccountType(acc.AccountType)
	if err != nil {
		return nil, err
	}

	// check for assetID validity
	for _, v := range acc.Balances {
		assetID, err := utils.ValidateAssetID(v.AssetID)
		if err != nil {
			return nil, err
		}

		if v.Amount < 0 {
			return nil, errors.New("invalid balance")
		}

		accArgs.Balances[assetID] = big.NewInt(v.Amount)
	}

	for i, assetDetails := range acc.AssetDetails {
		accArgs.Assets[i], err = validateAssetDetails(assetDetails)
		if err != nil {
			return nil, errors.Wrap(err, "invalid asset details")
		}
	}

	accArgs.BehaviouralContext = utils.KramaIDFromString(acc.BehaviourContext)
	accArgs.RandomContext = utils.KramaIDFromString(acc.RandomContext)
	accArgs.MoiID = acc.MOIId

	return accArgs, nil
}

func (c *ChainManager) addGenesisTesseract(
	address types.Address,
	stateHash, contextHash types.Hash,
	contextDelta types.ContextDelta,
) error {
	tesseract, err := createGenesisTesseract(address, stateHash, contextHash, contextDelta)
	if err != nil {
		return err
	}

	if err = c.addTesseract(true, address, tesseract, true, true); err != nil {
		return errors.New("error adding genesis tesseract")
	}

	return nil
}

func (c *ChainManager) SetupGenesis(path string) error {
	sargaAccount, genesisAccounts, err := c.parseGenesisFile(path)
	if err != nil {
		return err
	}

	stateHash, contextHash, err := c.sm.SetupSargaAccount(sargaAccount, genesisAccounts)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	if err = c.addGenesisTesseract(
		sargaAccount.Address,
		stateHash,
		contextHash,
		sargaAccount.ContextDelta(),
	); err != nil {
		return err
	}

	for _, v := range genesisAccounts {
		stateHash, contextHash, err = c.sm.SetupNewAccount(v)
		if err != nil {
			return err
		}

		if err = c.addGenesisTesseract(
			v.Address,
			stateHash,
			contextHash,
			v.ContextDelta(),
		); err != nil {
			return err
		}
	}

	return nil
}

func (c *ChainManager) parseGenesisFile(path string) (*gtypes.AccountSetupArgs, []*gtypes.AccountSetupArgs, error) {
	genesisData := new(Genesis)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	sargaAccount, err := c.validateAccountCreationInfo(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	genesisAccounts := make([]*gtypes.AccountSetupArgs, len(genesisData.Accounts))

	for index, accInfo := range genesisData.Accounts {
		genesisAccounts[index], err = c.validateAccountCreationInfo(accInfo)
		if err != nil {
			return nil, nil, errors.Wrap(err, fmt.Sprintf("invalid genesis account info %s", accInfo.Address))
		}
	}

	return sargaAccount, genesisAccounts, nil
}

func (c *ChainManager) sendTesseractSyncRequest(ts *types.Tesseract, clusterInfo *ptypes.ICSClusterInfo) {
	// fetch context info for agora
	fetchContext, err := c.fetchContextForAgora(*ts)
	if err != nil {
		c.logger.Error("Error fetching context for agora", "error", err)
	}

	// add ics random nodes to agora context
	fetchContext = append(fetchContext, utils.KramaIDFromString(clusterInfo.RandomSet)...)

	event := utils.TesseractSyncEvent{Tesseract: ts, ClusterInfo: clusterInfo, Context: fetchContext}
	if err := c.mux.Post(event); err != nil {
		log.Panic(err)
	}
}

func (c *ChainManager) tesseractHandler(pubSubMsg *pubsub.Message) error {
	var (
		msg                = new(ptypes.TesseractMessage)
		tsAdditionInitTime = time.Now()
	)

	if err := msg.FromBytes(pubSubMsg.GetData()); err != nil {
		return err
	}

	ts, err := msg.Tesseract()
	if err != nil {
		return errors.Wrap(err, "failed to parse tesseract message")
	}

	// TODO:Add logic to avoid posting event if validator is part of the tesseract already

	if ts.Operator() == string(c.network.GetKramaID()) {
		return errors.New("this node is the operator of tesseract")
	}

	tsHash, err := ts.Hash()
	if err != nil {
		return err
	}

	if exists, err := c.db.HasTesseract(tsHash); err != nil || exists {
		c.logger.Info("Skipping tesseract", "hash", tsHash)

		return types.ErrAlreadyKnown
	}

	clusterInfo := new(ptypes.ICSClusterInfo)
	if err = clusterInfo.FromBytes(msg.Delta[ts.GetICSHash()]); err != nil {
		return err
	}

	c.logger.Trace("Tesseract Received from",
		"Sender", msg.Sender,
		"Hash", tsHash,
		"Address", ts.Header.Address.Hex())

	if !c.knownTesseracts.Contains(tsHash) {
		c.knownTesseracts.Add(tsHash)

		if err = c.AddTesseractWithOutState(ts, msg.Sender, clusterInfo); err != nil {
			c.logger.Error("Error adding tesseract ", "error", err, "addr", ts.Address(), "hash", tsHash)

			return err
		}

		c.metrics.captureStatelessTesseractCounter(1)
		c.metrics.captureStatelessTesseractAdditionTime(tsAdditionInitTime)

		return nil
	}

	return types.ErrAlreadyKnown
}

func (c *ChainManager) Start() error {
	if err := c.network.Subscribe(c.ctx, TesseractTopic, c.tesseractHandler); err != nil {
		return errors.Wrap(err, "failed to subscribe pubsub topic")
	}

	return nil
}

func (c *ChainManager) Close() {
	log.Println("Closing Chain manager")
}

func (c *ChainManager) executeAndValidate(ts []*types.Tesseract) (map[types.Hash][]byte, error) {
	dirtyStorage := make(map[types.Hash][]byte)

	receipts, err := c.exec.ExecuteInteractions(ts[0].ClusterID(), ts[0].Interactions(), ts[0].ContextDelta())
	if err != nil {
		return nil, err
	}

	if !isReceiptAndGroupHashValid(ts, receipts) || !areStateHashesValid(ts, receipts) {
		if err = c.exec.Revert(ts[0].ClusterID()); err != nil {
			c.logger.Error("Failed to revert the execution changes", "cluster-id", ts[0].ClusterID())

			return nil, errors.Wrap(err, "failed to revert the execution changes")
		}

		return nil, errors.New("failed to validate the tesseract")
	}

	receiptHash, err := receipts.Hash()
	if err != nil {
		return nil, err
	}

	dirtyStorage[receiptHash], err = receipts.Bytes()
	if err != nil {
		return nil, err
	}

	return dirtyStorage, nil
}

func areStateHashesValid(tesseracts []*types.Tesseract, receipts types.Receipts) bool {
	if len(tesseracts[0].Interactions()) == 0 {
		return false
	}

	for _, ix := range tesseracts[0].Interactions() {
		receipt, err := receipts.GetReceipt(ix.Hash())
		if err != nil {
			return false
		}

		for _, ts := range tesseracts {
			if receipt.StateHashes[ts.Address()] != ts.StateHash() {
				return false
			}
		}
	}

	return true
}

func isReceiptAndGroupHashValid(tesseracts []*types.Tesseract, receipts types.Receipts) bool {
	gridHash := tesseracts[0].Header.GridHash

	receiptsHash, err := receipts.Hash()
	if err != nil {
		return false
	}

	for _, ts := range tesseracts {
		if ts.ReceiptHash() != receiptsHash || ts.Header.GridHash != gridHash {
			return false
		}
	}

	return true
}
