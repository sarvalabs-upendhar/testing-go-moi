package storage

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/storage/db/badger"
)

// MaxBucketCount tells the no of buckets , accounts can be classified into
const (
	MaxBucketCount uint64 = 1024
)

// PersistenceManager manages all the critical information to perform content-addressed persistence services
type PersistenceManager struct {
	config *config.DBConfig
	db     db.Database
	logger hclog.Logger
}

// NewPersistenceManager is used by the caller to instantiate a PersistenceManager
func NewPersistenceManager(
	logger hclog.Logger,
	config *config.DBConfig,
	metrics *db.Metrics,
) (*PersistenceManager, error) {
	badgerDB, err := badger.NewBadgerDB(config.DBFolderPath, metrics, hclog.NewNullLogger())
	if err != nil {
		return nil, errors.Wrap(common.ErrDBInit, err.Error())
	}

	if config.CleanDB {
		if err = badgerDB.CleanUp(); err != nil {
			panic(err)
		}
	}

	p := &PersistenceManager{
		config: config,
		logger: logger.Named("Persistence-Manager"),
		db:     badgerDB,
	}

	return p, nil
}

func (p *PersistenceManager) GetBucketCount(bucketNumber uint64) (uint64, error) {
	val, err := p.ReadEntry(bucketCountKey(bucketNumber))
	if err == common.ErrKeyNotFound { //nolint
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint64(val), nil
}

// GetBucketSizes returns the accounts count for each bucket
func (p *PersistenceManager) GetBucketSizes() (map[uint64]uint64, error) {
	buckets := make(map[uint64]uint64)

	for i := uint64(0); i < MaxBucketCount; i++ {
		count, err := p.GetBucketCount(i)
		if !errors.Is(err, common.ErrKeyNotFound) {
			return nil, err
		}

		if err == nil {
			buckets[i] = count
		}
	}

	return buckets, nil
}

// GetAccountMetaInfo fetches the account meta info for a given identifier
func (p *PersistenceManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	key, _ := BucketKeyAndID(NewIdentifierKey(id))

	data, err := p.ReadEntry(key)
	if err != nil {
		return nil, common.ErrAccountNotFound
	}

	accMetaInfo := new(common.AccountMetaInfo)
	if err = accMetaInfo.FromBytes(data); err != nil {
		return nil, err
	}

	return accMetaInfo, nil
}

func (p *PersistenceManager) HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool {
	accMetaInfo, err := p.GetAccountMetaInfo(id)
	if err != nil {
		return false
	}

	if height > accMetaInfo.Height {
		return false
	}

	return true
}

// incrementBucketCount is used to increment bucket count when new id is added to lattice
func (p *PersistenceManager) incrementBucketCount(bucket uint64, count uint64) error {
	var (
		rawCount = make([]byte, 8)
		key      = bucketCountKey(bucket)
	)

	data, err := p.ReadEntry(key)
	if err == nil {
		count = binary.BigEndian.Uint64(data) + count

		binary.BigEndian.PutUint64(rawCount, count)

		return p.UpdateEntry(key, rawCount)
	} else if errors.Is(err, common.ErrKeyNotFound) {
		binary.BigEndian.PutUint64(rawCount, count)

		return p.CreateEntry(key, rawCount)
	}

	return err
}

// UpdateAccMetaInfo is used to update the meta-data of an account, this meta-data includes
// Height - Current height of the lattice
func (p *PersistenceManager) UpdateAccMetaInfo(
	id identifiers.Identifier,
	height uint64,
	tesseractHash common.Hash,
	stateHash, contextHash common.Hash,
	consensusNodesHash common.Hash,
	commitHash common.Hash,
	accType common.AccountType,
	shouldUpdateContextSetPosition bool,
	positionInContextSet int,
) (int32, bool, error) {
	if id.IsNil() {
		return 0, false, common.ErrInvalidIdentifier
	}

	if tesseractHash.IsNil() {
		return 0, false, common.ErrEmptyHash
	}

	key, bucketID := BucketKeyAndID(NewIdentifierKey(id))

	data, err := p.ReadEntry(key)
	if err == nil {
		accMetaInfo := new(common.AccountMetaInfo)
		if err := accMetaInfo.FromBytes(data); err != nil {
			return -1, false, err
		}

		if height == accMetaInfo.Height && tesseractHash != accMetaInfo.TesseractHash {
			return -1, false, common.ErrHashMismatch
		}

		if height >= accMetaInfo.Height {
			accMetaInfo.TesseractHash = tesseractHash
			accMetaInfo.StateHash = stateHash
			accMetaInfo.ContextHash = contextHash
			accMetaInfo.ID = id
			accMetaInfo.Height = height
			accMetaInfo.CommitHash = commitHash

			if shouldUpdateContextSetPosition {
				accMetaInfo.PositionInContextSet = positionInContextSet
			}

			if !consensusNodesHash.IsNil() {
				accMetaInfo.ConsensusNodesHash = consensusNodesHash
			}
		}

		rawData, err := accMetaInfo.Bytes()
		if err != nil {
			return -1, false, err
		}

		return int32(bucketID), false, p.UpdateEntry(key, rawData)
	} else if errors.Is(err, common.ErrKeyNotFound) {
		msg := common.AccountMetaInfo{
			TesseractHash:        tesseractHash,
			StateHash:            stateHash,
			ContextHash:          contextHash,
			Type:                 accType,
			ID:                   id,
			Height:               height,
			CommitHash:           commitHash,
			ConsensusNodesHash:   consensusNodesHash,
			PositionInContextSet: positionInContextSet,
		}

		rawData, err := msg.Bytes()
		if err != nil {
			return -1, false, err
		}

		if err = p.CreateEntry(key, rawData); err != nil {
			return -1, false, err
		}

		if err = p.incrementBucketCount(bucketID, 1); err != nil {
			log.Panic(err)
		}

		return int32(bucketID), true, nil
	}

	return -1, false, err
}

// Close shutdowns the database
func (p *PersistenceManager) Close() {
	p.logger.Info("Closing Database")

	if err := p.db.Close(); err != nil {
		p.logger.Error("Error closing the local BadgerDB instance", "err", err)
	}
}

// CreateEntry stores the given k-v entry in database
func (p *PersistenceManager) CreateEntry(key []byte, value []byte) error {
	return p.db.Insert(key, value)
}

// UpdateEntry updates the value associated with the given key
func (p *PersistenceManager) UpdateEntry(key []byte, newValue []byte) error {
	return p.db.Update(key, newValue)
}

// StreamAccountMetaInfosRaw fetches meta info of all the accounts for a given bucket number
func (p *PersistenceManager) StreamAccountMetaInfosRaw(
	ctx context.Context,
	bucketNumber uint64,
	response chan []byte,
) error {
	it, err := p.db.NewIterator()
	if err != nil {
		return err
	}

	defer func() {
		it.Close()
		close(response)
	}()

	prefix := bucketPrefix(bucketNumber)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			dbEntry, err := it.GetNext()
			if err != nil {
				return err
			}
			response <- dbEntry.Value
		}
	}

	return nil
}

