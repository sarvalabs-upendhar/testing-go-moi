package dhruva

import (
	"context"
	"log"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	db "gitlab.com/sarvalabs/moichain/dhruva/db"
	"gitlab.com/sarvalabs/moichain/dhruva/db/badger"
	"gitlab.com/sarvalabs/polo/go-polo"
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
		return nil, errors.Wrap(ktypes.ErrDBInit, err.Error())
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
		} else if !errors.Is(err, ktypes.ErrKeyNotFound) {
			return nil, err
		}
	}

	return buckets, nil
}

// GetAccountMetaInfo fetches the account meta info for a given address
func (p *PersistenceManager) GetAccountMetaInfo(id []byte) (*ktypes.AccountMetaInfo, error) {
	key, _ := BucketIDFromAddress(id)

	data, err := p.ReadEntry(key)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrAccountNotFound, err.Error())
	}

	msg := new(ktypes.AccountMetaInfo)
	if err = polo.Depolorize(msg, data); err != nil {
		return nil, err
	}

	return msg, nil
}

// incrementBucketCount is used to increment bucket count when new address is added to chain
func (p *PersistenceManager) incrementBucketCount(id []byte, count int64) error {
	data, err := p.ReadEntry(id)
	if err == nil {
		updatedCount := new(big.Int).Add(big.NewInt(count), new(big.Int).SetBytes(data))

		return p.UpdateEntry(id, updatedCount.Bytes())
	} else if errors.Is(err, ktypes.ErrKeyNotFound) {
		return p.CreateEntry(id, big.NewInt(count).Bytes())
	}

	return err
}

// UpdateAccMetaInfo is used to update the meta-data of an account, this meta-data includes
// Height - Current height of the lattice
// StateExists - Does the latest state of account exists
// LatticeExists - Does complete lattice exists
func (p *PersistenceManager) UpdateAccMetaInfo(
	id ktypes.Address,
	height *big.Int,
	tesseractHash ktypes.Hash,
	accType ktypes.AccType,
	latticeExists, stateExists bool,
) (int32, bool, error) {
	if id == ktypes.NilAddress {
		return 0, false, ktypes.ErrInvalidAddress
	}

	if tesseractHash == ktypes.NilHash {
		return 0, false, ktypes.ErrEmptyHash
	}

	key, bucket := BucketIDFromAddress(id.Bytes())

	data, err := p.ReadEntry(key)
	if err == nil {
		msg := new(ktypes.AccountMetaInfo)
		if err := polo.Depolorize(msg, data); err != nil {
			return -1, false, err
		}

		if height.Cmp(msg.Height) == 0 && tesseractHash != msg.TesseractHash {
			return -1, false, ktypes.ErrHashMismatch
		}

		if height.Cmp(msg.Height) >= 0 {
			msg.StateExists = stateExists
			msg.TesseractHash = tesseractHash
			msg.Address = id
			msg.Height = height
		}

		if msg.LatticeExists {
			msg.LatticeExists = latticeExists
		}

		return int32(bucket.getID()), false, p.UpdateEntry(key, polo.Polorize(msg))
	} else if errors.Is(err, ktypes.ErrKeyNotFound) {
		msg := ktypes.AccountMetaInfo{
			StateExists:   stateExists,
			LatticeExists: latticeExists,
			TesseractHash: tesseractHash,
			Type:          accType,
			Address:       id,
			Height:        height,
		}

		if err = p.CreateEntry(key, polo.Polorize(msg)); err != nil {
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
	addr ktypes.Address,
	height uint64,
	hash ktypes.Hash,
	status bool,
) error {
	key, _ := BucketIDFromAddress(addr.Bytes())

	data, err := p.ReadEntry(key)
	if err != nil {
		return err
	}

	msg := new(ktypes.AccountMetaInfo)
	if err := polo.Depolorize(msg, data); err != nil {
		return err
	}

	if height < msg.Height.Uint64() {
		p.logger.Info("Skipping tesseract status update with less height")

		return nil
	}

	if hash == msg.TesseractHash {
		msg.StateExists = status
	} else {
		return ktypes.ErrHashMismatch
	}

	return p.UpdateEntry(key, polo.Polorize(msg))
}

// UpdateEntry updates the value associated with the given key
func (p *PersistenceManager) UpdateEntry(key []byte, newValue []byte) error {
	return p.db.Update(key, newValue)
}

// GetAccounts fetches meta info of all the accounts for a given bucket number
func (p *PersistenceManager) GetAccounts(bucketNumber int32) (ktypes.Accounts, error) {
	var acc ktypes.Accounts

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

		msg := new(ktypes.AccountMetaInfo)

		if err := polo.Depolorize(msg, dbEntry.Value); err != nil {
			return nil, err
		}

		acc = append(acc, msg)
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
func (p *PersistenceManager) GetEntries(prefix []byte) chan ktypes.DBEntry {
	ch := make(chan ktypes.DBEntry)

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

func (p *PersistenceManager) GetAccount(addr ktypes.Address, hash ktypes.Hash) ([]byte, error) {
	key := ktypes.DBKey(addr, ktypes.AccountGID, hash)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetBalance(addr ktypes.Address, hash ktypes.Hash) ([]byte, error) {
	key := ktypes.DBKey(addr, ktypes.BalanceGID, hash)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetContext(addr ktypes.Address, hash ktypes.Hash) ([]byte, error) {
	key := ktypes.DBKey(addr, ktypes.ContextGID, hash)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetStorage(addr ktypes.Address, hash ktypes.Hash) ([]byte, error) {
	key := ktypes.DBKey(addr, ktypes.StorageGID, hash)

	return p.ReadEntry(key)
}

func (p *PersistenceManager) GetTesseract(hash ktypes.Hash) ([]byte, error) {
	key := ktypes.DBKey(ktypes.NilAddress, ktypes.TesseractGID, hash)

	return p.ReadEntry(key)
}
