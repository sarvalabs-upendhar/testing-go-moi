package dhruva

import (
	"context"
	"log"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/dhruva/db/badger"
	"github.com/sarvalabs/moichain/types"
)

// PersistenceManager manages all the critical information to perform content-addressed persistence services
type PersistenceManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	Config    *common.DBConfig
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

	ctx, ctxCancel := context.WithCancel(ctx)
	p := &PersistenceManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		Config:    config,
		logger:    logger.Named("Dhruva"),
		db:        badgerDB,
	}

	return p, nil
}

func (p *PersistenceManager) getBucketCountByBucketNumber(bucketNumber []byte) (*big.Int, error) {
	val, err := p.ReadEntry(bucketNumber)
	if err != nil {
		return big.NewInt(-1), err
	}

	return new(big.Int).SetBytes(val), nil
}

// GetBucketSizes returns the accounts count for each bucket
func (p *PersistenceManager) GetBucketSizes() (map[int32]*big.Int, error) {
	buckets := make(map[int32]*big.Int, 0)

	for i := int64(0); i < BucketCount; i++ {
		bucketID := big.NewInt(i)
		bucket := make([]byte, 4)

		bucketID.FillBytes(bucket)

		if count, err := p.getBucketCountByBucketNumber(bucket); err == nil {
			buckets[int32(i)] = count
		} else if !errors.Is(err, types.ErrKeyNotFound) {
			return nil, err
		}
	}

	return buckets, nil
}

// GetAccountMetaInfo fetches the account meta info for a given address
func (p *PersistenceManager) GetAccountMetaInfo(id []byte) (*types.AccountMetaInfo, error) {
	key, _ := BucketIDFromAddress(id)

	data, err := p.ReadEntry(key)
	if err != nil {
		return nil, errors.Wrap(types.ErrAccountNotFound, err.Error())
	}

	accMetaInfo := new(types.AccountMetaInfo)
	if err := accMetaInfo.FromBytes(data); err != nil {
		return nil, err
	}

	return accMetaInfo, nil
}

// incrementBucketCount is used to increment bucket count when new address is added to lattice
func (p *PersistenceManager) incrementBucketCount(id []byte, count int64) error {
	data, err := p.ReadEntry(id)
	if err == nil {
		updatedCount := new(big.Int).Add(big.NewInt(count), new(big.Int).SetBytes(data))

		return p.UpdateEntry(id, updatedCount.Bytes())
	} else if errors.Is(err, types.ErrKeyNotFound) {
		return p.CreateEntry(id, big.NewInt(count).Bytes())
	}

	return err
}

// UpdateAccMetaInfo is used to update the meta-data of an account, this meta-data includes
// Height - Current height of the lattice
// StateExists - Does the latest state of account exists
// LatticeExists - Does complete lattice exists
func (p *PersistenceManager) UpdateAccMetaInfo(
	id types.Address,
	height *big.Int,
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

	key, bucket := BucketIDFromAddress(id.Bytes())

	data, err := p.ReadEntry(key)
	if err == nil {
		accMetaInfo := new(types.AccountMetaInfo)
		if err := accMetaInfo.FromBytes(data); err != nil {
			return -1, false, err
		}

		if height.Cmp(accMetaInfo.Height) == 0 && tesseractHash != accMetaInfo.TesseractHash {
			return -1, false, types.ErrHashMismatch
		}

		if height.Cmp(accMetaInfo.Height) >= 0 {
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

		return int32(bucket.getID()), false, p.UpdateEntry(key, rawData)
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

		if err = p.incrementBucketCount(bucket.getIDBytes(), 1); err != nil {
			log.Panic(err)
		}

		return int32(bucket.getID()), true, nil
	}

	return -1, false, err
}

// Close shutdowns the database
func (p *PersistenceManager) Close() {
	p.logger.Info("Closing the database")
	defer p.ctxCancel()
	// close the channels

	if err := p.db.Close(); err != nil {
		p.logger.Error("Error closing the local BadgerDB instance", "error", err)
	}
}

// CreateEntry stores the given k-v entry in database
func (p *PersistenceManager) CreateEntry(key []byte, value []byte) error {
	err := p.db.Insert(key, value)

	return err
}

// UpdateTesseractStatus is used to update the tesseract state after syncing
func (p *PersistenceManager) UpdateTesseractStatus(
	addr types.Address,
	height uint64,
	tsHash types.Hash,
	status bool,
) error {
	key, _ := BucketIDFromAddress(addr.Bytes())

	data, err := p.ReadEntry(key)
	if err != nil {
		return err
	}

	accMetaInfo := new(types.AccountMetaInfo)
	if err := accMetaInfo.FromBytes(data); err != nil {
		return err
	}

	if height < accMetaInfo.Height.Uint64() {
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

// GetAccounts fetches meta info of all the accounts for a given bucket number
func (p *PersistenceManager) GetAccounts(bucketNumber int32) (types.Accounts, error) {
	var acc types.Accounts

	it, err := p.db.NewIterator()
	if err != nil {
		return nil, err
	}

	defer it.Close()

	bucketID := IDToBytes(int64(bucketNumber))
	for it.Seek(bucketID); it.ValidForPrefix(bucketID); it.Next() {
		dbEntry, err := it.GetNext()
		if err != nil {
			return nil, err
		}

		accMetaInfo := new(types.AccountMetaInfo)

		if err := accMetaInfo.FromBytes(dbEntry.Value); err != nil {
			return nil, err
		}

		acc = append(acc, accMetaInfo)
	}

	return acc, nil
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

// GetEntries fetches array of k,v pair given a prefix key
func (p *PersistenceManager) GetEntries(prefix []byte) chan types.DBEntry {
	ch := make(chan types.DBEntry)

	go func() {
		it, err := p.db.NewIterator()
		if err != nil {
			p.logger.Error("Prefix Iteration failed", "error")
		} else {
			defer it.Close()

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				dbEntry, err := it.GetNext()
				if err != nil {
					p.logger.Error("Prefix Iteration failed", "error")

					break
				}
				ch <- *dbEntry
			}
		}

		close(ch)
	}()

	return ch
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

func (p *PersistenceManager) HasTesseract(tsHash types.Hash) (bool, error) {
	key := dbKey(types.NilAddress, Tesseract, tsHash.Bytes())

	return p.db.Has(key)
}

func (p *PersistenceManager) GetTesseractHeightEntry(addr types.Address, height uint64) ([]byte, error) {
	return p.ReadEntry(tesseractHeightKey(addr, height))
}

func (p *PersistenceManager) SetTesseractHeightEntry(addr types.Address, height uint64, tsHash types.Hash) error {
	return p.CreateEntry(tesseractHeightKey(addr, height), tsHash.Bytes())
}

func (p *PersistenceManager) GetInteractions(ixHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, Interaction, ixHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetInteractions(ixHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, Interaction, ixHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) GetIxLookup(ixHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, IxLookup, ixHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetIxLookup(ixHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, IxLookup, ixHash.Bytes())

	return p.CreateEntry(key, data)
}

func (p *PersistenceManager) GetReceipts(receiptHash types.Hash) ([]byte, error) {
	key := dbKey(types.NilAddress, Receipt, receiptHash.Bytes())

	return p.ReadEntry(key)
}

func (p *PersistenceManager) SetReceipts(receiptHash types.Hash, data []byte) error {
	key := dbKey(types.NilAddress, Receipt, receiptHash.Bytes())

	return p.CreateEntry(key, data)
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
