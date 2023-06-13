package dhruva

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/dhruva/db/badger"
	"github.com/sarvalabs/moichain/types"
)

// MaxBucketCount tells the no of buckets , accounts can be classified into
const (
	MaxBucketCount uint64 = 1024
)

// PersistenceManager manages all the critical information to perform content-addressed persistence services
type PersistenceManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	config    *common.DBConfig
	db        db.DB
	logger    hclog.Logger
}

// NewPersistenceManager is used by the caller to instantiate a PersistenceManager
func NewPersistenceManager(
	ctx context.Context,
	logger hclog.Logger,
	config *common.DBConfig,
) (*PersistenceManager, error) {
	badgerDB, err := badger.NewBadgerDB(config.DBFolderPath)
	if err != nil {
		return nil, errors.Wrap(types.ErrDBInit, err.Error())
	}

	if config.CleanDB {
		if err = badgerDB.CleanUp(); err != nil {
			panic(err)
		}
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	p := &PersistenceManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		config:    config,
		logger:    logger.Named("Dhruva"),
		db:        badgerDB,
	}

	return p, nil
}

func (p *PersistenceManager) GetBucketCount(bucketNumber uint64) (uint64, error) {
	val, err := p.ReadEntry(bucketCountKey(bucketNumber))
	if err == types.ErrKeyNotFound { //nolint
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
		if !errors.Is(err, types.ErrKeyNotFound) {
			return nil, err
		}

		if err == nil {
			buckets[i] = count
		}
	}

	return buckets, nil
}

