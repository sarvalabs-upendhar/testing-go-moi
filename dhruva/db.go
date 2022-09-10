package dhruva

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/polo/go-polo"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"math/big"
)

const (
	BucketCount int64 = 1024
)

// PersistenceService defines all the methods to be implemented as part of DHRUVA package's persistence manager
type PersistenceService interface {
	CreateEntry([]byte, []byte, bool) error
	CreateCidEntry([]byte, bool) ([]byte, error)
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	Clean() error
}

// PersistenceManager manages all the critical information to perform content-addressed persistence services
type PersistenceManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	Config    *common.DBConfig
	db        *badger.DB
	logger    hclog.Logger
}

// initBadgerInstance initiates BadgerDB at give path
func initBadgerInstance(path string) (*badger.DB, error) {
	opts := badger.DefaultOptions(path) // Add .WithInMemory(true) for in-memory mode
	opts.IndexCacheSize = 100 << 20     // For better performance and encryption support
	opts.SyncWrites = true              // For write consistency, may affect performance

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// NewPersistenceManager is used by the caller to instantiate a PersistenceManager
func NewPersistenceManager(
	ctx context.Context,
	logger hclog.Logger,
	config *common.DBConfig,
) (*PersistenceManager, error) {
	db, err := initBadgerInstance(config.DBFolderPath)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrDBInit, err.Error())
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	p := &PersistenceManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		Config:    config,
		logger:    logger.Named("Dhruva"),
		db:        db,
	}

	return p, nil
}

func (p *PersistenceManager) GetBucketSizes() (map[int32]*big.Int, error) {
	buckets := make(map[int32]*big.Int, 0)

	for i := int64(0); i < BucketCount; i++ {
		bucketID := big.NewInt(i)
		bucket := make([]byte, 4)

		bucketID.FillBytes(bucket)

		if data, err := p.ReadEntry(bucket); err == nil {
			buckets[int32(i)] = new(big.Int).SetBytes(data)
		} else if !errors.Is(err, ktypes.ErrKeyNotFound) {
			return nil, err
		}
	}

	return buckets, nil
}

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
func (p *PersistenceManager) UpdateAccounts(acc ktypes.Accounts) (int32, int64) {
	var (
		bucketID  int32 = 0
		fullCount int64 = 0
	)

	for _, v := range acc {
		var (
			count int64
			err   error
		)

		bucketID, count, err = p.UpdateAccMetaInfo(
			v.Address,
			v.Height,
			v.TesseractHash,
			v.Type,
			false,
			false)
		if err != nil {
			continue
		}

		fullCount += count
	}

	return bucketID, fullCount
}

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
func (p *PersistenceManager) UpdateAccMetaInfo(
	id ktypes.Address,
	height *big.Int,
	tesseractHash ktypes.Hash,
	accType ktypes.AccType,
	latticeExists, stateExists bool,
) (int32, int64, error) {
	if id == ktypes.NilAddress {
		return 0, 0, ktypes.ErrInvalidAddress
	}

	if tesseractHash == ktypes.NilHash {
		return 0, 0, ktypes.ErrEmptyHash
	}

	key, bucket := BucketIDFromAddress(id.Bytes())

	data, err := p.ReadEntry(key)
	if err == nil {
		msg := new(ktypes.AccountMetaInfo)
		if err := polo.Depolorize(msg, data); err != nil {
			return -1, -1, err
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

		return int32(bucket.getID()), 0, p.UpdateEntry(key, polo.Polorize(msg))
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
			return -1, -1, err
		}

		if err = p.incrementBucketCount(bucket.getIDBytes(), 1); err != nil {
			log.Panic(err)
		}

		return int32(bucket.getID()), 1, nil
	}

	return -1, -1, err
}

// Close is a destructor of PersistenceManager instance
func (p *PersistenceManager) Close() {
	p.logger.Info("BadgerDB is shut down down gracefully.")
	defer p.ctxCancel()
	// close the channels

	if err := p.db.Close(); err != nil {
		p.logger.Error("Error closing the local BadgerDB instance", "error", err)
	}
}

// CreateEntry stores the given k-v entry in the local BadgerDB instance and returns the control back with error, if any
func (p *PersistenceManager) CreateEntry(key []byte, value []byte) error {
	// 1. Check if key already exists
	data, err := p.ReadEntry(key)
	if err == nil {
		if bytes.Equal(data, value) {
			return nil
		}

		return ktypes.ErrKeyExists
	} else if errors.Is(err, ktypes.ErrKeyNotFound) {
		// Create a new entry in Badger DB and store the k-v data
		if err = p.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, value)
		}); err != nil { // Handle any errors while creating a new entry
			return errors.Wrap(ktypes.ErrDBCallFailed, err.Error())
		}

		return nil
	}

	return errors.Wrap(ktypes.ErrDBCallFailed, err.Error())
}

