package lattice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/moby/locker"
	"github.com/pkg/errors"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/state"
)

type store interface {
	NewBatchWriter() db.BatchWriter
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id identifiers.Address,
		height uint64,
		tesseractHash common.Hash,
		accType common.AccountType,
	) (int32, bool, error)
	GetTesseract(tsHash common.Hash) ([]byte, error)
	SetTesseract(tsHash common.Hash, data []byte) error
	HasTesseract(tsHash common.Hash) bool
	GetTesseractHeightEntry(addr identifiers.Address, height uint64) ([]byte, error)
	SetTesseractHeightEntry(addr identifiers.Address, height uint64, tsHash common.Hash) error
	SetReceipts(tsHash common.Hash, data []byte) error
	GetReceipts(tsHash common.Hash) ([]byte, error)
	SetInteractions(tsHash common.Hash, data []byte) error
	GetInteractions(tsHash common.Hash) ([]byte, error)
	GetIXLookup(ixHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error)
	HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool
	SetIXLookup(ixHash common.Hash, tsHash common.Hash) error
}

type reputationEngine interface {
	UpdateWalletCount(peerID kramaid.KramaID, delta int32) error
}

type stateManager interface {
	CreateStateObject(addr identifiers.Address, accType common.AccountType) *state.Object
	CreateDirtyObject(addr identifiers.Address, accType common.AccountType) *state.Object
	GetDirtyObject(addr identifiers.Address) (*state.Object, error)
	FlushDirtyObject(addrs identifiers.Address) error
	GetAccTypeUsingStateObject(address identifiers.Address) (common.AccountType, error)
	GetLatestTesseract(addr identifiers.Address, withInteractions bool) (*common.Tesseract, error)
	GetContextByHash(addr identifiers.Address, hash common.Hash) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error)
	GetPublicKeys(ctx context.Context, id ...kramaid.KramaID) (keys [][]byte, err error)
	Cleanup(addrs identifiers.Address)
	IsAccountRegistered(addr identifiers.Address) (bool, error)
	IsAccountRegisteredAt(addr identifiers.Address, tesseractHash common.Hash) (bool, error)
	FetchTesseractFromDB(hash common.Hash, withInteractions bool) (*common.Tesseract, error)
	GetLogicIDs(addr identifiers.Address, hash common.Hash) ([]identifiers.LogicID, error)
}

type server interface {
	GetKramaID() kramaid.KramaID
	Broadcast(topic string, data []byte) error
}

type ixpool interface {
	ResetWithHeaders(ts *common.Tesseract)
}

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type ChainManager struct {
	cfg               *config.ChainConfig
	db                store
	mux               *utils.TypeMux
	ixpool            ixpool
	tesseracts        *lru.Cache
	sm                stateManager
	latticeLocks      *locker.Locker
	logger            hclog.Logger
	senatus           reputationEngine
	network           server
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
	senatus reputationEngine,
	metrics *Metrics,
	verifier AggregatedSignatureVerifier,
) (*ChainManager, error) {
	c := &ChainManager{
		cfg:               cfg,
		db:                db,
		mux:               mux,
		ixpool:            ix,
		sm:                sm,
		tesseracts:        cache,
		network:           network,
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

func (c *ChainManager) fetchContextForAgora(addr identifiers.Address, ts common.Tesseract) ([]kramaid.KramaID, error) {
	peers := make([]kramaid.KramaID, 0)

	for {
		if len(peers) >= 10 {
			break
		}

		// fetch the context delta
		deltaGroup, _ := ts.GetContextDelta(addr)
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(addr, ts.PreviousContextHash(addr))
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.TransitiveLink(addr).IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.TransitiveLink(addr), false)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}

		ts = *t
	}

	return peers, nil
}

func (c *ChainManager) UpdateNodeInclusivity(delta *common.DeltaGroup) error {
	for _, kramaID := range delta.BehaviouralNodes {
		if err := c.senatus.UpdateWalletCount(kramaID, 1); err != nil {
			return err
		}
	}

	for _, kramaID := range delta.RandomNodes {
		if err := c.senatus.UpdateWalletCount(kramaID, 1); err != nil {
			return err
		}
	}

	for _, kramaID := range delta.ReplacedNodes {
		if err := c.senatus.UpdateWalletCount(kramaID, -1); err != nil {
			return err
		}
	}

	return nil
}

