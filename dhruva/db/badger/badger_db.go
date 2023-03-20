package badger

import (
	"bytes"

	"github.com/dgraph-io/badger/v3"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/types"
)

// BadgerDB is a LSM based key-value store, It implements db.DB interface
type BadgerDB struct {
	db *badger.DB
}

// initBadgerInstance initiates BadgerDB at give path
func initBadgerInstance(path string) (*badger.DB, error) {
	opts := badger.DefaultOptions(path) // Add .WithInMemory(true) for in-memory mode
	opts.IndexCacheSize = 100 << 20     // For better performance and encryption support

	return badger.Open(opts)
}

// NewBadgerDB returns a badger db instance which implements DB interface
func NewBadgerDB(path string) (db.DB, error) {
	database, err := initBadgerInstance(path)
	if err != nil {
		return nil, errors.Wrap(types.ErrDBInit, err.Error())
	}

	return &BadgerDB{db: database}, nil
}

// NewIterator returns badger iterator which implements db.Iterator interface
func (b *BadgerDB) NewIterator() (db.Iterator, error) {
	if b.db.IsClosed() {
		return nil, badger.ErrDBClosed
	}

	txn := b.db.NewTransaction(false)
	it := txn.NewIterator(badger.DefaultIteratorOptions)

	return &Iterator{it: it, txn: txn}, nil
}

// NewBatchWriter returns a badger write batch instance
func (b *BadgerDB) NewBatchWriter() db.BatchWriter {
	return &BatchWriter{
		bw: b.db.NewWriteBatch(),
	}
}

// Insert stores the give key-value to badger DB, entries are not synced to disk immediately
func (b *BadgerDB) Insert(key []byte, value []byte) error {
	// 1. Check if key already exists
	data, err := b.Get(key)
	if err == nil {
		if bytes.Equal(data, value) {
			return nil
		}

		return types.ErrKeyExists
	} else if errors.Is(err, types.ErrKeyNotFound) {
		// Create a new entry in Badger DB and store the k-v data
		if err = b.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, value)
		}); err != nil { // Handle any errors while creating a new entry
			return errors.Wrap(types.ErrDBCallFailed, err.Error())
		}

		return nil
	}

	return errors.Wrap(types.ErrDBCallFailed, err.Error())
}

// Has check's for the given key in the database
func (b *BadgerDB) Has(key []byte) (bool, error) {
	// 1. Assume by default that key does not exist
	var entryExists bool

	// 2. Query for the entry by the given key
	err := b.db.View(func(txn *badger.Txn) error {
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
		return false, errors.Wrap(err, "Could not read from BadgerDB")
	}

	// 5. Send the results
	return entryExists, nil
}

func (b *BadgerDB) Update(key []byte, value []byte) error {
	// 1. Read the current value stored under the key
	exists, err := b.Has(key)
	if exists && err == nil {
		// 4. If key exists try to update the entry
		err = b.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, value)
		})
		if err != nil { // Handle errors if failed to update entry in db
			return errors.Wrap(types.ErrDBCallFailed, err.Error())
		}
	}

	return err
}

func (b *BadgerDB) Delete(key []byte) error {
	err := b.db.Update(func(txn *badger.Txn) error {
		// Delete the entry and commit the update transaction
		return txn.Delete(key)
	})
	if err != nil {
		return errors.Wrap(types.ErrDBCallFailed, err.Error())
	}

	return nil
}

// Get returns the value for the given key. It returns ErrKeyNotFound if the database does not contain the key
func (b *BadgerDB) Get(key []byte) ([]byte, error) {
	var value []byte

	// 1. Check if the entry for requested CID already exists in the local Badger DB instance. Return data if found.
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == nil {
			return item.Value(func(val []byte) error {
				// Copying the value to return
				value = append([]byte{}, val...)

				return nil
			})
		} else if errors.Is(err, badger.ErrKeyNotFound) {
			return types.ErrKeyNotFound
		}

		return errors.Wrap(types.ErrDBCallFailed, err.Error())
	})
	if err != nil {
		return nil, err
	}
	// 2. Return the value

	return value, nil
}

// CleanUp will delete all the data stored in the database
func (b *BadgerDB) CleanUp() error {
	return b.db.DropAll()
}

// Close graceful shutdowns the badger database
func (b *BadgerDB) Close() error {
	return b.db.Close()
}
