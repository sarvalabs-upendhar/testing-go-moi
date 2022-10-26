package chain

import (
	"context"
	"encoding/json"
	"log"
	"math/big"
	"os"

	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/jug"
	"gitlab.com/sarvalabs/moichain/mudra"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
)

const (
	TesseractTopic = "MOI_PUBSUB_TESSERACT"
)

type db interface {
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id ktypes.Address,
		height *big.Int,
		tesseractHash ktypes.Hash,
		accType ktypes.AccType,
		latticeExists bool,
		tesseractExists bool,
	) (int32, int64, error)
}

type reputationEngine interface {
	UpdateInclusivity(key id.KramaID, delta int64) error
}
type stateManager interface {
	GetStateObjectByHash(addr ktypes.Address, hash ktypes.Hash) (*guna.StateObject, error)
	CreateDirtyObject(addr ktypes.Address, accType ktypes.AccType) *guna.StateObject
	GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error)
	GetDirtyObject(addr ktypes.Address) (*guna.StateObject, error)
	GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error)
	DeleteStateObject(addr ktypes.Address)
	GetContextByHash(addr ktypes.Address, hash ktypes.Hash) (ktypes.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(id ...id.KramaID) (keys [][]byte, err error)
	Cleanup(addrs ktypes.Address)
	IsGenesis(addr ktypes.Address) (bool, error)
	FetchContextLock(ts *ktypes.Tesseract) (*ktypes.ICSNodes, error)
}

type server interface {
	GetKramaID() id.KramaID
	Broadcast(topic string, data []byte) error
	Unsubscribe(topic string) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}

type ixpool interface {
	ResetWithHeaders(ts *ktypes.Tesseract)
}

type executor interface {
	ExecuteInteractions(
		clusterID ktypes.ClusterID,
		ixs []*ktypes.Interaction,
		contextDelta ktypes.ContextDelta,
	) (ktypes.Receipts, error)
	Revert(clusterID ktypes.ClusterID) error
}

type ChainManager struct {
	ctx                 context.Context
	cfg                 *common.ChainConfig
	db                  db
	mux                 *kutils.TypeMux
	ixpool              ixpool
	tesseracts          *lru.Cache
	orphanTesseracts    *lru.Cache
	gridsCache          *GridCache
	sm                  stateManager
	validatedTesseracts sync.Map
	latticeLocks        *locker.Locker
	logger              hclog.Logger
	knownTesseracts     *ktypes.KnownCache
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
	mux *kutils.TypeMux,
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
		knownTesseracts:  ktypes.NewKnownCache(150),
		latticeLocks:     locker.New(),
		logger:           logger.Named("Chain-Manager"),
		senatus:          senatus,
		metrics:          metrics,
	}

	return c, nil
}

func (c *ChainManager) hasTesseract(hash ktypes.Hash) bool {
	exists, err := c.db.Contains(hash.Bytes())
	if err != nil {
		c.logger.Error("Failed to fetch hash from db", "error", err)

		return true
	}

	return exists
}

func (c *ChainManager) fetchContextForAgora(t *ktypes.Tesseract) ([]id.KramaID, error) {
	tesseractHash := t.Hash()
	address := t.Address()
	peers := make([]id.KramaID, 0)

	for tesseractHash != ktypes.NilHash {
		if len(peers) >= 10 {
			break
		}

		ts, err := c.GetTesseract(tesseractHash)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}
		// fetch the context delta
		deltaGroup := ts.Body.ContextDelta[address]
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(ktypes.NilAddress, ts.Header.ContextLock[address].ContextHash)
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
	ts *ktypes.Tesseract,
	info *ktypes.ICSClusterInfo,
) (*ktypes.ICSNodes, error) {
	nodeSets, err := c.sm.FetchContextLock(ts)
	if err != nil {
		return nil, err
	}

	randomKeys, err := c.sm.GetPublicKeys(ktypes.ToKIPPeerID(info.RandomSet)...)
	if err != nil {
		return nil, err
	}

	nodeSets.UpdateNodeSet(ktypes.RandomSet, ktypes.NewNodeSet(ktypes.ToKIPPeerID(info.RandomSet), randomKeys))

	return nodeSets, nil
}