// ReadEntry takes the cid and returns corresponding content
func (p *PersistenceManager) ReadEntry(key []byte) ([]byte, error) {
	return p.db.Get(key)
}

// Contains is a light-weight function called to check for the presence of a k-v entry in the local Badgerdb instance
func (p *PersistenceManager) Contains(key []byte) (bool, error) {
	return p.db.Has(key)
}

// DeleteEntry deletes the key-value entry from the local BadgerDB instance
func (p *PersistenceManager) DeleteEntry(key []byte) error {
	return p.db.Delete(key)
}

func (p *PersistenceManager) NewBatchWriter() db.BatchWriter {
	return p.db.NewBatchWriter()
}

func (p *PersistenceManager) Cleanup() error {
	return p.db.CleanUp()
}

func (p *PersistenceManager) DropSenatusEntries() error {
	if err := p.db.DropWithPrefix(SenatusPrefix()); err != nil {
		return errors.Wrap(err, "failed to drop senatus entries")
	}

	if err := p.db.Delete(dbKey(identifiers.Nil, SenatusPeerCount, nil)); err != nil {
		return errors.Wrap(err, "failed to drop senatus peer count entry")
	}

	return nil
}

// GetEntriesWithPrefix fetches array of k,v pairs with the given prefix
func (p *PersistenceManager) GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error) {
	ch := make(chan *common.DBEntry)

	it, err := p.db.NewIterator()
	if err != nil {
		return nil, errors.New("failed to initiate iterator")
	}

	go func() {
		defer func() {
			it.Close()
			close(ch)
		}()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			dbEntry, err := it.GetNext()
			if err != nil {
				p.logger.Error("PrefixTag iteration failed", "err", err)

				break
			}

			select {
			case <-ctx.Done():
				return
			case ch <- dbEntry:
			}
		}
	}()

	return ch, nil
}

