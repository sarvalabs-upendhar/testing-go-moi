package decision

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/agora/db"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

var AgoraPrefix = []byte("agora")

func GetAgoraKey(key []byte) string {
	out := AgoraPrefix
	out = append(out, key...)

	return hex.EncodeToString(out)
}

func GetAgoraDBKey(address types.Address, key []byte) []byte {
	out := AgoraPrefix
	out = append(out, address.Bytes()[0:20]...)
	out = append(out, key...)

	return out
}

type ledgerStore interface {
	Get(key []byte) ([]byte, error)
	GetBatchWriter() db.BatchWriter
}

type job struct {
	key   []byte
	value *atypes.CanonicalPeerList
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

func (l *Ledger) GetAssociatedPeers(addr types.Address, stateHash atypes.CID) ([]id.KramaID, error) {
	key := GetAgoraKey(stateHash.Key())

	peerList, cacheErr := l.fetchFromCache(key)
	if cacheErr == nil {
		return peerList.Peers(), cacheErr
	}

	peerList, err := l.fetchFromDB(addr, stateHash)
	if err != nil {
		return nil, err
	}

	if peerList.Size() > 0 {
		l.addToCache(GetAgoraKey(stateHash.Key()), peerList)
	}

	return peerList.Peers(), nil
}

func (l *Ledger) addToCache(key string, list *atypes.PeerList) {
	l.cache.Add(key, list)
}

func (l *Ledger) fetchFromCache(key string) (*atypes.PeerList, error) {
	data, ok := l.cache.Get(key)
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	peerList, ok := data.(*atypes.PeerList)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return peerList, nil
}

func (l *Ledger) fetchFromDB(address types.Address, stateHash atypes.CID) (*atypes.PeerList, error) {
	rawData, err := l.db.Get(GetAgoraDBKey(address, stateHash.Key()))
	if err != nil {
		return nil, types.ErrKeyNotFound
	}

	plist := new(atypes.CanonicalPeerList)
	if err := polo.Depolorize(plist, rawData); err != nil {
		return nil, err
	}

	return plist.PeerList(), nil
}

func (l *Ledger) UpdateAssociatedPeers(address types.Address, stateHash atypes.CID, peerID id.KramaID) (err error) {
	peerList, cacheErr := l.fetchFromCache(GetAgoraKey(stateHash.Key()))
	if errors.Is(cacheErr, types.ErrKeyNotFound) {
		peerList, err = l.fetchFromDB(address, stateHash)

		if errors.Is(err, types.ErrKeyNotFound) {
			peerList = atypes.NewPeerList()
		} else if err != nil {
			return err
		}

		l.addToCache(GetAgoraKey(stateHash.Key()), peerList)
	} else if cacheErr != nil {
		return cacheErr
	}

	peerList.AddPeer(peerID)

	l.dbJobsLock.Lock()
	defer l.dbJobsLock.Unlock()

	l.dbJobs = append(l.dbJobs, &job{
		key:   GetAgoraDBKey(address, stateHash.Key()),
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
		jobs := l.dbJobs
		l.dbJobs = l.dbJobs[len(jobs):]
		l.dbJobsLock.Unlock()

		if len(jobs) > 0 {
			dbWriter := l.db.GetBatchWriter()

			for _, job := range jobs {
				rawData, err := polo.Polorize(job.value)
				if err != nil {
					l.logger.Error("Failed to polorize peer list")

					continue
				}

				if err := dbWriter.Set(job.key, rawData); err != nil {
					l.logger.Error("Error adding associated peer list to db")

					continue
				}
			}
		}
	}
}
