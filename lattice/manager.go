package lattice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/pkg/errors"

	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/state"
)

type store interface {
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id common.Address,
		height uint64,
		tesseractHash common.Hash,
		accType common.AccountType,
		latticeExists bool,
		tesseractExists bool,
	) (int32, bool, error)
	GetTesseract(tsHash common.Hash) ([]byte, error)
	SetTesseract(tsHash common.Hash, data []byte) error
	HasTesseract(tsHash common.Hash) bool
	SetInteractions(ixHash common.Hash, data []byte) error
	GetTesseractHeightEntry(addr common.Address, height uint64) ([]byte, error)
	SetTesseractHeightEntry(addr common.Address, height uint64, tsHash common.Hash) error
	SetReceipts(receiptHash common.Hash, data []byte) error
	GetReceipts(receiptHash common.Hash) ([]byte, error)
	GetInteractions(gridHash common.Hash) ([]byte, error)
	SetIXGridLookup(ixHash common.Hash, gridHash common.Hash) error
	GetIXGridLookup(ixHash common.Hash) ([]byte, error)
	SetTesseractParts(gridHash common.Hash, parts []byte) error
	GetTesseractParts(gridHash common.Hash) ([]byte, error)
	SetTSGridLookup(tsHash common.Hash, gridHash common.Hash) error
	GetTSGridLookup(tsHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id common.Address) (*common.AccountMetaInfo, error)
}

type reputationEngine interface {
	UpdateWalletCount(peerID id.KramaID, delta int32) error
}

type stateManager interface {
	CreateDirtyObject(addr common.Address, accType common.AccountType) *state.Object
	GetDirtyObject(addr common.Address) (*state.Object, error)
	FlushDirtyObject(addrs common.Address) error
	GetAccTypeUsingStateObject(address common.Address) (common.AccountType, error)
	GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error)
	GetContextByHash(addr common.Address, hash common.Hash) (common.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(ctx context.Context, id ...id.KramaID) (keys [][]byte, err error)
	Cleanup(addrs common.Address)
	IsAccountRegistered(addr common.Address) (bool, error)
	IsAccountRegisteredAt(addr common.Address, tesseractHash common.Hash) (bool, error)
	FetchContextLock(ts *common.Tesseract) (*common.ICSNodeSet, error)
	FetchTesseractFromDB(hash common.Hash, withInteractions bool) (*common.Tesseract, error)
	GetNodeSet(ids []id.KramaID) (*common.NodeSet, error)
	GetLogicIDs(addr common.Address, hash common.Hash) ([]common.LogicID, error)
}

type server interface {
	GetKramaID() id.KramaID
	Broadcast(topic string, data []byte) error
	Unsubscribe(topic string) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}

type ixpool interface {
	ResetWithHeaders(ts *common.Tesseract)
}

type executor interface {
	ExecuteInteractions(common.Interactions, *common.ExecutionContext) (common.Receipts, error)
	Revert(common.ClusterID) error
	SpawnExecutor() *compute.IxExecutor
	Cleanup(cluster common.ClusterID)
}

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type ChainManager struct {
	cfg               *config.ChainConfig
	db                store
	mux               *utils.TypeMux
	ixpool            ixpool
	tesseracts        *lru.Cache
	orphanTesseracts  *lru.Cache
	sm                stateManager
	latticeLocks      *locker.Locker
	logger            hclog.Logger
	knownTesseracts   *common.HashRegistry
	senatus           reputationEngine
	network           server
	exec              executor
	metrics           *Metrics
	signatureVerifier AggregatedSignatureVerifier
}

func NewChainManager(
	cfg *config.ChainConfig,
	db store,
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
		cfg:               cfg,
		db:                db,
		mux:               mux,
		ixpool:            ix,
		sm:                sm,
		tesseracts:        cache,
		exec:              exec,
		network:           network,
		orphanTesseracts:  orphansCache,
		knownTesseracts:   common.NewHashRegistry(150),
		latticeLocks:      locker.New(),
		logger:            logger.Named("Chain-Manager"),
		senatus:           senatus,
		metrics:           metrics,
		signatureVerifier: verifier,
	}

	return c, nil
}

