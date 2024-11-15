//nolint:nlreturn
package db

import (
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
	"github.com/pkg/errors"
)

// BadgerDB is an LSM based key-value store, It implements db.Database interface
type BadgerDB struct {
	db   *badger.DB
	root string
}

func NewBadgerDatabase(root string) (Database, error) {
	database, err := initBadgerDB(root)
	if err != nil {
		return nil, errors.Wrap(err, "database initialization failed")
	}

	return &BadgerDB{db: database, root: root}, nil
}

// initBadgerDB initiates BadgerDB at the given root directory
func initBadgerDB(root string) (*badger.DB, error) {
	dir := filepath.Join(root, "badger")

	opts := badger.DefaultOptions(dir)
	opts.IndexCacheSize = 100 << 20
	opts.Logger = nil

	return badger.Open(opts)
}

func (database *BadgerDB) Has(key []byte) (bool, error) {
	var exists bool

	if err := database.db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get(key); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				exists = false
				return nil
			}

			return err
		}

		exists = true
		return nil
	}); err != nil {
		return false, errors.Wrap(err, "could not read from badger db")
	}

	return exists, nil
}

func (database *BadgerDB) Get(key []byte) ([]byte, error) {
	var value []byte

	if err := database.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrKeyNotFound
			}

			return errors.Wrap(err, ErrPrefixOpFail)
		}

		return item.Value(func(val []byte) error {
			value = val
			return nil
		})
	}); err != nil {
		return nil, err
	}

	return value, nil
}

func (database *BadgerDB) Set(key, val []byte) error {
	if err := database.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, val)
	}); err != nil {
		return errors.Wrap(err, ErrPrefixOpFail)
	}

	return nil
}

func (database *BadgerDB) Del(key []byte) error {
	err := database.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return errors.Wrap(err, ErrPrefixOpFail)
	}

	return nil
}

// PrefixCollect retrieves all entries in BadgerDB with a given prefix
func (database *BadgerDB) PrefixCollect(prefix []byte) (map[string][]byte, error) {
	entries := make(map[string][]byte)

	err := database.db.View(func(txn *badger.Txn) error {
		// Iterate over keys with the given prefix
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.KeyCopy(nil))

			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			entries[key] = value
		}

		return nil
	})

	return entries, err
}

func (database *BadgerDB) PrefixDelete(prefix []byte) error {
	return database.db.DropPrefix(prefix)
}

func (database *BadgerDB) DropAll() error {
	return database.db.DropAll()
}

func (database *BadgerDB) Root() string {
	return database.root
}

func (database *BadgerDB) Close() error {
	return database.db.Close()
}
