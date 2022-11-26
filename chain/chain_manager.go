package chain

import (
	"context"
	"encoding/json"
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
	"github.com/sarvalabs/moichain/jug"
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
		accType types.AccType,
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
	GetStateObjectByHash(addr types.Address, hash types.Hash) (*guna.StateObject, error)
	CreateDirtyObject(addr types.Address, accType types.AccType) *guna.StateObject
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetDirtyObject(addr types.Address) (*guna.StateObject, error)
	GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error)
	DeleteStateObject(addr types.Address)
	GetContextByHash(addr types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(id ...id.KramaID) (keys [][]byte, err error)
	Cleanup(addrs types.Address)
	IsGenesis(addr types.Address) (bool, error)
	FetchContextLock(ts *types.Tesseract) (*ktypes.ICSNodes, error)
	FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
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
	ExecuteInteractions(
		clusterID types.ClusterID,
		ixs []*types.Interaction,
		contextDelta types.ContextDelta,
	) (types.Receipts, error)
	Revert(clusterID types.ClusterID) error
}

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
	exec *jug.Exec,
	senatus reputationEngine,
	metrics *Metrics,
) (*ChainManager, error) {
	orphansCache, err := lru.New(25)
	if err != nil {
		return nil, errors.New("failed to initiate orphan tesseracts cache")
	}

	c := &ChainManager{
		ctx:              ctx,
		cfg:              cfg,
		db:               db,
		mux:              mux,
		ixpool:           ix,
		sm:               sm,
		tesseracts:       cache,
		exec:             exec,
		network:          network,
		orphanTesseracts: orphansCache,
		gridsCache:       NewGridCache(),
		knownTesseracts:  NewKnownCache(150),
		latticeLocks:     locker.New(),
		logger:           logger.Named("Chain-Manager"),
		senatus:          senatus,
		metrics:          metrics,
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

func (c *ChainManager) fetchContextForAgora(t *types.Tesseract) ([]id.KramaID, error) {
	address := t.Address()
	peers := make([]id.KramaID, 0)

	tesseractHash, err := t.Hash()
	if err != nil {
		return nil, err
	}

	for !tesseractHash.IsNil() {
		if len(peers) >= 10 {
			break
		}

		ts, err := c.GetTesseract(tesseractHash, false)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}
		// fetch the context delta
		deltaGroup := ts.Body.ContextDelta[address]
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(address, ts.Header.ContextLock[address].ContextHash)
		if err != nil {
			tesseractHash = ts.Header.PrevHash

			continue
		}

		peers = append(peers, behaviour...)
		peers = append(peers, random...)

		break
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

	randomKeys, err := c.sm.GetPublicKeys(types.ToKIPPeerID(info.RandomSet)...)
	if err != nil {
		return nil, err
	}

	nodeSets.UpdateNodeSet(ktypes.RandomSet, ktypes.NewNodeSet(types.ToKIPPeerID(info.RandomSet), randomKeys))

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

func (c *ChainManager) GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetData, error) {
	rawData, err := c.db.ReadEntry(assetHash)
	if err != nil {
		return nil, err
	}

	assetData := new(gtypes.AssetData)

	if err := assetData.FromBytes(rawData); err != nil {
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
		c.logger.Error("Error fetching receipt root", "error", err.Error(), receiptRoot.Hex(), ixHash.Hex())

		return nil, err
	}

	receipts := new(types.Receipts)

	if err := receipts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceipt(addr types.Address, ixHash types.Hash) (*types.Receipt, error) {
	ts, err := c.GetLatestTesseract(addr, true)
	if err != nil {
		return nil, errors.Wrap(types.ErrReceiptNotFound, err.Error())
	}

	for !ts.Header.PrevHash.IsNil() {
		for _, ix := range ts.Interactions() {
			hash, err := ix.GetIxHash()
			if err != nil {
				return nil, err
			}

			if hash == ixHash {
				return c.getReceipt(hash, ts.Body.ReceiptHash)
			}
		}

		previousTesseract, err := c.GetTesseract(ts.Header.PrevHash, true)
		if err != nil {
			return nil, errors.Wrap(types.ErrReceiptNotFound, err.Error())
		}

		ts = previousTesseract
	}

	return nil, types.ErrReceiptNotFound
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
	verificationInitTime := time.Now()
	publicKeys := make([][]byte, 0, ts.Header.Extra.VoteSet.TrueIndicesSize())
	votesCounter := make([]int, 5) // Only 5 because we don't consider observer nodes vote

	for _, index := range ts.Header.Extra.VoteSet.GetTrueIndices() {
		slotID, _, _, publicKey := ics.GetKramaID(int32(index))
		if slotID != -1 && ts.Header.Extra.VoteSet.GetIndex(index) {
			publicKeys = append(publicKeys, publicKey)
			votesCounter[slotID]++
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

	verified, err := mudra.VerifyAggregateSignature(rawData, ts.Header.Extra.CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	c.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (c *ChainManager) verifyHeaders(ts *types.Tesseract) error {
	var (
		isGenesis bool
		err       error
	)

	c.logger.Trace("Verifying headers", "addr", ts.Header.Address.Hex())

	if ts.Header.ClusterID == "genesis" {
		return nil
	}

	if info, ok := ts.Header.ContextLock[guna.GenesisAddress]; !ok {
		isGenesis, err = c.sm.IsGenesis(ts.Header.Address)
	} else {
		isGenesis, err = c.IsGenesisAt(ts.Header.Address, info.TesseractHash)
	}

	if err != nil {
		return errors.Wrap(err, "Sarga account not found")
	}

	if !isGenesis {
		parent, err := c.GetTesseract(ts.Header.PrevHash, false)
		if err != nil {
			c.logger.Error("Failed to fetch parent tesseract", "error", err)

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
	var accType types.AccType

	tesseractHash, err := t.Hash()
	if err != nil {
		return err
	}

	latticeExists := true

	if exists, err := c.db.Contains(t.Header.PrevHash.Bytes()); !exists || err != nil {
		latticeExists = false
	}

	if stateExists {
		stateObject, err := c.sm.GetDirtyObject(addr)
		if err != nil {
			return err
		}

		if err := stateObject.CommitActiveStorageTreesToDB(); err != nil {
			return err
		}

		accType = stateObject.GetAccountType()

		for k, v := range stateObject.GetDirtyStorage() {
			if err := c.db.CreateEntry(types.Hex2Bytes(k), v); err != nil {
				c.logger.Error("Error writing key to db", "key", k)

				return err
			}
		}
	}

	tsRawData, err := t.Canonical().Bytes()
	if err != nil {
		return err
	}

	if err := c.db.SetTesseract(tesseractHash, tsRawData); err != nil {
		return errors.Wrap(err, "error writing tesseract to db")
	}

	ixRawData, err := t.Interactions().Bytes()
	if err != nil {
		return err
	}

	if err := c.db.SetInteractions(t.InteractionHash(), ixRawData); err != nil {
		return errors.Wrap(err, "error writing interactions to db")
	}

	if err := c.db.SetTesseractHeightEntry(addr, t.Height(), tesseractHash); err != nil {
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

	if err := c.mux.Post(utils.TesseractAddedEvent{Tesseract: t}); err != nil {
		c.logger.Error("error sending tesseract added event", "err", err)
	}

	// update peer occupancy metrics
	if err := c.UpdateNodeInclusivity(t.ContextDelta()); err != nil {
		return errors.Wrap(err, types.ErrUpdatingInclusivity.Error())
	}

	if isBucketCountIncremented && bucketNo != -1 {
		if err := c.mux.Post(utils.SyncStatusUpdate{BucketID: bucketNo, Count: 1}); err != nil {
			log.Panicln(err)
		}
	}

	if cache {
		c.tesseracts.Add(addr, tesseractHash)
		c.tesseracts.Add(tesseractHash, t)
	}

	c.logger.Info("!!!!!.... tesseract  added ....!!!!!", addr.Hex(), tesseractHash.Hex())

	c.sm.Cleanup(t.Header.Address)

	c.ixpool.ResetWithHeaders(t)

	return nil
}

func (c *ChainManager) addTesseractsWithState(
	clusterInfo *ptypes.ICSClusterInfo,
	dirtyStorage map[types.Hash][]byte,
	tesseracts ...*types.Tesseract,
) error {
	tsAdditionInitTime := time.Now()

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

			if err := c.addTesseract(true, addr, ts, true, true); err != nil {
				return err
			}

			return nil
		}(ts.Address(), ts); err != nil {
			return err
		}
	}

	// Add cluster info to db
	if clusterInfo != nil && len(tesseracts) > 0 {
		rawData, err := clusterInfo.Bytes()
		if err != nil {
			return err
		}

		if err := c.db.CreateEntry(tesseracts[0].Body.ConsensusProof.ICSHash.Bytes(), rawData); err != nil {
			return errors.Wrap(err, "failed to write cluster info to db")
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

func (c *ChainManager) addTesseractsWithOutState(
	clusterInfo *ptypes.ICSClusterInfo,
	tesseracts ...*types.Tesseract,
) error {
	for _, ts := range tesseracts {
		if err := func(addr types.Address, ts *types.Tesseract) error {
			tsHash, err := ts.Hash()
			if err != nil {
				return err
			}

			c.latticeLocks.Lock(tsHash.Hex())
			defer func() {
				tsHash, err := ts.Hash()
				if err != nil {
					c.logger.Error("failed to create tesseract hash", "error", err, "addr", addr)

					return
				}

				if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
					c.logger.Error("failed to unlock lattice", "error", err, "addr", addr)
				}

				c.validatedTesseracts.Delete(tsHash)
			}()

			if c.hasTesseract(tsHash) {
				return nil
			}

			if err := c.addTesseract(true, ts.Header.Address, ts, false, true); err != nil {
				return err
			}

			return nil
		}(ts.Address(), ts); err != nil {
			return err
		}
	}

	// Add cluster info to db
	if clusterInfo != nil && len(tesseracts) > 0 {
		rawData, err := clusterInfo.Bytes()
		if err != nil {
			return err
		}

		if err := c.db.CreateEntry(tesseracts[0].Body.ConsensusProof.ICSHash.Bytes(), rawData); err != nil {
			return errors.Wrap(err, "failed to write cluster info to db")
		}
	}

	return nil
}

func (c *ChainManager) validateTesseract(sender id.KramaID, ts *types.Tesseract, ics *ktypes.ICSNodes) error {
	tsHash, err := ts.Hash()
	if err != nil {
		return err
	}

	c.latticeLocks.Lock(tsHash.Hex())
	defer func() {
		if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
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
		if errors.Is(err, types.ErrFetchingTesseract) {
			c.orphanTesseracts.Add(tsHash, ts)
		}

		return err
	}

	verified, err := c.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.Wrap(err, "failed to verify signatures")
	}

	return nil
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

	log.Println("Adding gossiped tesseract", tsHash, ts.Address())

	if c.hasTesseract(tsHash) {
		return nil
	}

	ics, err := c.fetchICSNodeSet(ts, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to fetch ICSNodeSet")
	}

	if err := c.validateTesseract(sender, ts, ics); err != nil {
		return err
	}

	index, _ := ics.GetIndex(c.network.GetKramaID())

	if c.cfg.ShouldExecute && index == -1 { // TODO: Should execute if validator responded false to operator request
		if isGridComplete := c.gridsCache.AddTesseract(ts); !isGridComplete {
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

		return c.addTesseractsWithState(clusterInfo, dirtyStorage, tesseractGrid...)
	}

	if err = c.addTesseractsWithOutState(clusterInfo, ts); err != nil {
		return err
	}

	c.logger.Info("Added tesseract without state", "Addr", ts.Header.Address.Hex(), "Hash", tsHash.Hex())
	c.sendTesseractSyncRequest(ts, clusterInfo)

	return nil
}

func (c *ChainManager) AddTesseracts(tesseracts []*types.Tesseract, dirtyStorage map[types.Hash][]byte) error {
	if tesseracts != nil {
		gridHash := tesseracts[0].GridHash()
		for _, ts := range tesseracts {
			if ts.GridHash() != gridHash {
				return errors.New("grid id mismatch")
			}
		}

		return c.addTesseractsWithState(nil, dirtyStorage, tesseracts...)
	}

	return errors.New("nil grid")
}

func (c *ChainManager) setupSargaAccount(accounts []AccountInfo) error {
	var contextDelta types.ContextDelta

	stateObject := c.sm.CreateDirtyObject(guna.GenesisAddress, types.SargaAccount)

	for _, info := range accounts {
		addr := types.HexToAddress(info.Address)
		if addr == guna.GenesisAddress {
			bContext := types.ToKIPPeerID(info.BehaviourContext)
			rContext := types.ToKIPPeerID(info.RandomContext)
			contextDelta = map[types.Address]*types.DeltaGroup{
				addr: {
					BehaviouralNodes: bContext,
					RandomNodes:      rContext,
				},
			}

			if _, err := stateObject.CreateContext(bContext, rContext); err != nil {
				return errors.New("context initiation failed in genesis")
			}

			if _, err := stateObject.CreateStorageTreeForLogic(guna.GenesisLogicID); err != nil {
				return errors.Wrap(err, "failed to create storage tree")
			}

			break
		}
	}

	for _, info := range accounts {
		addr := types.HexToAddress(info.Address)
		if addr != guna.GenesisAddress {
			// Add account to sarga storage tree
			if err := stateObject.SetStorageEntry(
				guna.GenesisLogicID,
				addr.Bytes(),
				GenesisIxHash.Bytes(),
			); err != nil {
				return err
			}
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return err
	}

	tesseract, err := CreateGenesisTesseract(guna.GenesisAddress, stateHash, stateObject.ContextHash(), contextDelta)
	if err != nil {
		return err
	}

	if err := c.AddGenesis(guna.GenesisAddress, tesseract); err != nil {
		c.logger.Error("Error adding genesis", "err", err)

		return errors.New("error adding genesis tesseract")
	}

	return nil
}

func (c *ChainManager) IsGenesisAt(address types.Address, hash types.Hash) (bool, error) {
	ts, err := c.GetTesseract(hash, false)
	if err != nil {
		return false, err
	}

	object, err := c.sm.GetStateObjectByHash(ts.Header.Address, ts.Body.StateHash)
	if err != nil {
		return false, err
	}

	_, err = object.GetStorageEntry(guna.GenesisLogicID, address.Bytes())
	if err != nil {
		return true, nil
	}

	return false, nil
}

func (c *ChainManager) AddGenesis(addr types.Address, t *types.Tesseract) error {
	return c.addTesseract(true, addr, t, true, true)
}

func (c *ChainManager) SetupGenesis(path string) error {
	if path == "nil" {
		c.logger.Debug("Skipping genesis")

		return nil
	}

	genesisAccounts := new(Genesis)

	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrap(types.ErrGenesisSetupFailed, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisAccounts); err != nil {
		return err
	}

	if err = c.setupSargaAccount(genesisAccounts.Accounts); err != nil {
		return errors.Wrap(types.ErrGenesisSetupFailed, "failed to setup sarga account")
	}

	for _, v := range genesisAccounts.Accounts {
		addr := types.HexToAddress(v.Address)
		if addr == guna.GenesisAddress {
			continue
		}

		// Update the context
		stateObject := c.sm.CreateDirtyObject(addr, v.AccType)
		bContext := types.ToKIPPeerID(v.BehaviourContext)
		rContext := types.ToKIPPeerID(v.RandomContext)

		if _, err = stateObject.CreateContext(bContext, rContext); err != nil {
			return errors.New("context initiation failed in genesis")
		}

		if len(v.AssetDetails) > 0 {
			asset := v.AssetDetails[0]

			assetID, err := stateObject.CreateAsset(
				uint8(asset.Dimension),
				asset.IsFungible,
				asset.IsMintable,
				asset.Symbol,
				int64(asset.TotalSupply),
				nil,
			)
			if err != nil {
				c.logger.Error("Error creating asset", err)
			}

			c.logger.Info("Asset created ", "id", assetID)
		} else if v.Balances != nil {
			for _, balance := range v.Balances {
				stateObject.AddBalance(types.AssetID(balance.AssetID), new(big.Int).SetInt64(balance.Amount))
			}
		}

		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		contextDelta := map[types.Address]*types.DeltaGroup{
			addr: {
				BehaviouralNodes: bContext,
				RandomNodes:      rContext,
			},
		}

		tesseract, err := CreateGenesisTesseract(addr, stateHash, stateObject.ContextHash(), contextDelta)
		if err != nil {
			return err
		}

		if err := c.AddGenesis(addr, tesseract); err != nil {
			return errors.New("error adding genesis tesseract ")
		}
	}

	return nil
}

func (c *ChainManager) sendTesseractSyncRequest(ts *types.Tesseract, clusterInfo *ptypes.ICSClusterInfo) {
	// fetch context info for agora
	fetchContext, err := c.fetchContextForAgora(ts)
	if err != nil {
		c.logger.Error("Error fetching fetchContext for agora", "error", err)
	}

	// add ics random nodes to agora context
	fetchContext = append(fetchContext, types.ToKIPPeerID(clusterInfo.RandomSet)...)

	event := utils.TesseractSyncEvent{Tesseract: ts, Context: fetchContext}
	if err := c.mux.Post(event); err != nil {
		log.Panic(err)
	}
}

func (c *ChainManager) tesseractHandler(pubSubMsg *pubsub.Message) error {
	msg := new(ptypes.TesseractMessage)
	tsAdditionInitTime := time.Now()
	//	v1msg := proto.MessageV1(msg)
	if err := msg.FromBytes(pubSubMsg.GetData()); err != nil {
		log.Panic(err)
	}

	ts := msg.Tesseract
	// TODO:Add logic to avoid posting event if validator is part of the tesseract already

	if ts.Operator() == string(c.network.GetKramaID()) {
		return nil
	}

	tsHash, err := ts.Hash()
	if err != nil {
		return err
	}

	if exists, err := c.db.Contains(tsHash.Bytes()); err != nil || exists {
		c.logger.Info("Skipping tesseract", "hash", tsHash)

		return nil
	}

	clusterInfo := new(ptypes.ICSClusterInfo)
	if err := clusterInfo.FromBytes(msg.Delta[msg.Tesseract.GetICSHash()]); err != nil {
		c.logger.Error("Error depolarising cluster info", "err", err)
	}

	c.logger.Trace("Tesseract Received from", "Sender", msg.Sender,
		"Hash", tsHash.Hex(),
		"Address", msg.Tesseract.Header.Address.Hex())

	if !c.knownTesseracts.Contains(tsHash) {
		c.knownTesseracts.Add(tsHash)

		if err := c.AddTesseractWithOutState(ts, msg.Sender, clusterInfo); err != nil {
			c.logger.Error("Error adding tesseract ", "error", err, "addr", ts.Address(), "hash", tsHash)
		} else {
			c.metrics.captureStatelessTesseractCounter(1)
			c.metrics.captureStatelessTesseractAdditionTime(tsAdditionInitTime)
		}
	}

	return nil
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
	for _, ix := range tesseracts[0].Interactions() {
		ixHash, err := ix.GetIxHash()
		if err != nil {
			return false
		}

		receipt, err := receipts.GetReceipt(ixHash)
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
	groupHash := tesseracts[0].Header.GridHash

	receiptsHash, err := receipts.Hash()
	if err != nil {
		return false
	}

	for _, ts := range tesseracts {
		if ts.ReceiptHash() != receiptsHash || ts.Header.GridHash != groupHash {
			return false
		}
	}

	return true
}