func (p *PersistenceManager) SetTesseractStatus(
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

// CreateCidEntry computes CID for the given data, stores the CID and the data by calling CreateEntry
func (p *PersistenceManager) CreateCidEntry(value []byte) ([]byte, error) {
	// 1. Compute the CID for the given data
	pref := cid.Prefix{
		Version:  p.Config.CidPrefixVersion,
		Codec:    p.Config.CidPrefixCodec,
		MhType:   p.Config.CidPrefixMhType,
		MhLength: p.Config.CidPrefixMhLength,
	}

	newCid, err := pref.Sum(value)
	if err != nil {
		return newCid.Bytes(), status.Errorf(codes.Internal, fmt.Sprintf("Failed to create CID for the sent data: %v\n", err))
	}

	exists, _ := p.Contains(newCid.Bytes())
	if exists {
		return newCid.Bytes(), nil
	}
	// 2. Store the k-v entry in local Badger db instance
	if err = p.CreateEntry(newCid.Bytes(), value); err != nil {
		return nil, err
	}

	return newCid.Bytes(), nil
}

// UpdateEntry function is used to update the value for a given key
func (p *PersistenceManager) UpdateEntry(key []byte, newValue []byte) error {
	// 1. Read the current value stored under the key
	exists, err := p.Contains(key)
	if exists && err == nil {
		// 4. If not a CID, try to update the entry
		err = p.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, newValue)
		})
		if err != nil { // Handle errors if failed to update entry in db
			return errors.Wrap(ktypes.ErrDBCallFailed, err.Error())
		}
	}

	return err
}
func (p *PersistenceManager) GetAccounts(bucketID int32) (ktypes.Accounts, error) {
	var acc ktypes.Accounts

	err := p.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		//prefix := big.NewInt(int64(bucketID)).Bytes()

		bucketID := IDToBytes(int64(bucketID))
		for it.Seek(bucketID); it.ValidForPrefix(bucketID); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				if v != nil {
					msg := new(ktypes.AccountMetaInfo)
					if err := polo.Depolorize(msg, v); err != nil {
						return err
					}

					acc = append(acc, msg)
				}

				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return acc, nil
}

// ReadEntry takes the cid and returns corresponding content
func (p *PersistenceManager) ReadEntry(key []byte) ([]byte, error) {
	var value []byte

	// 1. Check if the entry for requested CID already exists in the local Badger DB instance. Return data if found.
	err := p.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == nil {
			return item.Value(func(val []byte) error {
				// Copying the value to return
				value = append([]byte{}, val...)

				return nil
			})
		} else if errors.Is(err, badger.ErrKeyNotFound) {
			return ktypes.ErrKeyNotFound
		}

		return errors.Wrap(ktypes.ErrDBCallFailed, err.Error())
	})
	if err != nil {
		return nil, err
	}
	// 2. Return the value

	return value, nil
}

// Contains is a light-weight function called to check for the presence of a k-v entry in the local Badgerdb instance
func (p *PersistenceManager) Contains(key []byte) (bool, error) {
	//1. Assume by default that key does not exist
	var entryExists bool

	// 2. Query for the entry by the given key
	err := p.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			// 3a. Entry does not exist!
			entryExists = false
		} else {
			// 3b. Entry exists!
			entryExists = true
		}

		return err
	})

	// 4. Check for any errors and handle them
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return false, status.Errorf(codes.Internal, fmt.Sprintf("Could not read from BadgerDB: %v\n", err))
	}

	// 5. Send the results
	return entryExists, nil
}

// DeleteEntry deletes the key-value entry from the local BadgerDB instance
func (p *PersistenceManager) DeleteEntry(key []byte) error {
	err := p.db.Update(func(txn *badger.Txn) error {
		// Delete the entry and commit the update transaction
		return txn.Delete(key)
	})
	if err != nil {
		return errors.Wrap(ktypes.ErrDBCallFailed, err.Error())
	}

	return nil
}

func (p *PersistenceManager) NewBatchWriter() *badger.WriteBatch {
	return p.db.NewWriteBatch()
}

func (p *PersistenceManager) Cleanup() error {
	return p.db.DropAll()
}

func (p *PersistenceManager) GetEntries(prefix []byte) chan ktypes.DBEntry {
	ch := make(chan ktypes.DBEntry)

	go func() {
		if err := p.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				err := item.Value(func(v []byte) error {
					if v != nil {
						ch <- ktypes.DBEntry{Key: item.Key(), Value: v}
					}

					return nil
				})
				if err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			p.logger.Error("Prefix Iteration failed", "error")
		}

		close(ch)
	}()

	return ch
}