func (c *ChainManager) AddKnownHashes(tesseracts []*ktypes.Tesseract) {
	for _, v := range tesseracts {
		c.knownTesseracts.Add(v.Hash())
	}
}

func (c *ChainManager) UpdateNodeInclusivity(delta ktypes.ContextDelta) error {
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

func (c *ChainManager) GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	tesseractData, isCached := c.tesseracts.Get(hash)
	if !isCached {
		tesseract := new(ktypes.Tesseract)

		buf, err := c.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, errors.Wrap(err, ktypes.ErrFetchingTesseract.Error())
		}

		if err = polo.Depolorize(tesseract, buf); err != nil {
			return nil, errors.Wrap(err, "failed to depolarize tesseract")
		}

		c.tesseracts.Add(hash, tesseract)

		return tesseract, nil
	}

	tesseract, ok := tesseractData.(*ktypes.Tesseract)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return tesseract, nil
}

func (c *ChainManager) GetTesseractByHeight(address string, height uint64) (*ktypes.Tesseract, error) {
	addressHeightKey := kutils.GetAddressHeightKey(ktypes.HexToAddress(address), height)
	tesseractHash, err := c.db.ReadEntry(addressHeightKey)

	if err != nil {
		return nil, ktypes.ErrFetchingTesseractHash
	}

	return c.GetTesseract(ktypes.BytesToHash(tesseractHash))
}

func (c *ChainManager) GetAssetDataByAssetHash(assetHash []byte) (*ktypes.AssetData, error) {
	rawData, err := c.db.ReadEntry(assetHash)
	if err != nil {
		return nil, err
	}

	assetData := new(ktypes.AssetData)

	if err := polo.Depolorize(assetData, rawData); err != nil {
		return nil, err
	}

	return assetData, nil
}

func (c *ChainManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	if addr == ktypes.NilAddress {
		return nil, ktypes.ErrInvalidAddress
	}

	return c.sm.GetLatestTesseract(addr)
}

func (c *ChainManager) getReceipt(ixHash, receiptRoot ktypes.Hash) (*ktypes.Receipt, error) {
	rawData, err := c.db.ReadEntry(receiptRoot.Bytes())
	if err != nil {
		c.logger.Error("Error fetching receipt root", "error", err.Error(), receiptRoot.Hex(), ixHash.Hex())

		return nil, err
	}

	receipts := new(ktypes.Receipts)

	if err := polo.Depolorize(receipts, rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error) {
	ts, err := c.GetLatestTesseract(addr)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrReceiptNotFound, err.Error())
	}

	for ts.Header.PrevHash != ktypes.NilHash {
		for _, ix := range ts.Interactions() {
			if ix.GetIxHash() == ixHash {
				return c.getReceipt(ix.GetIxHash(), ts.Body.ReceiptHash)
			}
		}

		previousTesseract, err := c.GetTesseract(ts.Header.PrevHash)
		if err != nil {
			return nil, errors.Wrap(ktypes.ErrReceiptNotFound, err.Error())
		}

		ts = previousTesseract
	}

	return nil, ktypes.ErrReceiptNotFound
}

func (c *ChainManager) isSealValid(ts *ktypes.Tesseract, id id.KramaID) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(id)
	if err != nil {
		c.logger.Error("Error fetching public key", "err", err)

		return false, err
	}

	return mudra.Verify(ts.Bytes(), ts.Seal, publicKey[0])
}