// GetAccountMetaInfo fetches the account meta info for a given address
func (p *PersistenceManager) GetAccountMetaInfo(id types.Address) (*types.AccountMetaInfo, error) {
	key, _ := BucketKeyAndID(id)

	data, err := p.ReadEntry(key)
	if err != nil {
		return nil, types.ErrAccountNotFound
	}

	accMetaInfo := new(types.AccountMetaInfo)
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
	} else if errors.Is(err, types.ErrKeyNotFound) {
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
	id types.Address,
	height uint64,
	tesseractHash types.Hash,
	accType types.AccountType,
	latticeExists, stateExists bool,
) (int32, bool, error) {
	if id.IsNil() {
		return 0, false, types.ErrInvalidAddress
	}

	if tesseractHash.IsNil() {
		return 0, false, types.ErrEmptyHash
	}

	key, bucketID := BucketKeyAndID(id)

	data, err := p.ReadEntry(key)
	if err == nil {
		accMetaInfo := new(types.AccountMetaInfo)
		if err := accMetaInfo.FromBytes(data); err != nil {
			return -1, false, err
		}

		if height == accMetaInfo.Height && tesseractHash != accMetaInfo.TesseractHash {
			return -1, false, types.ErrHashMismatch
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
	} else if errors.Is(err, types.ErrKeyNotFound) {
		msg := types.AccountMetaInfo{
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
	p.logger.Info("Closing the database")
	// close the channels

	if err := p.db.Close(); err != nil {
		p.logger.Error("Error closing the local BadgerDB instance", "error", err)
	}
}

// CreateEntry stores the given k-v entry in database
func (p *PersistenceManager) CreateEntry(key []byte, value []byte) error {
	return p.db.Insert(key, value)
}

// UpdateTesseractStatus is used to update the tesseract state after syncing
func (p *PersistenceManager) UpdateTesseractStatus(
	addr types.Address,
	height uint64,
	tsHash types.Hash,
	status bool,
) error {
	key, _ := BucketKeyAndID(addr)

	data, err := p.ReadEntry(key)
	if err != nil {
		return err
	}

	accMetaInfo := new(types.AccountMetaInfo)
	if err := accMetaInfo.FromBytes(data); err != nil {
		return err
	}

	if height < accMetaInfo.Height {
		return nil
	}

	if tsHash == accMetaInfo.TesseractHash {
		accMetaInfo.StateExists = status
	} else {
		return types.ErrHashMismatch
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
func (p *PersistenceManager) GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *types.DBEntry, error) {
	ch := make(chan *types.DBEntry)

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
				p.logger.Error("Prefix iteration failed", "error")

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

func (p *PersistenceManager) SetAccount(addr types.Address, stateHash types.Hash, data []byte) error {
	key := dbKey(addr, Account, stateHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) GetAccount(addr types.Address, stateHash types.Hash) ([]byte, error) {
	key := dbKey(addr, Account, stateHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetBalance(addr types.Address, balanceHash types.Hash) ([]byte, error) {
	key := dbKey(addr, Balance, balanceHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetContext(addr types.Address, contextHash types.Hash) ([]byte, error) {
	key := dbKey(addr, Context, contextHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetStorage(addr types.Address, hash types.Hash) ([]byte, error) {
	key := dbKey(addr, Storage, hash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetTesseract(tsHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, Tesseract, tsHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetTesseract(tsHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, Tesseract, tsHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) HasTesseract(tsHash types.Hash) bool {
	key := dbKey(types.NilAddress, Tesseract, tsHash.Bytes())

	exists, err := p.db.Has(key)
	if err != nil {
		p.logger.Error("Failed to check for tesseract", "error", err)
	}

	return exists
}

func (p *PersistenceManager) HasTesseractAt(addr types.Address, height uint64) bool {
	_, err := p.GetTesseractHeightEntry(addr, height)

	return err == nil
}

func (p *PersistenceManager) GetTesseractHeightEntry(addr types.Address, height uint64) ([]byte, error) {
	return p.ReadEntry(tesseractHeightKey(addr, height))
}

func (p *PersistenceManager) SetTesseractHeightEntry(addr types.Address, height uint64, tsHash types.Hash) error {
	return p.CreateEntry(tesseractHeightKey(addr, height), tsHash.Bytes())
}

// SetInteractions stores grid hash and raw interactions data as key value pair
func (p *PersistenceManager) SetInteractions(gridHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, Interaction, gridHash.Bytes())

	return p.CreateEntry(key, data)
}

// GetInteractions returns raw interactions data for the given grid hash
func (p *PersistenceManager) GetInteractions(gridHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, Interaction, gridHash.Bytes())

	return p.ReadEntry(key)
}

// SetTSGridLookup stores tesseract hash and grid hash as key value pair
func (p *PersistenceManager) SetTSGridLookup(tsHash types.Hash, gridHash types.Hash) error {
	key := dbKey(types.NilAddress, TSGridLookup, tsHash.Bytes())

	return p.CreateEntry(key, gridHash.Bytes())
}

// GetTSGridLookup returns raw grid hash for the given tesseract hash
func (p *PersistenceManager) GetTSGridLookup(tsHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, TSGridLookup, tsHash.Bytes())

	return p.ReadEntry(key)
}

// SetIXGridLookup stores interaction hash and grid hash as key value pair
func (p *PersistenceManager) SetIXGridLookup(ixHash types.Hash, gridHash types.Hash) error {
	return p.CreateEntry(ixHash.Bytes(), gridHash.Bytes())
}

// GetIXGridLookup returns raw grid hash for the given interaction hash
func (p *PersistenceManager) GetIXGridLookup(ixHash types.Hash) ([]byte, error) {
	return p.ReadEntry(ixHash.Bytes())
}

// SetTesseractParts stores grid hash and tesseract parts as key value pair
func (p *PersistenceManager) SetTesseractParts(gridHash types.Hash, parts []byte) error {
	return p.CreateEntry(gridHash.Bytes(), parts)
}

// GetTesseractParts returns raw tesseract parts for the given grid hash
func (p *PersistenceManager) GetTesseractParts(gridHash types.Hash) ([]byte, error) {
	return p.ReadEntry(gridHash.Bytes())
}

// SetReceipts stores grid hash and raw receipt data as key value pair
func (p *PersistenceManager) SetReceipts(gridHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, Receipt, gridHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) SetAccountSyncStatus(address types.Address, status *types.AccountSyncStatus) error {
	key := dbKey(types.NilAddress, AccountSyncJob, address.Bytes())

	rawData, err := status.Bytes()
	if err != nil {
		return err
	}

	return p.UpdateEntry(key, rawData)
}

func (p *PersistenceManager) CleanupAccountSyncStatus(address types.Address) error {
	key := dbKey(types.NilAddress, AccountSyncJob, address.Bytes())

	return p.DeleteEntry(key)
}

// GetReceipts returns raw receipt data for the given grid hash
func (p *PersistenceManager) GetReceipts(gridHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, Receipt, gridHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetMerkleTreeEntry(
	address types.Address,
	prefix Prefix,
	actualKey []byte,
) ([]byte, error) {
	key := dbKey(address, prefix, actualKey)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetMerkleTreeEntry(
	address types.Address,
	prefix Prefix,
	actualKey, value []byte,
) error {
	key := dbKey(address, prefix, actualKey)

	return p.CreateEntry(key, value)
}

func (p *PersistenceManager) SetMerkleTreeEntries(
	address types.Address,
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
	address types.Address,
	entries map[types.Hash][]byte,
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
	address types.Address,
	hash types.Hash,
) ([]byte, error) {
	key := PreImageKey(address, hash)

	return p.ReadEntry(key)
}

// GetAccountSnapshot generates a snapshot of all entries with the given key prefix
// Snapshot contains all the entries with version > sinceTs
func (p *PersistenceManager) GetAccountSnapshot(
	ctx context.Context,
	address types.Address,
	sinceTS uint64,
) (*types.Snapshot, error) {
	kv := NewKVCollector(p.config.MaxSnapSize)

	err := p.db.Snapshot(ctx, address.Bytes(), sinceTS, kv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate snapshot")
	}

	s := &types.Snapshot{
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

func (p *PersistenceManager) StoreAccountSnapShot(snap *types.Snapshot) error {
	batchWriter := p.db.NewBatchWriter()

	if err := batchWriter.WriteBuffer(snap.Entries); err != nil {
		return err
	}

	if err := batchWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func (p *PersistenceManager) GetRegisteredAccounts() ([]types.Address, error) {
	addrsList := make([]types.Address, 0)

	for i := uint64(0); i < 1024; i++ {
		prefix := bucketPrefix(i)

		entries, err := p.GetEntriesWithPrefix(context.Background(), prefix)
		if err != nil {
			return nil, err
		}

		for entry := range entries {
			addr := entry.Key[9:]
			addrsList = append(addrsList, types.BytesToAddress(addr))
		}
	}

	return addrsList, nil
}

func (p *PersistenceManager) GetAccountsSyncStatus() ([]*types.AccountSyncStatus, error) {
	syncInfos := make([]*types.AccountSyncStatus, 0)

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

		syncInfo := new(types.AccountSyncStatus)
		if err = syncInfo.FromBytes(dbEntry.Value); err != nil {
			return nil, err
		}

		syncInfos = append(syncInfos, syncInfo)
	}

	return syncInfos, nil
}

func (p *PersistenceManager) GetAssetRegistry(addr types.Address, registryHash types.Hash) ([]byte, error) {
	return p.ReadEntry(RegistryObjectKey(addr, registryHash))
}

func (p *PersistenceManager) DropPrefix(prefix []byte) error {
	return p.db.DropWithPrefix(prefix)
}

func (p *PersistenceManager) UpdatePrimarySyncStatus(address types.Address) error {
	return p.CreateEntry(AccSyncStatusKey(address), []byte{0x01})
}

func (p *PersistenceManager) IsAccountPrimarySyncDone(address types.Address) bool {
	isSynced, err := p.db.Has(AccSyncStatusKey(address))
	if err != nil {
		p.logger.Error("Error checking account sync status", "error", err)
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