func (c *ChainManager) hasTesseract(tsHash common.Hash) bool {
	return c.db.HasTesseract(tsHash)
}

func (c *ChainManager) fetchContextForAgora(ts common.Tesseract) ([]id.KramaID, error) {
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

func (c *ChainManager) AddKnownHashes(tesseracts []*common.Tesseract) {
	for _, v := range tesseracts {
		c.knownTesseracts.Add(v.Hash())
	}
}

func (c *ChainManager) UpdateNodeInclusivity(delta common.ContextDelta) error {
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

func (c *ChainManager) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
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

	tesseract, ok := tesseractData.(*common.Tesseract)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return tesseract, nil
}

func (c *ChainManager) GetTesseractByHeight(
	address common.Address,
	height uint64,
	withInteractions bool,
) (*common.Tesseract, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return c.GetTesseract(common.BytesToHash(tesseractHash), withInteractions)
}

func (c *ChainManager) GetTesseractHeightEntry(address common.Address, height uint64) (common.Hash, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return common.BytesToHash(tesseractHash), nil
}

func (c *ChainManager) GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error) {
	if addr.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	return c.sm.GetLatestTesseract(addr, withInteractions)
}

func (c *ChainManager) getReceipt(ixHash, gridHash common.Hash) (*common.Receipt, error) {
	rawData, err := c.db.GetReceipts(gridHash)
	if err != nil {
		c.logger.Error("Error fetching receipts", "err", err.Error(), "grid-hash", gridHash, "ix-hash", ixHash)

		return nil, err
	}

	receipts := new(common.Receipts)

	if err = receipts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	rawData, err := c.db.GetIXGridLookup(ixHash)
	if err != nil {
		return nil, errors.Wrap(err, "grid hash not found")
	}

	receipt, err := c.getReceipt(ixHash, common.BytesToHash(rawData))
	if err != nil {
		return nil, errors.Wrap(err, "receipt not found")
	}

	return receipt, nil
}

func (c *ChainManager) isSealValid(ts *common.Tesseract) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(context.Background(), ts.Sealer())
	if err != nil {
		c.logger.Error("Error fetching the public key", "err", err)

		return false, err
	}

	rawData, err := ts.Bytes()
	if err != nil {
		return false, err
	}

	return crypto.Verify(rawData, ts.Seal(), publicKey[0])
}

func (c *ChainManager) verifySignatures(ts *common.Tesseract, ics *common.ICSNodeSet) (bool, error) {
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
		return false, common.ErrQuorumFailed
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

func (c *ChainManager) verifyHeaders(ts *common.Tesseract) error {
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
			c.logger.Error("Failed to fetch parent tesseract", "err", err, "addr", ts.Address())

			return common.ErrPreviousTesseractNotFound
		}

		// Check Heights
		if parent.Height() != ts.Height()-1 {
			return common.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Timestamp() < parent.Timestamp() {
			return common.ErrInvalidBlockTime
		}
	}

	return nil
}

