package lattice

import (
	"fmt"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-polo"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"
)

type store interface {
	NewBatchWriter() db.BatchWriter
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	UpdateAccMetaInfo(
		id identifiers.Identifier,
		height uint64,
		tesseractHash common.Hash,
		stateHash, contextHash common.Hash,
		consensusNodesHash common.Hash,
		inheritedAccount identifiers.Identifier,
		commitHash common.Hash,
		accType common.AccountType,
		shouldUpdateContextSetPosition bool,
		positionInContextSet int,
	) (int32, bool, error)
	GetRawTesseract(tsHash common.Hash) ([]byte, error)
	SetTesseract(tsHash common.Hash, data []byte) error
	HasTesseract(tsHash common.Hash) bool
	GetTesseractHeightEntry(id identifiers.Identifier, height uint64) ([]byte, error)
	SetTesseractHeightEntry(id identifiers.Identifier, height uint64, tsHash common.Hash) error
	SetReceipts(tsHash common.Hash, data []byte) error
	GetReceipts(tsHash common.Hash) ([]byte, error)
	SetInteractions(tsHash common.Hash, data []byte) error
	GetInteractions(tsHash common.Hash) ([]byte, error)
	GetIXLookup(ixHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool
	SetIXLookup(ixHash common.Hash, tsHash common.Hash) error
	GetTesseract(
		hash common.Hash,
		withInteractions bool,
		withCommitInfo bool,
	) (*common.Tesseract, error)
}

type reputationEngine interface {
	UpdateWalletCount(peerID kramaid.KramaID, delta int32) error
}

type server interface {
	GetKramaID() kramaid.KramaID
	Broadcast(topic string, data []byte) error
}

type ixpool interface {
	ResetWithHeaders(ts *common.Tesseract)
	RemoveCachedObject(id identifiers.Identifier)
}

type ChainManager struct {
	db         store
	mux        *utils.TypeMux
	ixpool     ixpool
	tesseracts *lru.Cache
	logger     hclog.Logger
	senatus    reputationEngine
	network    server
	metrics    *Metrics
}

func NewChainManager(
	db store,
	logger hclog.Logger,
	mux *utils.TypeMux,
	network server,
	ix ixpool,
	cache *lru.Cache,
	senatus reputationEngine,
	metrics *Metrics,
) (*ChainManager, error) {
	c := &ChainManager{
		db:         db,
		mux:        mux,
		ixpool:     ix,
		tesseracts: cache,
		network:    network,
		logger:     logger.Named("Chain-Manager"),
		senatus:    senatus,
		metrics:    metrics,
	}

	return c, nil
}

func (c *ChainManager) hasTesseract(tsHash common.Hash) bool {
	return c.db.HasTesseract(tsHash)
}

func (c *ChainManager) UpdateNodeInclusivity(delta *common.DeltaGroup) error {
	for _, kramaID := range delta.ConsensusNodes {
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
	withInteractions, withCommitInfo bool,
) (*common.Tesseract, error) {
	if withInteractions {
		return c.db.GetTesseract(hash, withInteractions, withCommitInfo)
	}

	tesseractData, isCached := c.tesseracts.Get(hash)
	if !isCached {
		tesseract, err := c.db.GetTesseract(hash, withInteractions, true)
		if err != nil {
			return nil, err
		}

		c.tesseracts.Add(hash, tesseract)

		if !withCommitInfo {
			return tesseract.GetTesseractWithoutCommitInfo(), nil
		}

		return tesseract, nil
	}

	tesseract, ok := tesseractData.(*common.Tesseract)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	if !withCommitInfo {
		return tesseract.GetTesseractWithoutCommitInfo(), nil
	}

	return tesseract, nil
}

func (c *ChainManager) GetTesseractByHeight(
	id identifiers.Identifier,
	height uint64,
	withInteractions bool,
	withCommitInfo bool,
) (*common.Tesseract, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(id, height)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return c.GetTesseract(common.BytesToHash(tesseractHash), withInteractions, withCommitInfo)
}

func (c *ChainManager) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) (common.Hash, error) {
	tesseractHash, err := c.db.GetTesseractHeightEntry(id, height)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to fetch tesseract height entry")
	}

	return common.BytesToHash(tesseractHash), nil
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

func (c *ChainManager) storeCommitInfo(bw db.BatchWriter, ts *common.Tesseract) error {
	rawCommitInfo, err := ts.CommitInfo().Bytes()
	if err != nil {
		return err
	}

	if err = bw.Set(storage.TesseractCommitInfoKey(ts.Hash()), rawCommitInfo); err != nil {
		return errors.Wrap(err, "error writing receipts to db")
	}

	return nil
}

// The storeInteractions function uses tesseract hash as a key to store interactions.
// It also stores key-value pairs of ix hash and tesseract hash,
func (c *ChainManager) storeInteractions(bw db.BatchWriter, ts *common.Tesseract) error {
	ixs := ts.Interactions()

	ixRawData, err := ixs.Bytes()
	if err != nil {
		return err
	}

	tsHash := ts.Hash()

	if err := bw.Set(storage.InteractionsKey(tsHash), ixRawData); err != nil {
		return errors.Wrap(
			err,
			fmt.Sprintf("error writing interactions to db with ts-hash %s", tsHash))
	}

	for _, ix := range ts.Interactions().IxList() {
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

func (c *ChainManager) addParticipant(id identifiers.Identifier, tsHash common.Hash, commitHash common.Hash,
	participantState common.State, transition *state.Transition,
) error {
	if err := transition.Flush(id); err != nil {
		return err
	}

	c.logger.Info(
		"Participant added", "id", id, "height ",
		participantState.Height, "ts-hash", tsHash, "state-hash", participantState.StateHash)

	if err := c.db.SetTesseractHeightEntry(id, participantState.Height, tsHash); err != nil {
		return errors.Wrap(err, "failed to write tesseract height entry")
	}

	var (
		accType              = transition.GetAccTypeUsingStateObject(id)
		positionInContextSet int
	)

	if participantState.ContextDelta != nil {
		positionInContextSet = participantState.ContextDelta.NodeIndex(c.network.GetKramaID())
	}

	if _, _, err := c.db.UpdateAccMetaInfo(
		id,
		participantState.Height,
		tsHash,
		participantState.StateHash,
		transition.ContextHash(id),
		transition.ConsensusNodesHash(id),
		transition.InheritedAccount(id),
		commitHash,
		accType,
		participantState.ContextDelta != nil,
		positionInContextSet,
	); err != nil {
		return errors.Wrap(err, "account meta info update failed")
	}

	c.ixpool.RemoveCachedObject(id)

	return nil
}

func (c *ChainManager) addParticipantsData(
	id identifiers.Identifier,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if !allParticipants && id.IsNil() { // id is mandatory if specific participant needs to be added
		return errors.New("id is not specified")
	}

	participants := make(common.ParticipantsState)

	if allParticipants {
		participants = ts.Participants()
	} else {
		s, ok := ts.State(id)
		if !ok {
			panic(ok)
		}

		participants[id] = s
	}

	for id, pState := range participants {
		lockType, ok := ts.ConsensusInfo().AccountLocks[id]
		if ok && lockType > common.MutateLock {
			continue
		}

		if pState.StateHash != common.NilHash && c.db.HasAccMetaInfoAt(id, ts.Height(id)) {
			return nil
		}
	}

	for id, participantState := range participants {
		if participantState.StateHash == common.NilHash {
			continue
		}

		lockType, ok := ts.ConsensusInfo().AccountLocks[id]
		if ok && lockType > common.MutateLock {
			continue
		}

		if err := c.addParticipant(id, ts.Hash(), ts.CommitHash(), participantState, transition); err != nil {
			return err
		}
	}

	return nil
}

func (c *ChainManager) addTesseractData(
	bw db.BatchWriter,
	t *common.Tesseract,
) error {
	tsRawData, err := t.Bytes()
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

	if err = c.storeCommitInfo(bw, t); err != nil {
		return err
	}

	return nil
}

func (c *ChainManager) AddTesseract(
	cache bool,
	id identifiers.Identifier,
	t *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if err := c.addParticipantsData(id, t, transition, allParticipants); err != nil {
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

		c.logger.Info("ts added", "ts-hash ", t.Hash())

		if cache {
			c.tesseracts.Add(t.Hash(), t.GetTesseractWithoutIxns())
		}

		c.metrics.captureIxnsPerTesseract(float64(t.Interactions().Len()))

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
	}

	if allParticipants {
		c.ixpool.ResetWithHeaders(t)

		return nil
	}

	for id, p := range t.Participants() {
		if !c.db.HasAccMetaInfoAt(id, p.Height) {
			return nil
		}
	}

	c.ixpool.ResetWithHeaders(t)

	return nil
}

func (c *ChainManager) AddTesseractWithState(
	id identifiers.Identifier,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if ts == nil {
		return errors.New("nil tesseract")
	}

	// if len(dirtyStorage) == 0 {
	//	return errors.New("empty dirty storage")
	// }

	tsAdditionInitTime := time.Now()

	if ts.CommitInfo() == nil {
		return common.ErrCommitInfoNotFound
	}

	if err := c.AddTesseract(true, id, ts, transition, allParticipants); err != nil {
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

func (c *ChainManager) Close() {
	c.logger.Info("Closing ChainManager.")
}

func (c *ChainManager) getInteractionsByTSHash(tsHash common.Hash) ([]*common.Interaction, error) {
	interactions := make([]*common.Interaction, 0)

	buf, err := c.db.GetInteractions(tsHash)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching interactions")
	}

	if err := polo.Depolorize(&interactions, buf); err != nil {
		return nil, err
	}

	return interactions, nil
}

// GetInteractionAndParticipantsByTSHash returns interaction,participants for the given tesseract hash and ix index
func (c *ChainManager) GetInteractionAndParticipantsByTSHash(tsHash common.Hash, ixIndex int) (
	*common.Interaction,
	common.ParticipantsState,
	error,
) {
	ts, err := c.GetTesseract(tsHash, true, false)
	if err != nil {
		return nil, nil, err
	}

	interactions := ts.Interactions()

	if ixIndex >= interactions.Len() || ixIndex < 0 {
		return nil, nil, common.ErrIndexOutOfRange
	}

	return interactions.IxList()[ixIndex], ts.Participants(), nil
}

// GetInteractionAndParticipantsByIxHash returns interaction,ts hash,
// participants, ix index for the given tesseract hash
func (c *ChainManager) GetInteractionAndParticipantsByIxHash(ixHash common.Hash) (
	*common.Interaction,
	common.Hash,
	common.ParticipantsState,
	int,
	error,
) {
	rawData, err := c.db.GetIXLookup(ixHash)
	if err != nil {
		return nil, common.NilHash, nil, 0, errors.Wrap(common.ErrTSHashNotFound, err.Error())
	}

	tsHash := common.BytesToHash(rawData)

	ts, err := c.GetTesseract(tsHash, true, false)
	if err != nil {
		return nil, common.NilHash, nil, 0, err
	}

	for ixIndex, ix := range ts.Interactions().IxList() {
		if ix.Hash() == ixHash {
			return ix, ts.Hash(), ts.Participants(), ixIndex, nil
		}
	}

	return nil, common.NilHash, nil, 0, common.ErrFetchingInteraction
}
