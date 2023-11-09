package storage

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

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
	buckets := make(map[uint64]uint64, 0)

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

// GetAccountMetaInfo fetches the account meta info for a given address
func (p *PersistenceManager) GetAccountMetaInfo(id common.Address) (*common.AccountMetaInfo, error) {
	key, _ := BucketKeyAndID(id)

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

// incrementBucketCount is used to increment bucket count when new address is added to lattice
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
// StateExists - Does the latest state of account exists
// LatticeExists - Does complete lattice exists
func (p *PersistenceManager) UpdateAccMetaInfo(
	id common.Address,
	height uint64,
	tesseractHash common.Hash,
	accType common.AccountType,
	latticeExists, stateExists bool,
) (int32, bool, error) {
	if id.IsNil() {
		return 0, false, common.ErrInvalidAddress
	}

	if tesseractHash.IsNil() {
		return 0, false, common.ErrEmptyHash
	}

	key, bucketID := BucketKeyAndID(id)

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
			accMetaInfo.StateExists = stateExists
			accMetaInfo.TesseractHash = tesseractHash
			accMetaInfo.Address = id
			accMetaInfo.Height = height
		}

		if accMetaInfo.LatticeExists {
			accMetaInfo.LatticeExists = latticeExists
		}

		rawData, err := accMetaInfo.Bytes()
		if err != nil {
			return -1, false, err
		}

		return int32(bucketID), false, p.UpdateEntry(key, rawData)
	} else if errors.Is(err, common.ErrKeyNotFound) {
		msg := common.AccountMetaInfo{
			StateExists:   stateExists,
			LatticeExists: latticeExists,
			TesseractHash: tesseractHash,
			Type:          accType,
			Address:       id,
			Height:        height,
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

// UpdateTesseractStatus is used to update the tesseract state after syncing
func (p *PersistenceManager) UpdateTesseractStatus(
	addr common.Address,
	height uint64,
	tsHash common.Hash,
	status bool,
) error {
	key, _ := BucketKeyAndID(addr)

	data, err := p.ReadEntry(key)
	if err != nil {
		return err
	}

	accMetaInfo := new(common.AccountMetaInfo)
	if err := accMetaInfo.FromBytes(data); err != nil {
		return err
	}

	if height < accMetaInfo.Height {
		return nil
	}

	if tsHash == accMetaInfo.TesseractHash {
		accMetaInfo.StateExists = status
	} else {
		return common.ErrHashMismatch
	}

	rawData, err := accMetaInfo.Bytes()
	if err != nil {
		return err
	}

	return p.UpdateEntry(key, rawData)
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
				p.logger.Error("Prefix iteration failed", "err", err)

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

func (p *PersistenceManager) SetAccount(addr common.Address, stateHash common.Hash, data []byte) error {
	key := dbKey(addr, Account, stateHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) GetAccount(addr common.Address, stateHash common.Hash) ([]byte, error) {
	key := dbKey(addr, Account, stateHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetBalance(addr common.Address, balanceHash common.Hash) ([]byte, error) {
	key := dbKey(addr, Balance, balanceHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetContext(addr common.Address, contextHash common.Hash) ([]byte, error) {
	key := dbKey(addr, Context, contextHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetStorage(addr common.Address, hash common.Hash) ([]byte, error) {
	key := dbKey(addr, Storage, hash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetTesseract(tsHash common.Hash) ([]byte, error) {
	key := dbKey(common.NilAddress, Tesseract, tsHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetTesseract(tsHash common.Hash, data []byte) error {
	key := dbKey(common.NilAddress, Tesseract, tsHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) HasTesseract(tsHash common.Hash) bool {
	key := dbKey(common.NilAddress, Tesseract, tsHash.Bytes())

	exists, err := p.db.Has(key)
	if err != nil {
		p.logger.Error("Failed to check for tesseract", "err", err)
	}

	return exists
}

func (p *PersistenceManager) HasTesseractAt(addr common.Address, height uint64) bool {
	_, err := p.GetTesseractHeightEntry(addr, height)

	return err == nil
}

func (p *PersistenceManager) GetTesseractHeightEntry(addr common.Address, height uint64) ([]byte, error) {
	return p.ReadEntry(tesseractHeightKey(addr, height))
}

func (p *PersistenceManager) SetTesseractHeightEntry(addr common.Address, height uint64, tsHash common.Hash) error {
	return p.CreateEntry(tesseractHeightKey(addr, height), tsHash.Bytes())
}

// SetInteractions stores grid hash and raw interactions data as key value pair
func (p *PersistenceManager) SetInteractions(gridHash common.Hash, data []byte) error {
	key := dbKey(common.NilAddress, Interaction, gridHash.Bytes())

	return p.CreateEntry(key, data)
}

// GetInteractions returns raw interactions data for the given grid hash
func (p *PersistenceManager) GetInteractions(gridHash common.Hash) ([]byte, error) {
	key := dbKey(common.NilAddress, Interaction, gridHash.Bytes())

	return p.ReadEntry(key)
}

// SetTSGridLookup stores tesseract hash and grid hash as key value pair
func (p *PersistenceManager) SetTSGridLookup(tsHash common.Hash, gridHash common.Hash) error {
	key := dbKey(common.NilAddress, TSGridLookup, tsHash.Bytes())

	return p.CreateEntry(key, gridHash.Bytes())
}

// GetTSGridLookup returns raw grid hash for the given tesseract hash
func (p *PersistenceManager) GetTSGridLookup(tsHash common.Hash) ([]byte, error) {
	key := dbKey(common.NilAddress, TSGridLookup, tsHash.Bytes())

	return p.ReadEntry(key)
}

// SetIXGridLookup stores interaction hash and grid hash as key value pair
func (p *PersistenceManager) SetIXGridLookup(ixHash common.Hash, gridHash common.Hash) error {
	return p.CreateEntry(ixHash.Bytes(), gridHash.Bytes())
}

// GetIXGridLookup returns raw grid hash for the given interaction hash
func (p *PersistenceManager) GetIXGridLookup(ixHash common.Hash) ([]byte, error) {
	return p.ReadEntry(ixHash.Bytes())
}

// SetTesseractParts stores grid hash and tesseract parts as key value pair
func (p *PersistenceManager) SetTesseractParts(gridHash common.Hash, parts []byte) error {
	return p.CreateEntry(gridHash.Bytes(), parts)
}

// GetTesseractParts returns raw tesseract parts for the given grid hash
func (p *PersistenceManager) GetTesseractParts(gridHash common.Hash) ([]byte, error) {
	return p.ReadEntry(gridHash.Bytes())
}

// SetReceipts stores grid hash and raw receipt data as key value pair
func (p *PersistenceManager) SetReceipts(gridHash common.Hash, data []byte) error {
	key := dbKey(common.NilAddress, Receipt, gridHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) SetAccountSyncStatus(address common.Address, status *common.AccountSyncStatus) error {
	key := dbKey(common.NilAddress, AccountSyncJob, address.Bytes())

	rawData, err := status.Bytes()
	if err != nil {
		return err
	}

	return p.UpdateEntry(key, rawData)
}

func (p *PersistenceManager) CleanupAccountSyncStatus(address common.Address) error {
	key := dbKey(common.NilAddress, AccountSyncJob, address.Bytes())

	return p.DeleteEntry(key)
}

// GetReceipts returns raw receipt data for the given grid hash
func (p *PersistenceManager) GetReceipts(gridHash common.Hash) ([]byte, error) {
	key := dbKey(common.NilAddress, Receipt, gridHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetMerkleTreeEntry(
	address common.Address,
	prefix Prefix,
	actualKey []byte,
) ([]byte, error) {
	key := dbKey(address, prefix, actualKey)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetMerkleTreeEntry(
	address common.Address,
	prefix Prefix,
	actualKey, value []byte,
) error {
	key := dbKey(address, prefix, actualKey)

	return p.CreateEntry(key, value)
}

func (p *PersistenceManager) SetMerkleTreeEntries(
	address common.Address,
	prefix Prefix,
	entries map[string][]byte,
) error {
	// Create a batch writer
	batchWriter := p.NewBatchWriter()

	for k, v := range entries {
		key := dbKey(address, prefix, []byte(k))
		// Add to batch writer
		if err := batchWriter.Set(key, v); err != nil {
			return err
		}
	}

	return batchWriter.Flush()
}

func (p *PersistenceManager) WritePreImages(
	address common.Address,
	entries map[common.Hash][]byte,
) error {
	batchWriter := p.NewBatchWriter()

	for k, v := range entries {
		key := PreImageKey(address, k)
		// Add to batch writer
		if err := batchWriter.Set(key, v); err != nil {
			return err
		}
	}

	return batchWriter.Flush()
}

func (p *PersistenceManager) GetPreImage(
	address common.Address,
	hash common.Hash,
) ([]byte, error) {
	key := PreImageKey(address, hash)

	return p.ReadEntry(key)
}

// GetAccountSnapshot generates a snapshot of all entries with the given key prefix
// Snapshot contains all the entries with version > sinceTs
func (p *PersistenceManager) GetAccountSnapshot(
	ctx context.Context,
	address common.Address,
	sinceTS uint64,
) (*common.Snapshot, error) {
	kv := NewKVCollector(p.config.MaxSnapSize)

	err := p.db.Snapshot(ctx, address.Bytes(), sinceTS, kv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate snapshot")
	}

	s := &common.Snapshot{
		CreatedAt: utils.Canonical(time.Now()).UnixNano(),
		Prefix:    address.Bytes(),
		SinceTS:   sinceTS,
		Entries:   kv.Entries,
		Size:      kv.Size,
	}

	return s, nil
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

func (p *PersistenceManager) GetRegisteredAccounts() ([]common.Address, error) {
	addrsList := make([]common.Address, 0)

	for i := uint64(0); i < 1024; i++ {
		prefix := bucketPrefix(i)

		entries, err := p.GetEntriesWithPrefix(context.Background(), prefix)
		if err != nil {
			return nil, err
		}

		for entry := range entries {
			addr := entry.Key[9:]
			addrsList = append(addrsList, common.BytesToAddress(addr))
		}
	}

	return addrsList, nil
}

func (p *PersistenceManager) GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error) {
	syncInfos := make([]*common.AccountSyncStatus, 0)

	it, err := p.db.NewIterator()
	if err != nil {
		return nil, err
	}

	defer it.Close()

	for it.Seek([]byte{AccountSyncJob.Byte()}); it.ValidForPrefix([]byte{AccountSyncJob.Byte()}); it.Next() {
		dbEntry, err := it.GetNext()
		if err != nil {
			return nil, err
		}

		if len(dbEntry.Key) != 33 {
			continue
		}

		syncInfo := new(common.AccountSyncStatus)
		if err = syncInfo.FromBytes(dbEntry.Value); err != nil {
			return nil, err
		}

		syncInfos = append(syncInfos, syncInfo)
	}

	return syncInfos, nil
}

func (p *PersistenceManager) GetAssetRegistry(addr common.Address, registryHash common.Hash) ([]byte, error) {
	return p.ReadEntry(RegistryObjectKey(addr, registryHash))
}

func (p *PersistenceManager) DropPrefix(prefix []byte) error {
	return p.db.DropWithPrefix(prefix)
}

func (p *PersistenceManager) UpdatePrimarySyncStatus(address common.Address) error {
	return p.CreateEntry(AccSyncStatusKey(address), []byte{0x01})
}

func (p *PersistenceManager) IsAccountPrimarySyncDone(address common.Address) bool {
	isSynced, err := p.db.Has(AccSyncStatusKey(address))
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
