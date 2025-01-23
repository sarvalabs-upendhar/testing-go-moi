package decision

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/syncer/agora/db"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

var AgoraPrefix = []byte("agora")

func GetAgoraKey(key []byte) string {
	out := AgoraPrefix
	out = append(out, key...)

	return hex.EncodeToString(out)
}

func GetAgoraDBKey(id identifiers.Identifier, key []byte) []byte {
	out := AgoraPrefix
	out = append(out, id.Bytes()[4:28]...)
	out = append(out, key...)

	return out
}

type ledgerStore interface {
	Get(key []byte) ([]byte, error)
	GetBatchWriter() db.BatchWriter
}

type job struct {
	key   []byte
	value *CanonicalPeerList
}

type Ledger struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	logger       hclog.Logger
	db           ledgerStore
	cache        *lru.Cache
	dbJobsLock   sync.Mutex
	workersLock  sync.Mutex
	workersCount int
	dbJobs       []*job
}

func NewLedger(logger hclog.Logger, workersCount int, db ledgerStore) (*Ledger, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	l := &Ledger{
		ctx:          ctx,
		ctxCancel:    cancel,
		logger:       logger.Named("Ledger"),
		db:           db,
		cache:        cache,
		workersCount: workersCount,
		dbJobs:       make([]*job, 0),
	}

	return l, err
}

func (l *Ledger) GetAssociatedPeers(id identifiers.Identifier, stateHash cid.CID) ([]kramaid.KramaID, error) {
	key := GetAgoraKey(stateHash.Key())

	peerList, cacheErr := l.fetchFromCache(key)
	if cacheErr == nil {
		return peerList.Peers(), cacheErr
	}

	peerList, err := l.fetchFromDB(id, stateHash)
	if err != nil {
		return nil, err
	}

	if peerList.Size() > 0 {
		l.addToCache(GetAgoraKey(stateHash.Key()), peerList)
	}

	return peerList.Peers(), nil
}

func (l *Ledger) addToCache(key string, list *PeerList) {
	l.cache.Add(key, list)
}

func (l *Ledger) fetchFromCache(key string) (*PeerList, error) {
	data, ok := l.cache.Get(key)
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	peerList, ok := data.(*PeerList)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return peerList, nil
}

func (l *Ledger) fetchFromDB(id identifiers.Identifier, stateHash cid.CID) (*PeerList, error) {
	rawData, err := l.db.Get(GetAgoraDBKey(id, stateHash.Key()))
	if err != nil {
		return nil, common.ErrKeyNotFound
	}

	plist := new(CanonicalPeerList)
	if err := plist.FromBytes(rawData); err != nil {
		return nil, err
	}

	return plist.PeerList(), nil
}

func (l *Ledger) UpdateAssociatedPeers(id identifiers.Identifier, state cid.CID, peerID kramaid.KramaID) (err error) {
	peerList, cacheErr := l.fetchFromCache(GetAgoraKey(state.Key()))
	if errors.Is(cacheErr, common.ErrKeyNotFound) {
		peerList, err = l.fetchFromDB(id, state)

		if errors.Is(err, common.ErrKeyNotFound) {
			peerList = NewPeerList()
		} else if err != nil {
			return err
		}

		l.addToCache(GetAgoraKey(state.Key()), peerList)
	} else if cacheErr != nil {
		return cacheErr
	}

	peerList.AddPeer(peerID)

	l.dbJobsLock.Lock()
	defer l.dbJobsLock.Unlock()

	l.dbJobs = append(l.dbJobs, &job{
		key:   GetAgoraDBKey(id, state.Key()),
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

func (l *Ledger) scoopJobs() []*job {
	l.dbJobsLock.Lock()
	defer l.dbJobsLock.Unlock()
	jobs := l.dbJobs
	l.dbJobs = l.dbJobs[len(jobs):]

	return jobs
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
			l.logger.Debug("Context expired, closing worker")

			return
		case <-time.After(500 * time.Millisecond):
		}

		jobs := l.scoopJobs()

		if len(jobs) > 0 {
			dbWriter := l.db.GetBatchWriter()

			for _, job := range jobs {
				rawData, err := job.value.Bytes()
				if err != nil {
					l.logger.Error("failed to polorize peer list", "err", err)

					continue
				}

				if err := dbWriter.Set(job.key, rawData); err != nil {
					l.logger.Error("Error adding associated peer list to DB", "err", err)

					continue
				}
			}
		}
	}
}

func (l *Ledger) Close() {
	l.logger.Info("Closing Agora-Ledger")
	l.ctxCancel()
}