func (p *PersistenceManager) SetAccount(id identifiers.Identifier, stateHash common.Hash, data []byte) error {
	key := dbKey(id, Account, stateHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) GetAccount(id identifiers.Identifier, stateHash common.Hash) ([]byte, error) {
	key := dbKey(id, Account, stateHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetAccountKeys(id identifiers.Identifier, accountKeysHash common.Hash) ([]byte, error) {
	key := dbKey(id, AccountKeys, accountKeysHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetContext(id identifiers.Identifier, contextHash common.Hash) ([]byte, error) {
	key := dbKey(id, Context, contextHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetStorage(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	key := dbKey(id, Storage, hash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetRawTesseract(tsHash common.Hash) ([]byte, error) {
	key := dbKey(identifiers.Nil, Tesseract, tsHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetTesseract(tsHash common.Hash, data []byte) error {
	key := dbKey(identifiers.Nil, Tesseract, tsHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) HasTesseract(tsHash common.Hash) bool {
	key := dbKey(identifiers.Nil, Tesseract, tsHash.Bytes())

	exists, err := p.db.Has(key)
	if err != nil {
		p.logger.Error("failed to check for tesseract", "err", err)
	}

	return exists
}

func (p *PersistenceManager) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) ([]byte, error) {
	return p.ReadEntry(tesseractHeightKey(id, height))
}

func (p *PersistenceManager) SetTesseractHeightEntry(
	id identifiers.Identifier, height uint64, hash common.Hash,
) error {
	return p.CreateEntry(tesseractHeightKey(id, height), hash.Bytes())
}

// SetInteractions stores tesseract hash and raw interactions data as key value pair
func (p *PersistenceManager) SetInteractions(tsHash common.Hash, data []byte) error {
	key := dbKey(identifiers.Nil, Interaction, tsHash.Bytes())

	return p.CreateEntry(key, data)
}

// GetInteractions returns raw interactions data for the given tesseract hash
func (p *PersistenceManager) GetInteractions(tsHash common.Hash) ([]byte, error) {
	key := dbKey(identifiers.Nil, Interaction, tsHash.Bytes())

	return p.ReadEntry(key)
}

// SetIXLookup stores interaction hash and tesseract hash as key value pair
func (p *PersistenceManager) SetIXLookup(ixHash common.Hash, tsHash common.Hash) error {
	return p.CreateEntry(ixHash.Bytes(), tsHash.Bytes())
}

// GetIXLookup returns raw tesseract hash for the given interaction hash
func (p *PersistenceManager) GetIXLookup(ixHash common.Hash) ([]byte, error) {
	return p.ReadEntry(ixHash.Bytes())
}

// SetReceipts stores tesseract hash and raw receipt data as key value pair
func (p *PersistenceManager) SetReceipts(tsHash common.Hash, data []byte) error {
	key := dbKey(identifiers.Nil, Receipt, tsHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) SetCommitInfo(tsHash common.Hash, data []byte) error {
	key := TesseractCommitInfoKey(tsHash)

	return p.CreateEntry(key, data)
}

// GetReceipts returns raw receipt data for the given tesseract hash
func (p *PersistenceManager) GetReceipts(tsHash common.Hash) ([]byte, error) {
	key := dbKey(identifiers.Nil, Receipt, tsHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetAccountSyncStatus(id identifiers.Identifier, status *common.AccountSyncStatus) error {
	key := dbKey(identifiers.Nil, AccountSyncJob, id.Bytes())

	rawData, err := status.Bytes()
	if err != nil {
		return err
	}

	return p.UpdateEntry(key, rawData)
}

func (p *PersistenceManager) CleanupAccountSyncStatus(id identifiers.Identifier) error {
	key := dbKey(identifiers.Nil, AccountSyncJob, id.Bytes())

	return p.DeleteEntry(key)
}

func (p *PersistenceManager) GetMerkleTreeEntry(
	id identifiers.Identifier,
	prefix PrefixTag,
	actualKey []byte,
) ([]byte, error) {
	key := dbKey(id, prefix, actualKey)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetMerkleTreeEntry(
	id identifiers.Identifier,
	prefix PrefixTag,
	actualKey, value []byte,
) error {
	key := dbKey(id, prefix, actualKey)

	return p.CreateEntry(key, value)
}

func (p *PersistenceManager) SetMerkleTreeEntries(
	id identifiers.Identifier,
	prefix PrefixTag,
	entries map[string][]byte,
) error {
	// Create a batch writer
	batchWriter := p.NewBatchWriter()

	for k, v := range entries {
		key := dbKey(id, prefix, []byte(k))
		// Add to batch writer
		if err := batchWriter.Set(key, v); err != nil {
			return err
		}
	}

	return batchWriter.Flush()
}

func (p *PersistenceManager) WritePreImages(
	id identifiers.Identifier,
	entries map[common.Hash][]byte,
) error {
	batchWriter := p.NewBatchWriter()

	for k, v := range entries {
		key := PreImageKey(id, k)
		// Add to batch writer
		if err := batchWriter.Set(key, v); err != nil {
			return err
		}
	}

	return batchWriter.Flush()
}

func (p *PersistenceManager) GetPreImage(
	id identifiers.Identifier,
	hash common.Hash,
) ([]byte, error) {
	key := PreImageKey(id, hash)

	return p.ReadEntry(key)
}

// StreamSnapshot streams a snapshot of all entries with the given key prefix on resp channel
// Snapshot contains all the entries with version > sinceTs
func (p *PersistenceManager) StreamSnapshot(
	ctx context.Context,
	id identifiers.Identifier,
	sinceTS uint64,
	respChan chan<- common.SnapResponse,
) (uint64, error) {
	kv := NewKVCollector(ctx, p.logger, p.config.MaxSnapSize, respChan)

	err := p.db.Snapshot(ctx, id.Bytes(), sinceTS, kv)
	if err != nil {
		return 0, errors.Wrap(err, "failed to generate snapshot")
	}

	return kv.Size, nil
}

func (p *PersistenceManager) GetRecentUpdatedAccMetaInfosRaw(
	ctx context.Context,
	bucketID uint64,
	sinceTS uint64,
) ([][]byte, error) {
	vc := NewValueCollector(p.config.MaxSnapSize)

	err := p.db.Snapshot(ctx, bucketPrefix(bucketID), sinceTS, vc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate snapshot")
	}

	return vc.Entries, nil
}

func (p *PersistenceManager) StoreAccountSnapShot(snap *common.Snapshot) error {
	batchWriter := p.db.NewBatchWriter()

	if err := batchWriter.WriteBuffer(snap.Entries); err != nil {
		return err
	}

	if err := batchWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func (p *PersistenceManager) GetRegisteredAccounts() ([]identifiers.Identifier, error) {
	idsList := make([]identifiers.Identifier, 0)

	for i := uint64(0); i < 1024; i++ {
		prefix := bucketPrefix(i)

		entries, err := p.GetEntriesWithPrefix(context.Background(), prefix)
		if err != nil {
			return nil, err
		}

		for entry := range entries {
			id := entry.Key[16:]
			idsList = append(idsList, identifiers.Identifier(id))
		}
	}

	return idsList, nil
}

func (p *PersistenceManager) GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error) {
	syncInfos := make([]*common.AccountSyncStatus, 0)

	it, err := p.db.NewIterator()
	if err != nil {
		return nil, err
	}

	defer it.Close()

	for it.Seek(AccountSyncPrefix()); it.ValidForPrefix(AccountSyncPrefix()); it.Next() {
		dbEntry, err := it.GetNext()
		if err != nil {
			return nil, err
		}

		syncInfo := new(common.AccountSyncStatus)
		if err = syncInfo.FromBytes(dbEntry.Value); err != nil {
			p.logger.Error(err.Error())

			continue
		}

		syncInfos = append(syncInfos, syncInfo)
	}

	return syncInfos, nil
}

func (p *PersistenceManager) GetTesseract(
	tsHash common.Hash,
	withInteractions, withCommitInfo bool,
) (*common.Tesseract, error) {
	// Fetch ts from DB
	rawTesseract, err := p.GetRawTesseract(tsHash)
	if err != nil {
		return nil, err
	}

	// ts is a clone of the tesseract. The only difference is that it won't have the interaction's
	ts := new(common.Tesseract)

	if err = ts.FromBytes(rawTesseract); err != nil {
		return nil, err
	}

	interactions := new(common.Interactions)
	receipts := new(common.Receipts)
	commitInfo := new(common.CommitInfo)

	// Fetch interactions for non-genesis tesseracts from DB
	if withInteractions && ts.ConsensusInfo().View != common.GenesisView {
		rawIxns, err := p.GetInteractions(tsHash)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrFetchingInteractions.Error())
		}

		if err := interactions.FromBytes(rawIxns); err != nil {
			return nil, err
		}

		rawReceipts, err := p.GetReceipts(tsHash)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrReceiptNotFound.Error())
		}

		if rawReceipts != nil {
			if err = receipts.FromBytes(rawReceipts); err != nil {
				return nil, err
			}
		}
	}

	if withCommitInfo {
		rawCommitInfo, err := p.GetCommitInfo(tsHash)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrCommitInfoNotFound.Error())
		}

		if err = commitInfo.FromBytes(rawCommitInfo); err != nil {
			return nil, err
		}
	}

	ts.WithIxnAndReceipts(*interactions, *receipts, commitInfo)

	return ts, nil
}

func (p *PersistenceManager) GetDeeds(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	return p.ReadEntry(DeedsKey(id, hash))
}

func (p *PersistenceManager) DropPrefix(prefix []byte) error {
	return p.db.DropWithPrefix(prefix)
}

func (p *PersistenceManager) GetCommitInfo(tsHash common.Hash) ([]byte, error) {
	return p.db.Get(TesseractCommitInfoKey(tsHash))
}

func (p *PersistenceManager) UpdatePrimarySyncStatus(id identifiers.Identifier) error {
	return p.CreateEntry(AccSyncStatusKey(id), []byte{0x01})
}

func (p *PersistenceManager) IsAccountPrimarySyncDone(id identifiers.Identifier) bool {
	isSynced, err := p.db.Has(AccSyncStatusKey(id))
	if err != nil {
		p.logger.Error("Error checking the account sync status", "err", err)
	}

	return isSynced
}

func (p *PersistenceManager) IsPrincipalSyncDone() (bool, int64) {
	value, err := p.db.Get(principalSyncStatusKey())
	if err != nil {
		return false, 0
	}

	return true, int64(binary.BigEndian.Uint64(value))
}

func (p *PersistenceManager) UpdatePrincipalSyncStatus() error {
	value := make([]byte, 8)

	binary.BigEndian.PutUint64(value, uint64(time.Now().UnixNano()))

	return p.db.Update(principalSyncStatusKey(), value)
}

func (p *PersistenceManager) GetLastActiveTimeStamp() uint64 {
	return p.db.GetLastActiveTimeStamp()
}

func (p *PersistenceManager) SetConsensusProposalInfo(tsHash common.Hash, raw []byte) error {
	return p.UpdateEntry(ConsensusProposalKey(tsHash), raw)
}

func (p *PersistenceManager) DeleteConsensusProposalInfo(tsHash common.Hash) error {
	return p.DeleteEntry(ConsensusProposalKey(tsHash))
}

func (p *PersistenceManager) GetConsensusProposalInfo(tsHash common.Hash) ([]byte, error) {
	return p.ReadEntry(ConsensusProposalKey(tsHash))
}

func (p *PersistenceManager) GetAllConsensusProposalInfo(ctx context.Context) ([][]byte, error) {
	values := make([][]byte, 0)

	prefix := append([]byte(NonAccountPrefix), ConsensusProposals.Byte())

	entries, err := p.GetEntriesWithPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}

	for entry := range entries {
		values = append(values, entry.Value)
	}

	return values, nil
}

func (p *PersistenceManager) GetSafetyData(id identifiers.Identifier) ([]byte, error) {
	return p.ReadEntry(AccountSafetyInfoKey(id))
}

func (p *PersistenceManager) SetSafetyData(id identifiers.Identifier, data []byte) error {
	return p.UpdateEntry(AccountSafetyInfoKey(id), data)
}

func (p *PersistenceManager) DeleteSafetyData(id identifiers.Identifier) error {
	return p.DeleteEntry(AccountSafetyInfoKey(id))
}
