package lattice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/sarvalabs/moichain/jug"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type db interface {
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id types.Address,
		height uint64,
		tesseractHash types.Hash,
		accType types.AccountType,
		latticeExists bool,
		tesseractExists bool,
	) (int32, bool, error)
	GetTesseract(tsHash types.Hash) ([]byte, error)
	SetTesseract(tsHash types.Hash, data []byte) error
	HasTesseract(tsHash types.Hash) bool
	SetInteractions(ixHash types.Hash, data []byte) error
	GetTesseractHeightEntry(addr types.Address, height uint64) ([]byte, error)
	SetTesseractHeightEntry(addr types.Address, height uint64, tsHash types.Hash) error
	SetReceipts(receiptHash types.Hash, data []byte) error
	GetReceipts(receiptHash types.Hash) ([]byte, error)
	GetInteractions(gridHash types.Hash) ([]byte, error)
	SetIXGridLookup(ixHash types.Hash, gridHash types.Hash) error
	GetIXGridLookup(ixHash types.Hash) ([]byte, error)
	SetTesseractParts(gridHash types.Hash, parts []byte) error
	GetTesseractParts(gridHash types.Hash) ([]byte, error)
	SetTSGridLookup(tsHash types.Hash, gridHash types.Hash) error
	GetTSGridLookup(tsHash types.Hash) ([]byte, error)
	GetAccountMetaInfo(id types.Address) (*types.AccountMetaInfo, error)
}

type reputationEngine interface {
	UpdateWalletCount(peerID id.KramaID, delta int32) error
}

type stateManager interface {
	CreateDirtyObject(addr types.Address, accType types.AccountType) *guna.StateObject
	GetDirtyObject(addr types.Address) (*guna.StateObject, error)
	FlushDirtyObject(addrs types.Address) error
	GetAccTypeUsingStateObject(address types.Address) (types.AccountType, error)
	GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error)
	GetContextByHash(addr types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(id ...id.KramaID) (keys [][]byte, err error)
	Cleanup(addrs types.Address)
	IsAccountRegistered(addr types.Address) (bool, error)
	IsAccountRegisteredAt(addr types.Address, tesseractHash types.Hash) (bool, error)
	FetchContextLock(ts *types.Tesseract) (*types.ICSNodeSet, error)
	FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
	GetNodeSet(ids []id.KramaID) (*types.NodeSet, error)
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
	SpawnExecutor(fuelLimit uint64) *jug.IxExecutor
}

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type ChainManager struct {
	ctx               context.Context
	cfg               *common.ChainConfig
	db                db
	mux               *utils.TypeMux
	ixpool            ixpool
	tesseracts        *lru.Cache
	orphanTesseracts  *lru.Cache
	sm                stateManager
	latticeLocks      *locker.Locker
	logger            hclog.Logger
	knownTesseracts   *types.HashRegistry
	senatus           reputationEngine
	network           server
	exec              executor
	metrics           *Metrics
	signatureVerifier AggregatedSignatureVerifier
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
		knownTesseracts:   types.NewHashRegistry(150),
		latticeLocks:      locker.New(),
		logger:            logger.Named("Chain-Manager"),
		senatus:           senatus,
		metrics:           metrics,
		signatureVerifier: verifier,
	}

	return c, nil
}

func (c *ChainManager) hasTesseract(tsHash types.Hash) bool {
	return c.db.HasTesseract(tsHash)
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
		deltaGroup, _ := ts.GetContextDeltaByAddress(address)
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		contextLock, _ := ts.ContextLockByAddress(address)

		_, behaviour, random, err := c.sm.GetContextByHash(address, contextLock.ContextHash)
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.PrevHash().IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.PrevHash(), false)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}

		ts = *t
	}

	return peers, nil
}

func (c *ChainManager) FetchICSNodeSet(
	ts *types.Tesseract,
	info *types.ICSClusterInfo,
) (*types.ICSNodeSet, error) {
	icsNodeSets, err := c.sm.FetchContextLock(ts)
	if err != nil {
		return nil, err
	}

	if info.Responses == nil {
		return nil, errors.New("nil responses slice")
	}

	for index, set := range icsNodeSets.Nodes {
		if set != nil && info.Responses[index] != nil {
			set.Responses = info.Responses[index]
		}
	}

	randomSet, err := c.sm.GetNodeSet(info.RandomSet)
	if err != nil {
		return nil, err
	}

	icsNodeSets.UpdateNodeSet(types.RandomSet, randomSet)

	observerSet, err := c.sm.GetNodeSet(info.ObserverSet)
	if err != nil {
		return nil, err
	}

	icsNodeSets.UpdateNodeSet(types.ObserverSet, observerSet)

	return icsNodeSets, nil
}