func (c *ChainManager) verifySignatures(ts *ktypes.Tesseract, ics *ktypes.ICSNodes) (bool, error) {
	verificationInitTime := time.Now()
	publicKeys := make([][]byte, 0, ts.Header.Extra.VoteSet.TrueIndicesSize())
	votesCounter := make([]int, 5) //Only 5 because we don't consider observer nodes vote

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
		return false, ktypes.ErrQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   ktypes.PRECOMMIT,
		Round:  ts.Header.Extra.Round,
		GridID: ts.Header.Extra.GridID,
	}

	verified, err := mudra.VerifyAggregateSignature(polo.Polorize(vote), ts.Header.Extra.CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	c.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (c *ChainManager) verifyHeaders(ts *ktypes.Tesseract) error {
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
		parent, err := c.GetTesseract(ts.Header.PrevHash)
		if err != nil {
			c.logger.Error("Failed to fetch parent tesseract", "error", err)

			return ktypes.ErrFetchingTesseract
		}

		// Check Heights
		if parent.Header.Height != ts.Header.Height-1 {
			return ktypes.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Header.Timestamp < parent.Header.Timestamp {
			return ktypes.ErrInvalidBlockTime
		}
	}

	return nil
}

func (c *ChainManager) addTesseract(
	cache bool,
	addr ktypes.Address,
	t *ktypes.Tesseract,
	stateExists,
	tesseractExists bool,
) error {
	var accType ktypes.AccType

	tesseractHash := t.Hash()
	latticeExists := true

	if exists, err := c.db.Contains(t.Header.PrevHash.Bytes()); !exists || err != nil {
		latticeExists = false
	}

	if stateExists {
		stateObject, err := c.sm.GetDirtyObject(addr)
		if err != nil {
			return err
		}

		accType = stateObject.GetAccountType()

		for k, v := range stateObject.GetDirtyStorage() {
			if err := c.db.CreateEntry(k.Bytes(), v); err != nil {
				c.logger.Error("Error writing key to db", "key", k.Hex())

				return err
			}
		}
	}

	if err := c.db.CreateEntry(tesseractHash.Bytes(), polo.Polorize(t)); err != nil {
		return errors.Wrap(err, "error writing tesseract to db")
	}

	addressHeightKey := kutils.GetAddressHeightKey(addr, t.Height())
	if err := c.db.CreateEntry(addressHeightKey, tesseractHash.Bytes()); err != nil {
		return errors.Wrap(err, "error writing addressHeightKey to db")
	}

	bucketNo, updateCount, err := c.db.UpdateAccMetaInfo(
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

	// update peer occupancy metrics
	if err := c.UpdateNodeInclusivity(t.ContextDelta()); err != nil {
		return errors.Wrap(err, ktypes.ErrUpdatingInclusivity.Error())
	}

	if updateCount != -1 && bucketNo != -1 {
		if err := c.mux.Post(kutils.SyncStatusUpdate{BucketID: bucketNo, Count: updateCount}); err != nil {
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
	clusterInfo *ktypes.ICSClusterInfo,
	dirtyStorage map[ktypes.Hash][]byte,
	tesseracts ...*ktypes.Tesseract,
) error {
	tsAdditionInitTime := time.Now()

	for _, ts := range tesseracts {
		if err := func(addr ktypes.Address, ts *ktypes.Tesseract) error {
			c.latticeLocks.Lock(ts.Hash().Hex())
			defer func() {
				if err := c.latticeLocks.Unlock(ts.Hash().Hex()); err != nil {
					c.logger.Error("failed to unlock lattice", "error", err, "addr", addr)
				}

				c.validatedTesseracts.Delete(ts.Hash())
			}()

			if c.hasTesseract(ts.Hash()) {
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
		if err := c.db.CreateEntry(
			tesseracts[0].Body.ConsensusProof.ICSHash.Bytes(),
			polo.Polorize(clusterInfo),
		); err != nil {
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
	clusterInfo *ktypes.ICSClusterInfo,
	tesseracts ...*ktypes.Tesseract,
) error {
	for _, ts := range tesseracts {
		if err := func(addr ktypes.Address, ts *ktypes.Tesseract) error {
			c.latticeLocks.Lock(ts.Hash().Hex())
			defer func() {
				if err := c.latticeLocks.Unlock(ts.Hash().Hex()); err != nil {
					c.logger.Error("failed to unlock lattice", "error", err, "addr", addr)
				}

				c.validatedTesseracts.Delete(ts.Hash())
			}()

			if c.hasTesseract(ts.Hash()) {
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
		if err := c.db.CreateEntry(
			tesseracts[0].Body.ConsensusProof.ICSHash.Bytes(),
			polo.Polorize(clusterInfo),
		); err != nil {
			return errors.Wrap(err, "failed to write cluster info to db")
		}
	}

	return nil
}

func (c *ChainManager) validateTesseract(sender id.KramaID, ts *ktypes.Tesseract, ics *ktypes.ICSNodes) error {
	c.latticeLocks.Lock(ts.Hash().Hex())
	defer func() {
		if err := c.latticeLocks.Unlock(ts.Hash().Hex()); err != nil {
			c.logger.Error("failed to unlock lattice", "error", err, "addr", ts.Address())
		}
	}()

	_, ok := c.validatedTesseracts.Load(ts.Hash())
	if ok || c.hasTesseract(ts.Hash()) {
		return ktypes.ErrAlreadyKnown
	}

	validSeal, err := c.isSealValid(ts, sender)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal ", "err", err)

		return ktypes.ErrInvalidSeal
	}

	if err = c.verifyHeaders(ts); err != nil {
		if errors.Is(err, ktypes.ErrFetchingTesseract) {
			c.orphanTesseracts.Add(ts.Hash(), ts)
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
	ts *ktypes.Tesseract,
	sender id.KramaID,
	clusterInfo *ktypes.ICSClusterInfo,
) error {
	log.Println("Adding gossiped tesseract", ts.Hash(), ts.Address())

	if c.hasTesseract(ts.Hash()) {
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

	if c.cfg.ShouldExecute && index == -1 { //TODO: Should execute if validator responded false to operator request
		if isGridComplete := c.gridsCache.AddTesseract(ts); !isGridComplete {
			return nil
		}

		var tesseractGrid []*ktypes.Tesseract

		if ts.GridLength() == 1 {
			tesseractGrid = []*ktypes.Tesseract{ts}
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

	c.logger.Info("Added tesseract without state", "Addr", ts.Header.Address.Hex(), "Hash", ts.Hash().Hex())
	c.sendTesseractSyncRequest(ts, clusterInfo)

	return nil
}

func (c *ChainManager) AddTesseracts(tesseracts []*ktypes.Tesseract, dirtyStorage map[ktypes.Hash][]byte) error {
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
	var contextDelta ktypes.ContextDelta

	stateObject := c.sm.CreateDirtyObject(guna.GenesisAddress, ktypes.SargaAccount)

	for _, info := range accounts {
		addr := ktypes.HexToAddress(info.Address)
		if addr == guna.GenesisAddress {
			bContext := ktypes.ToKIPPeerID(info.BehaviourContext)
			rContext := ktypes.ToKIPPeerID(info.RandomContext)
			contextDelta = map[ktypes.Address]*ktypes.DeltaGroup{
				addr: {
					BehaviouralNodes: bContext,
					RandomNodes:      rContext,
				},
			}

			if _, err := stateObject.CreateContext(bContext, rContext); err != nil {
				return errors.New("context initiation failed in genesis")
			}
		} else {
			stateObject.AddAccountGenesisInfo(addr, GenesisIxHash)
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return err
	}

	tesseract := CreateGenesisTesseract(guna.GenesisAddress, stateHash, stateObject.GetContextHash(), contextDelta)

	if err := c.AddGenesis(guna.GenesisAddress, tesseract); err != nil {
		c.logger.Error("Error adding genesis", "err", err)

		return errors.New("error adding genesis tesseract ")
	}

	return nil
}

func (c *ChainManager) IsGenesisAt(address ktypes.Address, hash ktypes.Hash) (bool, error) {
	ts, err := c.GetTesseract(hash)
	if err != nil {
		return false, err
	}

	object, err := c.sm.GetStateObjectByHash(ts.Header.Address, ts.Body.StateHash)
	if err != nil {
		return false, err
	}

	_, err = object.GetStorageEntry(ktypes.GetHash(address.Bytes()))
	if err != nil {
		return true, nil
	}

	return false, nil
}

func (c *ChainManager) AddGenesis(addr ktypes.Address, t *ktypes.Tesseract) error {
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
		return ktypes.ErrGenesisSetupFailed
	}

	if err = json.Unmarshal(data, genesisAccounts); err != nil {
		return err
	}

	if err = c.setupSargaAccount(genesisAccounts.Accounts); err != nil {
		return ktypes.ErrGenesisSetupFailed
	}

	for _, v := range genesisAccounts.Accounts {
		addr := ktypes.HexToAddress(v.Address)
		if addr == guna.GenesisAddress {
			continue
		}

		// Update the context
		stateObject := c.sm.CreateDirtyObject(addr, v.AccType)
		bContext := ktypes.ToKIPPeerID(v.BehaviourContext)
		rContext := ktypes.ToKIPPeerID(v.RandomContext)

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
				stateObject.AddBalance(ktypes.AssetID(balance.AssetID), new(big.Int).SetInt64(balance.Amount))
			}
		}

		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		contextDelta := map[ktypes.Address]*ktypes.DeltaGroup{
			addr: {
				BehaviouralNodes: bContext,
				RandomNodes:      rContext,
			},
		}

		tesseract := CreateGenesisTesseract(addr, stateHash, stateObject.GetContextHash(), contextDelta)

		if err := c.AddGenesis(addr, tesseract); err != nil {
			return errors.New("error adding genesis tesseract ")
		}
	}

	return nil
}

func (c *ChainManager) sendTesseractSyncRequest(ts *ktypes.Tesseract, clusterInfo *ktypes.ICSClusterInfo) {
	// fetch context info for agora
	fetchContext, err := c.fetchContextForAgora(ts)
	if err != nil {
		c.logger.Error("Error fetching fetchContext for agora", "error", err)
	}

	// add ics random nodes to agora context
	fetchContext = append(fetchContext, ktypes.ToKIPPeerID(clusterInfo.RandomSet)...)

	event := kutils.TesseractSyncEvent{Tesseract: ts, Context: fetchContext}
	if err := c.mux.Post(event); err != nil {
		log.Panic(err)
	}
}

func (c *ChainManager) tesseractHandler(pubSubMsg *pubsub.Message) error {
	msg := new(ktypes.TesseractMessage)
	tsAdditionInitTime := time.Now()
	//	v1msg := proto.MessageV1(msg)
	if err := polo.Depolorize(msg, pubSubMsg.GetData()); err != nil {
		log.Panic(err)
	}

	ts := msg.Tesseract
	//TODO:Add logic to avoid posting event if validator is part of the tesseract already

	if ts.Operator() == string(c.network.GetKramaID()) {
		return nil
	}

	if exists, err := c.db.Contains(ts.Hash().Bytes()); err != nil || exists {
		c.logger.Info("Skipping tesseract", "hash", ts.Hash())

		return nil
	}

	clusterInfo := new(ktypes.ICSClusterInfo)
	if err := polo.Depolorize(clusterInfo, msg.Delta[msg.Tesseract.GetICSHash()]); err != nil {
		c.logger.Error("Error depolarising cluster info", "err", err)
	}

	c.logger.Trace("Tesseract Received from", "Sender", msg.Sender,
		"Hash", msg.Tesseract.Hash().Hex(),
		"Address", msg.Tesseract.Header.Address.Hex())

	if !c.knownTesseracts.Contains(ts.Hash()) {
		c.knownTesseracts.Add(ts.Hash())

		if err := c.AddTesseractWithOutState(ts, msg.Sender, clusterInfo); err != nil {
			c.logger.Error("Error adding tesseract ", "error", err, "addr", ts.Address(), "hash", ts.Hash())
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

func (c *ChainManager) executeAndValidate(ts []*ktypes.Tesseract) (map[ktypes.Hash][]byte, error) {
	clusterID := ts[0].ClusterID()
	contextDelta := ts[0].ContextDelta()
	ixs := ts[0].Interactions()
	dirtyStorage := make(map[ktypes.Hash][]byte)

	receipts, err := c.exec.ExecuteInteractions(clusterID, ixs, contextDelta)
	if err != nil {
		return nil, err
	}

	if !isReceiptAndGroupHashValid(ts, receipts) || !areStateHashesValid(ts, receipts) {
		if err = c.exec.Revert(clusterID); err != nil {
			c.logger.Error("Failed to revert the execution changes", "cluster-id", clusterID)
		}

		return nil, errors.New("failed to validate the tesseract")
	}

	dirtyStorage[receipts.Hash()] = polo.Polorize(receipts)

	return dirtyStorage, nil
}

func areStateHashesValid(tesseracts []*ktypes.Tesseract, receipts ktypes.Receipts) bool {
	for _, ix := range tesseracts[0].Interactions() {
		receipt, err := receipts.GetReceipt(ix.GetIxHash())
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

func isReceiptAndGroupHashValid(tesseracts []*ktypes.Tesseract, receipts ktypes.Receipts) bool {
	groupHash := tesseracts[0].Header.GridHash
	for _, ts := range tesseracts {
		if ts.ReceiptHash() != receipts.Hash() || ts.Header.GridHash != groupHash {
			return false
		}
	}

	return true
}