func (c *ChainManager) GetTesseract(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
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
	address identifiers.Address,
	height uint64,
	withInteractions bool,
) (*common.Tesseract, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return c.GetTesseract(common.BytesToHash(tesseractHash), withInteractions)
}

func (c *ChainManager) GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(address, height)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return common.BytesToHash(tesseractHash), nil
}

func (c *ChainManager) GetLatestTesseract(addr identifiers.Address, withInteractions bool) (*common.Tesseract, error) {
	if addr.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	return c.sm.GetLatestTesseract(addr, withInteractions)
}

func (c *ChainManager) getReceipt(ixHash, tsHash common.Hash) (*common.Receipt, error) {
	rawData, err := c.db.GetReceipts(tsHash)
	if err != nil {
		c.logger.Error("Error fetching receipts", "err", err.Error(), "ts-hash", tsHash, "ix-hash", ixHash)

		return nil, err
	}

	receipts := new(common.Receipts)

	if err = receipts.FromBytes(rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}

func (c *ChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	rawData, err := c.db.GetIXLookup(ixHash)
	if err != nil {
		return nil, errors.Wrap(err, "tesseract hash not found")
	}

	receipt, err := c.getReceipt(ixHash, common.BytesToHash(rawData))
	if err != nil {
		return nil, errors.Wrap(err, "receipt not found")
	}

	return receipt, nil
}

func (c *ChainManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(context.Background(), ts.SealBy())
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
		consensusInfo        = ts.ConsensusInfo()
		publicKeys           = make([][]byte, 0, consensusInfo.BFTVoteSet.TrueIndicesSize())
		votesCounter         = make([]uint32, ts.ParticipantCount()+1)
	)

	for _, valIndex := range ts.BFTVoteSet().GetTrueIndices() {
		nodeSetIndices, _, _, publicKey := ics.GetKramaID(int32(valIndex))
		if nodeSetIndices != nil { // ts.Header.Extra.VoteSet.GetIndex(index)
			publicKeys = append(publicKeys, publicKey)

			for _, index := range nodeSetIndices {
				votesCounter[index/2]++
			}
		} else {
			c.logger.Debug("Error fetching validator address", "index", valIndex)
		}
	}

	for index := range ts.Addresses() {
		if votesCounter[index] < ics.ParticipantQuorum(index*2) {
			return false, common.ErrQuorumFailed
		}
	}

	if votesCounter[len(ts.Addresses())] < ics.RandomQuorumSize() {
		return false, common.ErrQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   ktypes.PRECOMMIT,
		Round:  consensusInfo.Round,
		TSHash: ts.Hash(),
	}

	rawData, err := vote.Bytes()
	if err != nil {
		return false, err
	}

	verified, err := c.signatureVerifier(rawData, consensusInfo.CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	c.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (c *ChainManager) verifyTransitions(
	addr identifiers.Address,
	ts *common.Tesseract,
	allParticipants bool,
) error {
	if ts.ClusterID() == "genesis" {
		return nil
	}

	addresses := make([]identifiers.Address, 0)

	if allParticipants {
		addresses = ts.Addresses()
	} else {
		addresses = append(addresses, addr)
	}

	for _, addr := range addresses {
		initial, err := c.IsInitialTesseract(ts, addr)
		if err != nil {
			return errors.Wrap(err, "Sarga account not found")
		}

		if !initial {
			parent, err := c.GetTesseract(ts.TransitiveLink(addr), false)
			if err != nil {
				c.logger.Error("Failed to fetch parent tesseract", "err", err, "addr", addr)

				return common.ErrPreviousTesseractNotFound
			}

			// Check Heights
			if parent.Height(addr) != ts.Height(addr)-1 {
				return common.ErrInvalidHeight
			}
			// TODO: Add more checks
			// Check time stamp
			if ts.Timestamp() < parent.Timestamp() {
				return common.ErrInvalidBlockTime
			}
		}
	}

	return nil
}

func (c *ChainManager) storeReceipts(bw db.BatchWriter, ts *common.Tesseract) error {
	rawReceipts, err := ts.Receipts().Bytes()
	if err != nil {
		return err
	}

	if err = bw.Set(storage.ReceiptsKey(ts.Hash()), rawReceipts); err != nil {
		return errors.Wrap(err, "error writing receipts to db")
	}

	return nil
}

// The storeInteractions function uses tesseract hash as a key to store interactions.
// It also stores key-value pairs of ix hash and tesseract hash,
func (c *ChainManager) storeInteractions(bw db.BatchWriter, ts *common.Tesseract) error {
	ixRawData, err := ts.Interactions().Bytes()
	if err != nil {
		return err
	}

	tsHash := ts.Hash()

	if err := bw.Set(storage.InteractionsKey(tsHash), ixRawData); err != nil {
		return errors.Wrap(
			err,
			fmt.Sprintf("error writing interactions to db with ts-hash %s", tsHash))
	}

	for _, ix := range ts.Interactions() {
		c.logger.Trace(
			"Storing ts-hash by ix-hash", "ix-hash", ix.Hash(), "ts-hash", tsHash)

		if err = bw.Set(ix.Hash().Bytes(), tsHash.Bytes()); err != nil {
			return errors.Wrap(
				err,
				fmt.Sprintf("error writing tesseract hash to db ix-hash %s ts-hash %s",
					ix.Hash(),
					tsHash,
				))
		}
	}

	return nil
}

func (c *ChainManager) addParticipant(
	cache bool,
	tsHash common.Hash,
	addr identifiers.Address,
	participantState common.State,
) error {
	defer func() {
		c.sm.Cleanup(addr)
	}()

	if err := c.sm.FlushDirtyObject(addr); err != nil {
		return err
	}

	c.logger.Info(
		"Participant added", "addr", addr, "height ", participantState.Height, "ts-hash", tsHash)

	if err := c.db.SetTesseractHeightEntry(addr, participantState.Height, tsHash); err != nil {
		return errors.Wrap(err, "failed to write tesseract height entry")
	}

	accType, err := c.sm.GetAccTypeUsingStateObject(addr)
	if err != nil {
		return errors.Wrap(err, "failed to fetch account type")
	}

	_, _, err = c.db.UpdateAccMetaInfo(
		addr,
		participantState.Height,
		tsHash,
		accType,
	)
	if err != nil {
		return errors.Wrap(err, "account meta info update failed")
	}

	if cache {
		c.tesseracts.Add(addr, tsHash)
	}

	return nil
}

func (c *ChainManager) addParticipantsData(
	cache bool,
	addr identifiers.Address,
	ts *common.Tesseract,
	allParticipants bool,
) error {
	if !allParticipants && addr.IsNil() { // address is mandatory if specific participant needs to be added
		return errors.New("address is not specified")
	}

	participants := make(common.ParticipantStates)

	if allParticipants {
		participants = ts.Participants()
	} else {
		s, ok := ts.State(addr)
		if !ok {
			panic(ok)
		}

		participants[addr] = s
	}

	for address := range participants {
		if c.db.HasAccMetaInfoAt(address, ts.Height(address)) {
			return nil
		}
	}

	for addr, participantState := range participants {
		if err := c.addParticipant(cache, ts.Hash(), addr, participantState); err != nil {
			return err
		}
	}

	return nil
}

func (c *ChainManager) addTesseractData(
	bw db.BatchWriter,
	t *common.Tesseract,
) error {
	tsRawData, err := t.Canonical().Bytes()
	if err != nil {
		return err
	}

	if err := bw.Set(storage.TesseractKey(t.Hash()), tsRawData); err != nil {
		return errors.Wrap(err, "error writing tesseract to db")
	}

	if err = c.storeInteractions(bw, t); err != nil {
		return errors.Wrap(err, "failed to store interactions")
	}

	if err = c.storeReceipts(bw, t); err != nil {
		return errors.Wrap(err, "failed to store receipts")
	}

	return nil
}

func (c *ChainManager) addTesseract(
	cache bool,
	addr identifiers.Address,
	t *common.Tesseract,
	allParticipants bool,
) error {
	if err := c.addParticipantsData(cache, addr, t, allParticipants); err != nil {
		return err
	}

	if !c.db.HasTesseract(t.Hash()) {
		bw := c.db.NewBatchWriter()

		if err := c.addTesseractData(bw, t); err != nil {
			return err
		}

		if err := bw.Flush(); err != nil {
			return errors.Wrap(err, "failed to flush tesseract")
		}

		c.logger.Info("Tesseract added", "ts-hash ", t.Hash())

		if cache {
			c.tesseracts.Add(t.Hash(), t.GetTesseractWithoutIxns())
		}

		if err := c.mux.Post(utils.TesseractAddedEvent{Tesseract: t}); err != nil {
			c.logger.Error("Error sending tesseract added event", "err", err)
		}

		// update peer occupancy metrics
		for _, p := range t.Participants() {
			if p.ContextDelta == nil {
				continue
			}

			if err := c.UpdateNodeInclusivity(p.ContextDelta); err != nil {
				return errors.Wrap(err, common.ErrUpdatingInclusivity.Error())
			}
		}

		c.ixpool.ResetWithHeaders(t)
	}

	return nil
}

func (c *ChainManager) AddTesseractWithState(
	addr identifiers.Address,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	allParticipants bool,
) error {
	if ts == nil {
		return errors.New("nil tesseract")
	}

	if len(dirtyStorage) == 0 {
		return errors.New("empty dirty storage")
	}

	tsAdditionInitTime := time.Now()

	if _, ok := dirtyStorage[ts.ICSHash()]; !ok {
		return errors.New("cluster info not found")
	}

	if err := func(ts *common.Tesseract) error {
		tsHash := ts.Hash()

		c.latticeLocks.Lock(tsHash.Hex())

		defer func() {
			if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
				c.logger.Error("Failed to unlock lattice", "err", err, "ts-hash", tsHash)
			}
		}()

		if err := c.addTesseract(true, addr, ts, allParticipants); err != nil {
			return err
		}

		return nil
	}(ts); err != nil {
		return err
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

func (c *ChainManager) ValidateTesseract(
	addr identifiers.Address,
	ts *common.Tesseract,
	ics *common.ICSNodeSet,
	allParticipants bool,
) error {
	tsHash := ts.Hash()

	c.latticeLocks.Lock(tsHash.Hex())
	defer func() {
		if err := c.latticeLocks.Unlock(tsHash.Hex()); err != nil {
			c.logger.Error("Failed to unlock lattice", "err", err, "addr", addr)
		}
	}()

	if c.db.HasAccMetaInfoAt(addr, ts.Height(addr)) {
		return common.ErrAlreadyKnown
	}

	validSeal, err := c.IsSealValid(ts)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal", "err", err)

		return common.ErrInvalidSeal
	}

	if err = c.verifyTransitions(addr, ts, allParticipants); err != nil {
		return err
	}

	verified, err := c.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to verify signatures %v %v", addr, ts.Height(addr)))
	}

	return nil
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

func (c *ChainManager) IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error) {
	var (
		accountRegistered bool
		err               error
	)

	if info, ok := ts.State(common.SargaAddress); !ok {
		accountRegistered, err = c.sm.IsAccountRegistered(addr)
	} else {
		c.logger.Debug(
			"Checking for new account",
			"addr", addr,
			"height", info.Height,
			"ts-hash", info.TransitiveLink,
		)
		accountRegistered, err = c.sm.IsAccountRegisteredAt(addr, info.TransitiveLink)
	}

	return !accountRegistered && ts.Height(addr) == 0, err
}

func (c *ChainManager) AddGenesisTesseract(
	addresses []identifiers.Address,
	stateHashes, contextHashes []common.Hash,
	timestamp uint64,
) error {
	tesseract := createGenesisTesseract(addresses, stateHashes, contextHashes, timestamp)

	if err := c.addTesseract(true, identifiers.NilAddress, tesseract, true); err != nil {
		return errors.Wrap(err, "error adding genesis tesseract")
	}

	return nil
}

func (c *ChainManager) SetupGenesis(cfg *config.ChainConfig) error {
	dirtyObjects := make(map[identifiers.Address]*state.Object)

	sargaAccount, genesisAccounts, assetAccounts, logics, err := c.ParseGenesisFile(cfg.GenesisFilePath)
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

	count := len(dirtyObjects)
	addresses := make([]identifiers.Address, 0, count)
	stateHashes := make([]common.Hash, 0, count)
	contextHashes := make([]common.Hash, 0, count)

	for _, stateObject := range dirtyObjects {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		addresses = append(addresses, stateObject.Address())
		stateHashes = append(stateHashes, stateHash)
		contextHashes = append(contextHashes, stateObject.ContextHash())
	}

	if err = c.AddGenesisTesseract(addresses, stateHashes, contextHashes, cfg.GenesisTimestamp); err != nil {
		return err
	}

	return nil
}

func (c *ChainManager) SetupAssetAccounts(
	stateObjects map[identifiers.Address]*state.Object,
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

		if assetAccount.AssetInfo.Operator != identifiers.NilAddress {
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

func (c *ChainManager) getInteractionsByTSHash(tsHash common.Hash) (common.Interactions, error) {
	interactions := new(common.Interactions)

	buf, err := c.db.GetInteractions(tsHash)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching interactions")
	}

	if err := interactions.FromBytes(buf); err != nil {
		return nil, err
	}

	return *interactions, nil
}

// GetInteractionAndParticipantsByTSHash returns interaction,participants for the given tesseract hash and ix index
func (c *ChainManager) GetInteractionAndParticipantsByTSHash(tsHash common.Hash, ixIndex int) (
	*common.Interaction,
	common.ParticipantStates,
	error,
) {
	ts, err := c.GetTesseract(tsHash, true)
	if err != nil {
		return nil, nil, err
	}

	interactions := ts.Interactions()

	if ixIndex >= len(interactions) || ixIndex < 0 {
		return nil, nil, common.ErrIndexOutOfRange
	}

	return interactions[ixIndex], ts.Participants(), nil
}

// GetInteractionAndParticipantsByIxHash returns interaction ,ts hash,
// participants, ix index for the given tesseract hash
func (c *ChainManager) GetInteractionAndParticipantsByIxHash(ixHash common.Hash) (
	*common.Interaction,
	common.Hash,
	common.ParticipantStates,
	int,
	error,
) {
	rawData, err := c.db.GetIXLookup(ixHash)
	if err != nil {
		return nil, common.NilHash, nil, 0, errors.Wrap(common.ErrTSHashNotFound, err.Error())
	}

	tsHash := common.BytesToHash(rawData)

	ts, err := c.GetTesseract(tsHash, true)
	if err != nil {
		return nil, common.NilHash, nil, 0, err
	}

	for ixIndex, ix := range ts.Interactions() {
		if ix.Hash() == ixHash {
			return ix, ts.Hash(), ts.Participants(), ixIndex, nil
		}
	}

	return nil, common.NilHash, nil, 0, common.ErrFetchingInteraction
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
	dirtyObjects map[identifiers.Address]*state.Object,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicAddr := common.CreateAddressFromString(logic.Name)

		if !common.ContainsAddress(common.GenesisLogicAddrs, logicAddr) {
			c.logger.Error("Mismatch of contract address", "logic-name", logic.Name)

			return nil, errors.New("generated address does not exist in predefined contract address")
		}

		// Create state object for the logic
		logicState := c.sm.CreateDirtyObject(logicAddr, common.LogicAccount)

		// Create a dummy state object for the deployer
		// NOTE: This is a dummy object we create at genesis deployment with the 0x00..00 address
		// to act as a placeholder account for the execution environment's sender state driver.
		deployerState := c.sm.CreateStateObject(identifiers.NilAddress, common.RegularAccount)

		behaviouralCtx := logic.BehaviouralContext
		randomCtx := logic.RandomContext

		_, err := logicState.CreateContext(behaviouralCtx, randomCtx)
		if err != nil {
			return nil, errors.Wrap(err, "context initiation failed in genesis")
		}

		// Create a new execution context
		ctx := &common.ExecutionContext{
			CtxDelta: nil,
			Cluster:  "genesis",
			Time:     c.cfg.GenesisTimestamp,
		}

		// Create a new IxLogicDeploy interaction with the logic payload
		ix, _ := common.NewInteraction(common.IxData{Input: common.IxInput{
			Type: common.IxLogicDeploy,
			Payload: func() []byte {
				payload := &common.LogicPayload{
					Callsite: logic.Callsite,
					Calldata: logic.Calldata,
					Manifest: logic.Manifest.Bytes(),
				}

				encoded, _ := payload.Bytes()

				return encoded
			}(),
		}}, nil)

		// Deploy the genesis logic and check for errors
		_, receipt, err := compute.DeployLogic(ctx, ix, logicState, deployerState)
		if err != nil {
			c.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "deployment failed for logic")
		}

		if receipt.Error != nil {
			return nil, errors.Errorf("deployment call failed: %#x", receipt.Error)
		}

		// Update the dirty objects map with the logic state object
		dirtyObjects[logicState.Address()] = logicState

		// Obtain the logic ID from the call receipt
		logicID := receipt.LogicID
		c.logger.Info("Deployed genesis contract",
			"logic-name", logic.Name,
			"logic-ID", logicID.String(),
		)
	}

	return hashes, nil
}