func (c *ChainManager) AddKnownHashes(tesseracts []*types.Tesseract) {
	for _, v := range tesseracts {
		c.knownTesseracts.Add(v.Hash())
	}
}

func (c *ChainManager) UpdateNodeInclusivity(delta types.ContextDelta) error {
	for _, deltaGroup := range delta {
		for _, kramaID := range deltaGroup.BehaviouralNodes {
			if err := c.senatus.UpdateWalletCount(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.RandomNodes {
			if err := c.senatus.UpdateWalletCount(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.ReplacedNodes {
			if err := c.senatus.UpdateWalletCount(kramaID, -1); err != nil {
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

func (c *ChainManager) GetTesseractHeightEntry(address types.Address, height uint64) (types.Hash, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return types.BytesToHash(tesseractHash), nil
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

func (c *ChainManager) getReceipt(ixHash, gridHash types.Hash) (*types.Receipt, error) {
	rawData, err := c.db.GetReceipts(gridHash)
	if err != nil {
		c.logger.Error("Error fetching receipts ", "error", err.Error(), gridHash, ixHash)

		return nil, err
	}

	receipts := new(types.Receipts)

	if err = receipts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceiptByIxHash(ixHash types.Hash) (*types.Receipt, error) {
	rawData, err := c.db.GetIXGridLookup(ixHash)
	if err != nil {
		return nil, errors.Wrap(err, "grid hash not found")
	}

	receipt, err := c.getReceipt(ixHash, types.BytesToHash(rawData))
	if err != nil {
		return nil, errors.Wrap(err, "receipt not found")
	}

	return receipt, nil
}

func (c *ChainManager) isSealValid(ts *types.Tesseract) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(ts.Sealer())
	if err != nil {
		c.logger.Error("Error fetching public key", "err", err)

		return false, err
	}

	rawData, err := ts.Bytes()
	if err != nil {
		return false, err
	}

	return mudra.Verify(rawData, ts.Seal(), publicKey[0])
}

func (c *ChainManager) verifySignatures(ts *types.Tesseract, ics *types.ICSNodeSet) (bool, error) {
	var (
		verificationInitTime = time.Now()
		publicKeys           = make([][]byte, 0, ts.Extra().VoteSet.TrueIndicesSize())
		votesCounter         = make([]int, 5) // Only 5 because we don't consider observer nodes vote
	)

	for _, index := range ts.Extra().VoteSet.GetTrueIndices() {
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
		Round:  ts.Extra().Round,
		GridID: ts.Extra().GridID,
	}

	rawData, err := vote.Bytes()
	if err != nil {
		return false, err
	}

	verified, err := c.signatureVerifier(rawData, ts.Extra().CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	c.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (c *ChainManager) verifyHeaders(ts *types.Tesseract) error {
	c.logger.Debug("Verifying headers", "addr", ts.Address(), "height", ts.Height())

	if ts.ClusterID() == "genesis" {
		return nil
	}

	initial, err := c.IsInitialTesseract(ts)
	if err != nil {
		return errors.Wrap(err, "Sarga account not found")
	}

	if !initial {
		parent, err := c.GetTesseract(ts.PrevHash(), false)
		if err != nil {
			c.logger.Error("Failed to fetch parent tesseract", "error", err, ts.Address())

			return types.ErrPreviousTesseractNotFound
		}

		// Check Heights
		if parent.Height() != ts.Height()-1 {
			return types.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Timestamp() < parent.Timestamp() {
			return types.ErrInvalidBlockTime
		}
	}

	return nil
}

func (c *ChainManager) storeReceipts(ts *types.Tesseract) error {
	if ts.HasReceipts() {
		rawReceipts, err := ts.Receipts().Bytes()
		if err != nil {
			return err
		}

		gridHash := ts.GridHash()

		_, err = c.db.GetReceipts(gridHash)
		if errors.Is(err, types.ErrKeyNotFound) {
			if err = c.db.SetReceipts(gridHash, rawReceipts); err != nil {
				return errors.Wrap(err, "error writing receipts to db")
			}

			return nil
		}

		return err
	}

	return nil
}

// The storeInteractions function uses grid hash as a key to store interactions.
// It also stores key-value pairs of ix hash and grid hash,
// as well as key-value pairs of grid hash and tesseract parts.
func (c *ChainManager) storeInteractions(ts *types.Tesseract) error {
	if ts.Interactions() != nil {
		ixRawData, err := ts.Interactions().Bytes()
		if err != nil {
			return err
		}

		gridHash := ts.GridHash()

		_, err = c.db.GetInteractions(gridHash)
		if errors.Is(err, types.ErrKeyNotFound) {
			if err = c.db.SetInteractions(gridHash, ixRawData); err != nil {
				return errors.Wrap(
					err,
					fmt.Sprintf("error writing interactions to db with grid-hash %s", gridHash))
			}

			for _, ix := range ts.Interactions() {
				c.logger.Debug(
					"Storing ix grid lookup",
					"address", ts.Address(),
					"ix-hash", ix.Hash(),
					"grid-hash", gridHash)

				if err = c.db.SetIXGridLookup(ix.Hash(), gridHash); err != nil {
					return errors.Wrap(
						err,
						fmt.Sprintf("error writing gridID hash to db ix-hash %s grid-hash %s",
							ix.Hash(),
							gridHash,
						))
				}
			}

			parts, err := ts.Parts()
			if err != nil {
				return err
			}

			rawPartsData, err := parts.Bytes()
			if err != nil {
				return err
			}

			if err = c.db.SetTesseractParts(gridHash, rawPartsData); err != nil {
				return errors.Wrap(err, "error writing tesseract parts to db")
			}

			return nil
		}

		return err
	}

	return nil
}

func (c *ChainManager) addTesseract(
	cache bool,
	addr types.Address,
	t *types.Tesseract,
	tesseractExists bool,
) error {
	var (
		accType       types.AccountType
		err           error
		latticeExists bool
	)

	defer func() {
		c.sm.Cleanup(t.Address())
	}()

	latticeExists = c.db.HasTesseract(t.PrevHash())

	if err = c.sm.FlushDirtyObject(addr); err != nil {
		return err
	}

	accType, err = c.sm.GetAccTypeUsingStateObject(addr)
	if err != nil {
		return errors.Wrap(err, "failed to fetch account type")
	}

	tsRawData, err := t.Canonical().Bytes()
	if err != nil {
		return err
	}

	if t.ClusterID() != GenesisIdentifier {
		gridHash := t.GridHash()

		if err := c.db.SetTSGridLookup(t.Hash(), gridHash); err != nil {
			return errors.Wrap(err, "failed to set grid look up")
		}
	}

	if err = c.storeInteractions(t); err != nil {
		return errors.Wrap(err, "failed to store interactions")
	}

	if err = c.storeReceipts(t); err != nil {
		return errors.Wrap(err, "failed to store receipts")
	}

	if err = c.db.SetTesseract(t.Hash(), tsRawData); err != nil {
		return errors.Wrap(err, "error writing tesseract to db")
	}

	if err = c.db.SetTesseractHeightEntry(addr, t.Height(), t.Hash()); err != nil {
		return errors.Wrap(err, "failed to write tesseract height entry")
	}

	_, _, err = c.db.UpdateAccMetaInfo(
		addr,
		t.Height(),
		t.Hash(),
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

	if cache {
		c.tesseracts.Add(addr, t.Hash())
		c.tesseracts.Add(t.Hash(), t.GetTesseractWithoutIxns())
	}

	c.logger.Info("Tesseract Added", "addr", addr, "hash", t.Hash(), "sealer", t.Sealer())

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

	if _, ok := dirtyStorage[tesseracts[0].ICSHash()]; !ok {
		return errors.New("cluster info not found")
	}

	for _, ts := range tesseracts {
		if err := func(addr types.Address, ts *types.Tesseract) error {
			tsHash := ts.Hash()

			c.latticeLocks.Lock(tsHash.Hex())

			defer func() {
				if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
					c.logger.Error("failed to unlock lattice", "error", err, "addr", addr)
				}
			}()

			if c.hasTesseract(tsHash) {
				return nil
			}

			if err := c.addTesseract(true, addr, ts, true); err != nil {
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

func (c *ChainManager) ValidateTesseract(ts *types.Tesseract, ics *types.ICSNodeSet) error {
	tsHash := ts.Hash()

	c.latticeLocks.Lock(tsHash.Hex())
	defer func() {
		if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
			c.logger.Error("failed to unlock lattice", "error", err, "addr", ts.Address())
		}
	}()

	if c.hasTesseract(tsHash) {
		return types.ErrAlreadyKnown
	}

	validSeal, err := c.isSealValid(ts)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal ", "err", err)

		return types.ErrInvalidSeal
	}

	if err = c.verifyHeaders(ts); err != nil {
		switch {
		case errors.Is(err, types.ErrPreviousTesseractNotFound):
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

func (c *ChainManager) AddTesseracts(dirtyStorage map[types.Hash][]byte, tesseracts ...*types.Tesseract) error {
	if len(tesseracts) == 0 {
		return errors.New("nil grid")
	}

	if len(dirtyStorage) == 0 {
		return errors.New("empty dirty storage")
	}

	groupHash := tesseracts[0].GroupHash()
	for _, ts := range tesseracts {
		if ts.GroupHash() != groupHash {
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

func (c *ChainManager) IsInitialTesseract(ts *types.Tesseract) (bool, error) {
	var (
		accountRegistered bool
		err               error
	)

	if info, ok := ts.ContextLock()[types.SargaAddress]; !ok {
		accountRegistered, err = c.sm.IsAccountRegistered(ts.Address())
	} else {
		c.logger.Debug(
			"Checking for new account ",
			"addr", ts.Address(),
			"at height", info.Height,
			"hash", info.TesseractHash,
		)
		accountRegistered, err = c.sm.IsAccountRegisteredAt(ts.Address(), info.TesseractHash)
	}

	return !accountRegistered, err
}

func (c *ChainManager) AddGenesisTesseract(
	address types.Address,
	stateHash, contextHash types.Hash,
	contextDelta types.ContextDelta,
) error {
	tesseract, err := createGenesisTesseract(address, stateHash, contextHash, contextDelta)
	if err != nil {
		return err
	}

	if err = c.addTesseract(true, address, tesseract, true); err != nil {
		return errors.Wrap(err, "error adding genesis tesseract")
	}

	return nil
}

func (c *ChainManager) SetupGenesis(path string) error {
	sargaAccount, genesisAccounts, logicPaths, err := c.ParseGenesisFile(path)
	if err != nil {
		return err
	}

	if _, err = c.db.GetAccountMetaInfo(sargaAccount.Address); err == nil {
		c.logger.Info("!!!!!!....Skipping Genesis....!!!!!!")

		return nil
	}

	stateHash, contextHash, err := c.SetupSargaAccount(sargaAccount, genesisAccounts, logicPaths)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	if err = c.AddGenesisTesseract(
		sargaAccount.Address,
		stateHash,
		contextHash,
		sargaAccount.ContextDelta(),
	); err != nil {
		return err
	}

	for _, v := range genesisAccounts {
		stateHash, contextHash, err = c.SetupNewAccount(v)
		if err != nil {
			return err
		}

		if err = c.AddGenesisTesseract(
			v.Address,
			stateHash,
			contextHash,
			v.ContextDelta(),
		); err != nil {
			return err
		}
	}

	if _, err = c.ExecuteGenesisLogics(logicPaths); err != nil {
		return err
	}

	return nil
}

func (c *ChainManager) ParseGenesisFile(path string) (
	*gtypes.AccountSetupArgs,
	[]*gtypes.AccountSetupArgs,
	[]GenesisLogic,
	error,
) {
	genesisData := new(Genesis)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	sargaAccount, err := c.validateAccountCreationInfo(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	genesisAccounts := make([]*gtypes.AccountSetupArgs, len(genesisData.Accounts))

	for index, accInfo := range genesisData.Accounts {
		genesisAccounts[index], err = c.validateAccountCreationInfo(accInfo)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, fmt.Sprintf("invalid genesis account info %s", accInfo.Address))
		}
	}

	return sargaAccount, genesisAccounts, genesisData.Logics, nil
}

func (c *ChainManager) Start() error {
	return nil
}

func (c *ChainManager) Close() {
	log.Println("Closing Chain manager")
}

func (c *ChainManager) ExecuteAndValidate(tesseracts ...*types.Tesseract) error {
	c.logger.Debug(
		"Executing interactions of grid",
		tesseracts[0].GridHash(),
		"lock", tesseracts[0].Header().ContextLock,
	)

	receipts, err := c.exec.ExecuteInteractions(
		tesseracts[0].ClusterID(),
		tesseracts[0].Interactions(),
		tesseracts[0].ContextDelta(),
	)
	if err != nil {
		return err
	}

	if !isReceiptAndGroupHashValid(tesseracts, receipts) || !areStateHashesValid(tesseracts, receipts) {
		if err = c.exec.Revert(tesseracts[0].ClusterID()); err != nil {
			c.logger.Error("Failed to revert the execution changes", "cluster-id", tesseracts[0].ClusterID())

			return errors.Wrap(err, "failed to revert the execution changes")
		}

		return errors.New("failed to validate the tesseract")
	}

	for _, t := range tesseracts {
		t.SetReceipts(receipts)
	}

	return nil
}

func (c *ChainManager) getTesseractPartsByGridHash(gridHash types.Hash) (*types.TesseractParts, error) {
	parts := &types.TesseractParts{
		Grid: make(map[types.Address]types.TesseractHeightAndHash),
	}

	rawData, err := c.db.GetTesseractParts(gridHash)
	if err != nil {
		return nil, errors.Wrap(err, "tesseract parts not found")
	}

	if err := parts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return parts, nil
}

func (c *ChainManager) getInteractionsByGridHash(gridHash types.Hash) (types.Interactions, error) {
	interactions := new(types.Interactions)

	buf, err := c.db.GetInteractions(gridHash)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching interactions")
	}

	if err := interactions.FromBytes(buf); err != nil {
		return nil, err
	}

	return *interactions, nil
}

// GetInteractionAndPartsByTSHash returns interaction,tesseract parts for the given tesseract hash and ix index
func (c *ChainManager) GetInteractionAndPartsByTSHash(tsHash types.Hash, ixIndex int) (
	*types.Interaction,
	*types.TesseractParts,
	error,
) {
	rawData, err := c.db.GetTSGridLookup(tsHash)
	if err != nil {
		return nil, nil, errors.Wrap(types.ErrGridHashNotFound, err.Error())
	}

	gridHash := types.BytesToHash(rawData)

	interactions, err := c.getInteractionsByGridHash(gridHash)
	if err != nil {
		return nil, nil, err
	}

	if ixIndex >= len(interactions) || ixIndex < 0 {
		return nil, nil, types.ErrIndexOutOfRange
	}

	parts, err := c.getTesseractPartsByGridHash(gridHash)
	if err != nil {
		return nil, nil, err
	}

	return interactions[ixIndex], parts, nil
}

// GetInteractionAndPartsByIxHash returns interaction,tesseract parts, ix index for the given tesseract hash
func (c *ChainManager) GetInteractionAndPartsByIxHash(ixHash types.Hash) (
	*types.Interaction,
	*types.TesseractParts,
	int,
	error,
) {
	rawData, err := c.db.GetIXGridLookup(ixHash)
	if err != nil {
		return nil, nil, 0, errors.Wrap(types.ErrGridHashNotFound, err.Error())
	}

	gridHash := types.BytesToHash(rawData)

	interactions, err := c.getInteractionsByGridHash(gridHash)
	if err != nil {
		return nil, nil, 0, err
	}

	for ixIndex, ix := range interactions {
		if ix.Hash() == ixHash {
			parts, err := c.getTesseractPartsByGridHash(gridHash)
			if err != nil {
				return nil, nil, 0, err
			}

			return ix, parts, ixIndex, nil
		}
	}

	return nil, nil, 0, types.ErrFetchingInteraction
}

func (c *ChainManager) SetupSargaAccount(
	sarga *gtypes.AccountSetupArgs,
	accounts []*gtypes.AccountSetupArgs,
	logics []GenesisLogic,
) (types.Hash, types.Hash, error) {
	if sarga.Address != types.SargaAddress {
		return types.NilHash, types.NilHash, errors.New("invalid sarga account address")
	}

	stateObject := c.sm.CreateDirtyObject(types.SargaAddress, types.SargaAccount)

	if _, err := stateObject.CreateContext(sarga.BehaviouralContext, sarga.RandomContext); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(types.SargaLogicID); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "failed to create storage tree")
	}

	for _, account := range accounts {
		if account.Address != types.SargaAddress {
			// Add account to sarga storage tree
			if err := stateObject.AddAccountGenesisInfo(account.Address, types.GenesisIxHash); err != nil {
				return types.NilHash, types.NilHash, err
			}
		}
	}

	for _, logic := range logics {
		// Add logic account to sarga
		if err := stateObject.AddAccountGenesisInfo(
			types.CreateAddressFromString(logic.Name),
			types.GenesisIxHash,
		); err != nil {
			return types.NilHash, types.NilHash, err
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return types.NilHash, types.NilHash, err
	}

	return stateHash, stateObject.ContextHash(), nil
}

func (c *ChainManager) SetupNewAccount(info *gtypes.AccountSetupArgs) (types.Hash, types.Hash, error) {
	stateObject := c.sm.CreateDirtyObject(info.Address, info.AccType)

	if _, err := stateObject.CreateContext(info.BehaviouralContext, info.RandomContext); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "context initiation failed in genesis")
	}

	if len(info.Assets) > 0 {
		for _, asset := range info.Assets {
			_, err := stateObject.CreateAsset(asset)
			if err != nil {
				return types.NilHash, types.NilHash, errors.Wrap(err, "failed to create an asset")
			}
		}
	}

	if len(info.Balances) > 0 {
		for assetID, balance := range info.Balances {
			stateObject.AddBalance(assetID, balance)
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return types.NilHash, types.NilHash, err
	}

	return stateHash, stateObject.ContextHash(), nil
}

func (c *ChainManager) ExecuteGenesisLogics(logics []GenesisLogic) ([]types.Hash, error) {
	hashes := make([]types.Hash, len(logics))

	for index, logic := range logics {
		logicAddr := types.CreateAddressFromString(logic.Name)

		payload := &types.LogicPayload{
			Callsite: logic.Callsite,
			Calldata: logic.Calldata,
			Manifest: logic.Manifest,
		}

		if !types.Contains(types.GenesisLogicAddrs, logicAddr) {
			c.logger.Error("mismatch of contract address for contract %d", logic.Name)

			return nil, errors.New("generated address does not exist in predefined contract address")
		}

		// Create state object for the logic
		stateObj := c.sm.CreateDirtyObject(logicAddr, types.LogicAccount)

		behaviouralCtx := utils.KramaIDFromString(logic.BehaviouralContext)
		randomCtx := utils.KramaIDFromString(logic.RandomContext)

		contextHash, err := stateObj.CreateContext(behaviouralCtx, randomCtx)
		if err != nil {
			return nil, errors.Wrap(err, "context initiation failed in genesis")
		}

		// Deploy the genesis logic on that state
		logicID, err := jug.DeployGenesisLogic(stateObj, payload)
		if err != nil {
			c.logger.Error("unable to deploy logic for contract %s", logic.Name)

			return nil, errors.Wrap(err, "unable to deploy logic for contract")
		}

		stateHash, err := stateObj.Commit()
		if err != nil {
			return nil, err
		}

		err = c.AddGenesisTesseract(
			logicID.Address(),
			stateHash,
			contextHash,
			map[types.Address]*types.DeltaGroup{
				logicID.Address(): {
					BehaviouralNodes: behaviouralCtx,
					RandomNodes:      randomCtx,
				},
			},
		)

		if err != nil {
			c.logger.Error("unable to create genesis tesseract for cotract %d", logic.Name)

			return nil, errors.Wrap(err, "unable to create genesis tesseract for contract")
		}

		c.logger.Info("deployed genesis contract", "name", logic.Name, "logicID", logicID.Hex())

		hashes[index] = stateHash
	}

	return hashes, nil
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
	groupHash := tesseracts[0].GroupHash()

	receiptsHash, err := receipts.Hash()
	if err != nil {
		return false
	}

	for _, ts := range tesseracts {
		if ts.ReceiptHash() != receiptsHash || ts.GroupHash() != groupHash {
			return false
		}
	}

	return true
}
