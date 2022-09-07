package decision

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/db"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/polo/go-polo"
	"sync"
	"time"
)

var (
	AgoraPrefix = []byte("agora")
)

func GetAgoraKey(hash ktypes.Hash) string {
	out := append(AgoraPrefix, hash.Bytes()...)

	return hex.EncodeToString(out)
}

func GetAgoraDBKey(address ktypes.Address, hash ktypes.Hash) []byte {
	out := append(AgoraPrefix, address.Bytes()[0:20]...)
	out = append(out, hash.Bytes()...)

	return out
}

type ledgerStore interface {
	Get(key []byte) ([]byte, error)
	GetBatchWriter() db.BatchWriter
}

type job struct {
	key   []byte
	value *types.CanonicalPeerList
}

type Ledger struct {
	ctx          context.Context
	logger       hclog.Logger
	db           ledgerStore
	cache        *lru.Cache
	dbJobsLock   sync.Mutex
	workersLock  sync.Mutex
	workersCount int
	dbJobs       []*job
}

func NewLedger(ctx context.Context, logger hclog.Logger, workersCount int, db ledgerStore) (*Ledger, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, err
	}

	l := &Ledger{
		ctx:          ctx,
		logger:       logger.Named("Ledger"),
		db:           db,
		cache:        cache,
		workersCount: workersCount,
		dbJobs:       make([]*job, 0),
	}

	return l, err
}

func (l *Ledger) GetAssociatedPeers(addr ktypes.Address, stateHash ktypes.Hash) ([]id.KramaID, error) {
	key := GetAgoraKey(stateHash)

	peerList, cacheErr := l.fetchFromCache(key)
	if cacheErr == nil {
		return peerList.Peers(), cacheErr
	}

	peerList, err := l.fetchFromDB(addr, stateHash)
	if err != nil {
		return nil, err
	}

	if peerList.Size() > 0 {
		l.addToCache(GetAgoraKey(stateHash), peerList)
	}

	return peerList.Peers(), nil
}

func (l *Ledger) addToCache(key string, list *types.PeerList) {
	l.cache.Add(key, list)
}

func (l *Ledger) fetchFromCache(key string) (*types.PeerList, error) {
	data, ok := l.cache.Get(key)
	if !ok {
		return nil, ktypes.ErrKeyNotFound
	}

	peerList, ok := data.(*types.PeerList)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return peerList, nil
}

func (l *Ledger) fetchFromDB(address ktypes.Address, stateHash ktypes.Hash) (*types.PeerList, error) {
	rawData, err := l.db.Get(GetAgoraDBKey(address, stateHash))
	if err != nil {
		return nil, ktypes.ErrKeyNotFound
	}

	plist := new(types.CanonicalPeerList)
	if err := polo.Depolorize(plist, rawData); err != nil {
		return nil, err
	}

	return plist.PeerList(), nil
}

func (l *Ledger) UpdateAssociatedPeers(address ktypes.Address, stateHash ktypes.Hash, peerID id.KramaID) (err error) {
	peerList, cacheErr := l.fetchFromCache(GetAgoraKey(stateHash))
	if errors.Is(cacheErr, ktypes.ErrKeyNotFound) {
		peerList, err = l.fetchFromDB(address, stateHash)

		if errors.Is(err, ktypes.ErrKeyNotFound) {
			peerList = types.NewPeerList()
		} else if err != nil {
			return err
		}

		l.addToCache(GetAgoraKey(stateHash), peerList)
	} else if cacheErr != nil {
		return cacheErr
	}

	peerList.AddPeer(peerID)

	l.dbJobsLock.Lock()
	defer l.dbJobsLock.Unlock()

	l.dbJobs = append(l.dbJobs, &job{
		key:   GetAgoraDBKey(address, stateHash),
		value: peerList.CanonicalPeerList(),
	})

	return nil
}

func (l *Ledger) Start() {
	l.workersLock.Lock()
	defer l.workersLock.Unlock()

	for i := 0; i < l.workersCount; i++ {
		go l.worker()
	}
}

func (l *Ledger) worker() {
	defer func() {
		l.workersLock.Lock()
		l.workersCount--
		l.workersLock.Unlock()
	}()

	for {
		select {
		case <-l.ctx.Done():
			l.logger.Info("Context expired closing worker")

			return
		case <-time.After(2 * time.Second):
		}

		l.dbJobsLock.Lock()
		jobs := l.dbJobs[:]
		l.dbJobs = l.dbJobs[len(jobs):]
		l.dbJobsLock.Unlock()

		if len(jobs) > 0 {
			dbWriter := l.db.GetBatchWriter()

			for _, job := range jobs {
				if err := dbWriter.Set(job.key, polo.Polorize(job.value)); err != nil {
					l.logger.Error("Error adding associated peer list to db")

					continue
				}
			}
		}
	}
}
