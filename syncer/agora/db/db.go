package db

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/syncer/cid"

	"github.com/sarvalabs/moichain/common"
	db "github.com/sarvalabs/moichain/storage"
	dhruva "github.com/sarvalabs/moichain/storage/db"
)

const (
	DefaultWorkerCount = 10
)

type BatchWriter interface {
	Set(key, value []byte) error
	Flush() error
}

type PersistenceManager interface {
	NewBatchWriter() dhruva.BatchWriter
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

			return
		case job := <-ds.jobs:
			job()
		}
	}
}

func (ds *DataStore) GetData(
	ctx context.Context,
	address common.Address,
	keys []cid.CID,
) (map[cid.CID][]byte, error) {
	res := make(map[cid.CID][]byte, len(keys))

	if len(keys) == 0 {
		return res, nil
	}

	var lk sync.Mutex

	return res, ds.jobPerKey(ctx, keys, func(c cid.CID) {
		blk, err := ds.db.ReadEntry(db.DBKey(address, db.Prefix(c.ContentType()), c.Key()))
		if err != nil {
			if errors.Is(err, common.ErrKeyNotFound) {
				ds.logger.Error("Key not found", "CID", c)
			}
		} else {
			lk.Lock()
			res[c] = blk
			lk.Unlock()
		}
	})
}

func (ds *DataStore) DoesStateExists(address common.Address, stateHash cid.CID) bool {
	keyExists, err := ds.db.Contains(db.AccountKey(address, common.BytesToHash(stateHash.Key())))
	if err != nil {
		ds.logger.Error("Error fetching state info from DB", "err", err)
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

func (ds *DataStore) jobPerKey(ctx context.Context, keys []cid.CID, jobFn func(key cid.CID)) error {
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
