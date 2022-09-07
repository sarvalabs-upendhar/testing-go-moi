package db

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-hclog"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
)

const (
	DefaultWorkerCount = 10
)

type BatchWriter interface {
	Set(key, value []byte) error
	Flush() error
}

type PersistenceManager interface {
	NewBatchWriter() *badger.WriteBatch
	Contains(key []byte) (bool, error)
	ReadEntry([]byte) ([]byte, error)
}
type DataStore struct {
	ctx         context.Context
	logger      hclog.Logger
	db          PersistenceManager
	workerCount int
	workerLock  sync.Mutex
	jobs        chan func()
}

func NewDataStore(ctx context.Context, logger hclog.Logger, db PersistenceManager) *DataStore {
	return &DataStore{
		ctx:         ctx,
		logger:      logger.Named("DataStore"),
		db:          db,
		workerCount: DefaultWorkerCount,
		jobs:        make(chan func()),
	}
}

func (ds *DataStore) worker() {
	defer func() {
		ds.workerLock.Lock()
		defer ds.workerLock.Unlock()
		ds.workerCount--
	}()

	for {
		select {
		case <-ds.ctx.Done():
			ds.logger.Info("Closing data store worker")
		case job := <-ds.jobs:
			job()
		}
	}
}
func (ds *DataStore) GetData(ctx context.Context, keys []ktypes.Hash) ([][]byte, error) {
	res := make([][]byte, 0, len(keys))
	if len(keys) == 0 {
		return res, nil
	}

	var lk sync.Mutex

	return res, ds.jobPerKey(ctx, keys, func(c ktypes.Hash) {
		blk, err := ds.db.ReadEntry(c.Bytes())
		if err != nil {
			if errors.Is(err, ktypes.ErrKeyNotFound) {
				ds.logger.Error("Key not found", "id", c.Hex())
			}
		} else {
			lk.Lock()
			res = append(res, blk)
			lk.Unlock()
		}
	})
}

func (ds *DataStore) DoesStateExists(stateHash ktypes.Hash) bool {
	keyExists, err := ds.db.Contains(stateHash.Bytes())
	if err != nil {
		ds.logger.Error("Error fetching state info from db", "error", err)
	}

	return keyExists
}

func (ds *DataStore) Get(key []byte) ([]byte, error) {
	return ds.db.ReadEntry(key)
}

func (ds *DataStore) GetBatchWriter() BatchWriter {
	return ds.db.NewBatchWriter()
}

func (ds *DataStore) addJob(ctx context.Context, job func()) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ds.ctx.Done():
		return fmt.Errorf("shutting down")
	case ds.jobs <- job:
		return nil
	}
}

func (ds *DataStore) jobPerKey(ctx context.Context, keys []ktypes.Hash, jobFn func(key ktypes.Hash)) error {
	var err error

	wg := sync.WaitGroup{}

	for _, k := range keys {
		c := k

		wg.Add(1)

		err = ds.addJob(ctx, func() {
			jobFn(c)
			wg.Done()
		})

		if err != nil {
			wg.Done()

			break
		}
	}

	wg.Wait()

	return err
}

func (ds *DataStore) Start() {
	ds.workerLock.Lock()
	defer ds.workerLock.Unlock()

	for i := 0; i < ds.workerCount; i++ {
		go ds.worker()
	}
}
