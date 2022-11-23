package db

import (
	"context"
	"errors"
	"fmt"
	"sync"

	db "gitlab.com/sarvalabs/moichain/dhruva"

	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"

	"github.com/hashicorp/go-hclog"
	dhruva "gitlab.com/sarvalabs/moichain/dhruva/db"
	"gitlab.com/sarvalabs/moichain/types"
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
		case job := <-ds.jobs:
			job()
		}
	}
}

func (ds *DataStore) GetData(
	ctx context.Context,
	address types.Address,
	keys []atypes.CID,
) (map[atypes.CID][]byte, error) {
	res := make(map[atypes.CID][]byte, len(keys))

	if len(keys) == 0 {
		return res, nil
	}

	var lk sync.Mutex

	return res, ds.jobPerKey(ctx, keys, func(c atypes.CID) {
		blk, err := ds.db.ReadEntry(db.DBKey(address, db.Prefix(c.ContentType()), c.Key()))
		if err != nil {
			if errors.Is(err, types.ErrKeyNotFound) {
				ds.logger.Error("Key not found", "id", c)
			}
		} else {
			lk.Lock()
			res[c] = blk
			lk.Unlock()
		}
	})
}

func (ds *DataStore) DoesStateExists(address types.Address, stateHash atypes.CID) bool {
	keyExists, err := ds.db.Contains(db.AccountKey(address, types.BytesToHash(stateHash.Key())))
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

func (ds *DataStore) jobPerKey(ctx context.Context, keys []atypes.CID, jobFn func(key atypes.CID)) error {
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
