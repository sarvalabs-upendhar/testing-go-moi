package dhruva

/*
import (
	"github.com/dgraph-io/badger/v3"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

type InMemoryDB struct {
	db *badger.DB
}

func NewInMemoryDB() (*InMemoryDB, error) {
	var err error
	memDB := new(InMemoryDB)
	if memDB.db, err = badger.Open(badger.DefaultOptions("").WithInMemory(true)); err != nil {
		return nil, errors.Wrap(ktypes.ErrDBInit, err.Error())
	}

	return memDB, nil
}

func (d *InMemoryDB) CreateCIDEntry(value []byte) ([]byte, error) {
	cID, err := getCid(value)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrDBInsertFailed, err.Error())
	}

	return cID.Bytes(), d.Set(cID.Bytes(), value)
}

func (d *InMemoryDB) Set(key, value []byte) error {
	err := d.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})

	return err
}

// Get returns the value associated with the given key
// returns ErrKeyNotFound if key doesn't exist
func (d *InMemoryDB) Get(key []byte) (val []byte, err error) {
	err = d.db.View(func(txn *badger.Txn) error {
		item, txnErr := txn.Get(key)
		if txnErr != nil && txnErr == badger.ErrKeyNotFound {
			return ktypes.ErrKeyNotFound
		} else if txnErr != nil {
			return ktypes.ErrDBCallFailed
		}

		err = item.Value(func(value []byte) error {
			val = append([]byte{}, value...)

			return nil
		})

		return err
	})

	return
}

func (d *InMemoryDB) Delete(key []byte) error {
	err := d.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (d *InMemoryDB) Contains(key []byte) bool {
	err := d.db.View(func(txn *badger.Txn) error {
		_, txnErr := txn.Get(key)
		if txnErr != nil && txnErr == badger.ErrKeyNotFound {
			return ktypes.ErrKeyNotFound
		} else if txnErr != nil {
			return ktypes.ErrDBCallFailed
		}

		return nil
	})

	return err == nil
}

func (d *InMemoryDB) Close() error {
	return d.db.Close()
}
*/