func (c *ChainManager) storeReceipts(ts *common.Tesseract) error {
	if ts.HasReceipts() {
		rawReceipts, err := ts.Receipts().Bytes()
		if err != nil {
			return err
		}

		gridHash := ts.GridHash()

		_, err = c.db.GetReceipts(gridHash)
		if errors.Is(err, common.ErrKeyNotFound) {
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
func (c *ChainManager) storeInteractions(ts *common.Tesseract) error {
	if ts.Interactions() != nil {
		ixRawData, err := ts.Interactions().Bytes()
		if err != nil {
			return err
		}

		gridHash := ts.GridHash()

		_, err = c.db.GetInteractions(gridHash)
		if errors.Is(err, common.ErrKeyNotFound) {
			if err = c.db.SetInteractions(gridHash, ixRawData); err != nil {
				return errors.Wrap(
					err,
					fmt.Sprintf("error writing interactions to db with grid-hash %s", gridHash))
			}

			for _, ix := range ts.Interactions() {
				c.logger.Debug(
					"Storing interaction grid lookup",
					"addr", ts.Address(),
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
	addr common.Address,
	t *common.Tesseract,
	tesseractExists bool,
) error {
	var (
		accType       common.AccountType
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

	if t.ClusterID() != common.GenesisIdentifier {
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
		c.logger.Error("Error sending tesseract added event", "err", err)
	}

	// update peer occupancy metrics
	if err = c.UpdateNodeInclusivity(t.ContextDelta()); err != nil {
		return errors.Wrap(err, common.ErrUpdatingInclusivity.Error())
	}

	if cache {
		c.tesseracts.Add(addr, t.Hash())
		c.tesseracts.Add(t.Hash(), t.GetTesseractWithoutIxns())
	}

	c.logger.Info("Tesseract added", "addr", addr, "ts-hash", t.Hash(), "sealer", t.Sealer())

	c.ixpool.ResetWithHeaders(t)

	return nil
}

func (c *ChainManager) addTesseractsWithState(
	dirtyStorage map[common.Hash][]byte,
	tesseracts ...*common.Tesseract,
) error {
	tsAdditionInitTime := time.Now()

	if len(tesseracts) == 0 {
		return errors.New("empty tesseracts")
	}

	if _, ok := dirtyStorage[tesseracts[0].ICSHash()]; !ok {
		return errors.New("cluster info not found")
	}

	for _, ts := range tesseracts {
		if err := func(addr common.Address, ts *common.Tesseract) error {
			tsHash := ts.Hash()

			c.latticeLocks.Lock(tsHash.Hex())

			defer func() {
				if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
					c.logger.Error("Failed to unlock lattice", "err", err, "addr", addr)
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

func (c *ChainManager) ValidateTesseract(ts *common.Tesseract, ics *common.ICSNodeSet) error {
	tsHash := ts.Hash()

	c.latticeLocks.Lock(tsHash.Hex())
	defer func() {
		if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
			c.logger.Error("Failed to unlock lattice", "err", err, "addr", ts.Address())
		}
	}()

	if c.hasTesseract(tsHash) {
		return common.ErrAlreadyKnown
	}

	validSeal, err := c.isSealValid(ts)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal", "err", err)

		return common.ErrInvalidSeal
	}

	if err = c.verifyHeaders(ts); err != nil {
		switch {
		case errors.Is(err, common.ErrPreviousTesseractNotFound):
			c.orphanTesseracts.Add(tsHash, ts)

			return common.ErrPreviousTesseractNotFound
		default:
			return err
		}
	}

	verified, err := c.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.New("failed to verify signatures")
	}

	return nil
}

func (c *ChainManager) AddTesseracts(dirtyStorage map[common.Hash][]byte, tesseracts ...*common.Tesseract) error {
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

func (c *ChainManager) validateAccountCreationInfo(accs ...common.AccountSetupArgs) error {
	for _, acc := range accs {
		if acc.Address == common.SargaAddress {
			return common.ErrInvalidAddress
		}
		// check for address validity
		err := utils.ValidateAccountType(acc.AccType)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("invalid genesis account creation info %s", acc.Address))
		}
	}

	return nil
}

func (c *ChainManager) validateSargaAccountCreationInfo(acc common.AccountSetupArgs) error {
	if acc.Address != common.SargaAddress {
		return common.ErrInvalidAddress
	}

	return nil
}

func (c *ChainManager) validateAssetAccountCreationArgs(assetAccounts ...common.AssetAccountSetupArgs) error {
	for _, acc := range assetAccounts {
		if len(acc.AssetInfo.Allocations) == 0 {
			return errors.New("empty allocations")
		}
	}

	return nil
}

func (c *ChainManager) validateLogicCreationArgs(logicAccounts ...common.LogicSetupArgs) error {
	for _, acc := range logicAccounts {
		if len(acc.Manifest) == 0 {
			return errors.New("invalid manifest")
		}
	}

	return nil
}

func (c *ChainManager) IsInitialTesseract(ts *common.Tesseract) (bool, error) {
	var (
		accountRegistered bool
		err               error
	)

	if info, ok := ts.ContextLock()[common.SargaAddress]; !ok {
		accountRegistered, err = c.sm.IsAccountRegistered(ts.Address())
	} else {
		c.logger.Debug(
			"Checking for new account",
			"addr", ts.Address(),
			"height", info.Height,
			"ts-hash", info.TesseractHash,
		)
		accountRegistered, err = c.sm.IsAccountRegisteredAt(ts.Address(), info.TesseractHash)
	}

	return !accountRegistered && ts.Height() == 0, err
}

func (c *ChainManager) AddGenesisTesseract(
	address common.Address,
	stateHash, contextHash common.Hash,
) error {
	tesseract, err := createGenesisTesseract(address, stateHash, contextHash)
	if err != nil {
		return err
	}

	if err = c.addTesseract(true, address, tesseract, true); err != nil {
		return errors.Wrap(err, "error adding genesis tesseract")
	}

	return nil
}

func (c *ChainManager) SetupGenesis(path string) error {
	dirtyObjects := make(map[common.Address]*state.Object)

	sargaAccount, genesisAccounts, assetAccounts, logics, err := c.ParseGenesisFile(path)
	if err != nil {
		return errors.Wrap(err, "failed to parse genesis file")
	}

	if _, err = c.db.GetAccountMetaInfo(sargaAccount.Address); err == nil {
		c.logger.Info("!!!!!!....Skipping Genesis....!!!!!!")

		return nil
	}

	sargaObject, err := c.SetupSargaAccount(sargaAccount, genesisAccounts, assetAccounts, logics)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	dirtyObjects[sargaObject.Address()] = sargaObject

	for _, v := range genesisAccounts {
		if dirtyObjects[v.Address], err = c.SetupNewAccount(v); err != nil {
			return errors.Wrap(err, "failed to setup genesis account")
		}
	}

	if _, err = c.SetupGenesisLogics(dirtyObjects, logics); err != nil {
		return errors.Wrap(err, "failed to setup genesis logic")
	}

	if err = c.SetupAssetAccounts(dirtyObjects, assetAccounts); err != nil {
		return errors.Wrap(err, "failed to setup asset accounts")
	}

	for _, stateObject := range dirtyObjects {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		if err = c.AddGenesisTesseract(stateObject.Address(), stateHash, stateObject.ContextHash()); err != nil {
			return err
		}
	}

	return nil
}

func (c *ChainManager) SetupAssetAccounts(
	stateObjects map[common.Address]*state.Object,
	assetAccs []common.AssetAccountSetupArgs,
) error {
	for _, assetAccount := range assetAccs {
		accAddress := common.CreateAddressFromString(assetAccount.AssetInfo.Symbol)

		stateObjects[accAddress] = c.sm.CreateDirtyObject(accAddress, common.AssetAccount)

		_, err := stateObjects[accAddress].CreateContext(assetAccount.BehaviouralContext, assetAccount.RandomContext)
		if err != nil {
			return err
		}

		assetID, err := stateObjects[accAddress].CreateAsset(accAddress, assetAccount.AssetInfo.AssetDescriptor())
		if err != nil {
			return err
		}

		if assetAccount.AssetInfo.Operator != common.NilAddress {
			if _, ok := stateObjects[assetAccount.AssetInfo.Operator]; !ok {
				return errors.New("operator account not found")
			}

			_, err = stateObjects[assetAccount.AssetInfo.Operator].CreateAsset(
				accAddress, assetAccount.AssetInfo.AssetDescriptor())
			if err != nil {
				return err
			}
		}

		for _, allocation := range assetAccount.AssetInfo.Allocations {
			if _, ok := stateObjects[allocation.Address]; !ok {
				return errors.New("allocation address not found in state objects")
			}

			stateObjects[allocation.Address].AddBalance(assetID, allocation.Amount.ToInt())
		}
	}

	return nil
}

func (c *ChainManager) ParseGenesisFile(path string) (
	*common.AccountSetupArgs,
	[]common.AccountSetupArgs,
	[]common.AssetAccountSetupArgs,
	[]common.LogicSetupArgs,
	error,
) {
	genesisData := new(common.GenesisFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	err = c.validateSargaAccountCreationInfo(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	err = c.validateAccountCreationInfo(genesisData.Accounts...)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if err = c.validateAssetAccountCreationArgs(genesisData.AssetAccounts...); err != nil {
		return nil, nil, nil, nil, err
	}

	if err = c.validateLogicCreationArgs(genesisData.Logics...); err != nil {
		return nil, nil, nil, nil, err
	}

	return &genesisData.SargaAccount, genesisData.Accounts, genesisData.AssetAccounts, genesisData.Logics, nil
}

func (c *ChainManager) Start() error {
	return nil
}

func (c *ChainManager) Close() {
	c.logger.Info("Closing ChainManager.")
}

func (c *ChainManager) ExecuteAndValidate(tesseracts ...*common.Tesseract) error {
	defer c.exec.Cleanup(tesseracts[0].ClusterID())

	c.logger.Debug(
		"Executing interactions of grid",
		"grid-hash", tesseracts[0].GridHash(),
		"lock", tesseracts[0].Header().ContextLock,
	)

	receipts, err := c.exec.ExecuteInteractions(
		tesseracts[0].Interactions(),
		tesseracts[0].ExecutionContext(),
	)
	if err != nil {
		return err
	}

	if !isReceiptAndGroupHashValid(tesseracts, receipts) || !areStateHashesValid(tesseracts, receipts) {
		if err = c.exec.Revert(tesseracts[0].ClusterID()); err != nil {
			c.logger.Error("Failed to revert the execution changes", "cluster-ID", tesseracts[0].ClusterID())

			return errors.Wrap(err, "failed to revert the execution changes")
		}

		return errors.New("failed to validate the tesseract")
	}

	for _, t := range tesseracts {
		t.SetReceipts(receipts)
	}

	return nil
}

func (c *ChainManager) GetTesseractPartsByGridHash(gridHash common.Hash) (*common.TesseractParts, error) {
	parts := &common.TesseractParts{
		Grid: make(map[common.Address]common.TesseractHeightAndHash),
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

func (c *ChainManager) getInteractionsByGridHash(gridHash common.Hash) (common.Interactions, error) {
	interactions := new(common.Interactions)

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
func (c *ChainManager) GetInteractionAndPartsByTSHash(tsHash common.Hash, ixIndex int) (
	*common.Interaction,
	*common.TesseractParts,
	error,
) {
	rawData, err := c.db.GetTSGridLookup(tsHash)
	if err != nil {
		return nil, nil, errors.Wrap(common.ErrGridHashNotFound, err.Error())
	}

	gridHash := common.BytesToHash(rawData)

	interactions, err := c.getInteractionsByGridHash(gridHash)
	if err != nil {
		return nil, nil, err
	}

	if ixIndex >= len(interactions) || ixIndex < 0 {
		return nil, nil, common.ErrIndexOutOfRange
	}

	parts, err := c.GetTesseractPartsByGridHash(gridHash)
	if err != nil {
		return nil, nil, err
	}

	return interactions[ixIndex], parts, nil
}

// GetInteractionAndPartsByIxHash returns interaction,tesseract parts, ix index for the given tesseract hash
func (c *ChainManager) GetInteractionAndPartsByIxHash(ixHash common.Hash) (
	*common.Interaction,
	*common.TesseractParts,
	int,
	error,
) {
	rawData, err := c.db.GetIXGridLookup(ixHash)
	if err != nil {
		return nil, nil, 0, errors.Wrap(common.ErrGridHashNotFound, err.Error())
	}

	gridHash := common.BytesToHash(rawData)

	interactions, err := c.getInteractionsByGridHash(gridHash)
	if err != nil {
		return nil, nil, 0, err
	}

	for ixIndex, ix := range interactions {
		if ix.Hash() == ixHash {
			parts, err := c.GetTesseractPartsByGridHash(gridHash)
			if err != nil {
				return nil, nil, 0, err
			}

			return ix, parts, ixIndex, nil
		}
	}

	return nil, nil, 0, common.ErrFetchingInteraction
}

func (c *ChainManager) SetupSargaAccount(
	sarga *common.AccountSetupArgs,
	accounts []common.AccountSetupArgs,
	assets []common.AssetAccountSetupArgs,
	logics []common.LogicSetupArgs,
) (*state.Object, error) {
	stateObject := c.sm.CreateDirtyObject(common.SargaAddress, common.SargaAccount)

	if _, err := stateObject.CreateContext(sarga.BehaviouralContext, sarga.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(common.SargaLogicID); err != nil {
		return nil, errors.Wrap(err, "failed to create storage tree")
	}

	if err := stateObject.AddAccountGenesisInfo(common.SargaAddress, common.GenesisIxHash); err != nil {
		return nil, err
	}

	for _, account := range accounts {
		// Add account to sarga storage tree
		if err := stateObject.AddAccountGenesisInfo(account.Address, common.GenesisIxHash); err != nil {
			return nil, err
		}
	}

	for _, logic := range logics {
		// Add logic account to sarga
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(logic.Name),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	for _, assetAcc := range assets {
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(assetAcc.AssetInfo.Symbol),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	return stateObject, nil
}

func (c *ChainManager) SetupNewAccount(info common.AccountSetupArgs) (*state.Object, error) {
	stateObject := c.sm.CreateDirtyObject(info.Address, info.AccType)

	if _, err := stateObject.CreateContext(info.BehaviouralContext, info.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	return stateObject, nil
}

func (c *ChainManager) SetupGenesisLogics(
	dirtyObjects map[common.Address]*state.Object,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicAddr := common.CreateAddressFromString(logic.Name)

		payload := &common.LogicPayload{
			Callsite: logic.Callsite,
			Calldata: logic.Calldata,
			Manifest: logic.Manifest.Bytes(),
		}

		if !common.ContainsAddress(common.GenesisLogicAddrs, logicAddr) {
			c.logger.Error("Mismatch of contract address", "logic-name", logic.Name)

			return nil, errors.New("generated address does not exist in predefined contract address")
		}

		// Create state object for the logic
		stateObj := c.sm.CreateDirtyObject(logicAddr, common.LogicAccount)

		behaviouralCtx := logic.BehaviouralContext
		randomCtx := logic.RandomContext

		_, err := stateObj.CreateContext(behaviouralCtx, randomCtx)
		if err != nil {
			return nil, errors.Wrap(err, "context initiation failed in genesis")
		}

		ctx := &common.ExecutionContext{
			CtxDelta: nil,
			Cluster:  "genesis",
			Time:     c.cfg.GenesisTimestamp,
		}

		// Deploy the genesis logic on that state
		logicID, err := compute.DeployGenesisLogic(ctx, stateObj, payload)
		if err != nil {
			c.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "unable to deploy logic for contract")
		}

		dirtyObjects[stateObj.Address()] = stateObj

		c.logger.Info("Deployed genesis contract", "logic-name", logic.Name, "logic-ID", logicID.String())
	}

	return hashes, nil
}

func areStateHashesValid(tesseracts []*common.Tesseract, receipts common.Receipts) bool {
	if len(tesseracts[0].Interactions()) == 0 {
		return false
	}

	for _, ix := range tesseracts[0].Interactions() {
		receipt, err := receipts.GetReceipt(ix.Hash())
		if err != nil {
			return false
		}

		for _, ts := range tesseracts {
			if receipt.Hashes.StateHash(ts.Address()) != ts.StateHash() {
				return false
			}
		}
	}

	return true
}

func isReceiptAndGroupHashValid(tesseracts []*common.Tesseract, receipts common.Receipts) bool {
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
