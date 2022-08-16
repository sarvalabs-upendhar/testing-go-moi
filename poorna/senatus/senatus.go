package senatus

import (
	"bytes"
	"context"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/mudra"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"time"
	// "go/build"
)

const KeyPrefix = "PEER_INFO"

type PersistenceManager interface {
	CreateEntry(key []byte, value []byte) error
	ReadEntry(key []byte) ([]byte, error)
	Contains([]byte) (bool, error)
	UpdateEntry(key []byte, newValue []byte) error
	NewBatchWriter() *badger.WriteBatch
	GetEntries(prefix []byte) chan ktypes.DBEntry
}
type State interface {
	GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error)
}

type ReputationInfo struct {
	Addrs  []string
	NTQ    int32
	Degree int64
}
type ReputationEngine struct {
	kramaID  id.KramaID
	ctx      context.Context
	addrs    []multiaddr.Multiaddr
	logger   hclog.Logger
	db       PersistenceManager
	cache    *lru.Cache
	locks    *locker.Locker
	state    State
	messages []*ktypes.HelloMsg
}

func DBKey(key id.KramaID) []byte {
	key = KeyPrefix + key

	return []byte(key)
}
func CacheKey(key id.KramaID) string {
	return KeyPrefix + string(key)
}

func NewReputationEngine(
	ctx context.Context,
	logger hclog.Logger,
	id id.KramaID,
	ntq int32,
	state State,
	db PersistenceManager,
) (*ReputationEngine, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, errors.Wrap(err, "reputation engine failed")
	}

	r := &ReputationEngine{
		logger:   logger,
		ctx:      ctx,
		db:       db,
		cache:    cache,
		locks:    locker.New(),
		state:    state,
		kramaID:  id,
		messages: make([]*ktypes.HelloMsg, 0, 200), //Max message queue limit is 200
	}

	// Add self reputation info to DB
	info := &ReputationInfo{
		Addrs: kutils.MultiAddrToString(r.addrs...),
		NTQ:   ntq,
	}

	if err := r.AddNewPeer(r.kramaID, info); err != nil {
		r.logger.Error("Error starting reputation engine", "error", err)
		panic(err)
	}

	return r, nil
}
func (r *ReputationEngine) AddNewPeer(key id.KramaID, data *ReputationInfo) error {
	r.logger.Debug("Added peer to NTQ table")

	contains, err := r.db.Contains(DBKey(key))
	if err != nil {
		return err
	}

	if !contains {
		if err := r.db.CreateEntry(DBKey(key), polo.Polorize(data)); err != nil {
			return err
		}

		r.cache.Add(CacheKey(key), data)

		return nil
	}

	if err := r.UpdateAddress(key, data.Addrs); err != nil {
		return err
	}

	if err := r.UpdateNTQ(key, data.NTQ); err != nil {
		return err
	}

	return nil
}
func (r *ReputationEngine) UpdateAddress(key id.KramaID, addrs []string) error {
	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, ktypes.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.Addrs = addrs

		r.cache.Add(CacheKey(key), info)

		return r.db.UpdateEntry(DBKey(key), polo.Polorize(info))
	}

	info = &ReputationInfo{
		Addrs: addrs,
		NTQ:   0,
	}
	r.cache.Add(CacheKey(key), info)

	return r.db.CreateEntry(DBKey(key), polo.Polorize(info))
}

func (r *ReputationEngine) UpdateNTQ(key id.KramaID, ntq int32) error {
	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, ktypes.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.NTQ = ntq

		r.cache.Add(CacheKey(key), info)

		return r.db.UpdateEntry(DBKey(key), polo.Polorize(info))
	}

	info = &ReputationInfo{
		Addrs: nil,
		NTQ:   ntq,
	}

	r.cache.Add(CacheKey(key), info)

	return r.db.CreateEntry(DBKey(key), polo.Polorize(info))
}

func (r *ReputationEngine) UpdateInclusivity(key id.KramaID, delta int64) error {
	r.locks.Lock(string(key))
	defer func() {
		if err := r.locks.Unlock(string(key)); err != nil {
			r.logger.Error("Error Unlocking the tuple", "id", key)
		}
	}()

	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, ktypes.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.Degree += delta

		r.cache.Add(CacheKey(key), info)

		return r.db.UpdateEntry(DBKey(key), polo.Polorize(info))
	}

	info = &ReputationInfo{
		Addrs:  nil,
		Degree: delta,
	}

	r.cache.Add(CacheKey(key), info)

	return r.db.CreateEntry(DBKey(key), polo.Polorize(info))
}
func (r *ReputationEngine) GetAddress(key id.KramaID) (multiAddrs []multiaddr.Multiaddr, err error) {
	info, err := r.getInfo(key)
	if err != nil {
		return nil, err
	}

	for _, addr := range info.Addrs {
		var mAddr multiaddr.Multiaddr

		mAddr, err = multiaddr.NewMultiaddr(addr)
		if err != nil {
			r.logger.Error("Error parsing multiAddr", err)
		}

		multiAddrs = append(multiAddrs, mAddr)
	}

	return
}

func (r *ReputationEngine) GetNTQ(id id.KramaID) (int32, error) {
	info, err := r.getInfo(id)
	if err != nil {
		return 0, err
	}

	return info.NTQ, nil
}

func (r *ReputationEngine) getInfo(id id.KramaID) (*ReputationInfo, error) {
	dbKey := DBKey(id)
	info := new(ReputationInfo)

	data, exists := r.cache.Get(CacheKey(id))
	if !exists {
		rawData, err := r.db.ReadEntry(dbKey)
		if err == nil {
			if err = polo.Depolorize(info, rawData); err != nil {
				return nil, err
			}
		} else if errors.Is(err, ktypes.ErrKeyNotFound) {
			return nil, ktypes.ErrKramaIDNotFound
		}

		return nil, err
	}

	info, ok := data.(*ReputationInfo)
	if !ok {
		return nil, errors.New("error type assert failed")
	}

	return info, nil
}

func (r *ReputationEngine) AddEntries(msg ktypes.SyncReputationInfo) error {
	writer := r.db.NewBatchWriter()

	for _, v := range msg.Msg {
		err := writer.Set(DBKey(v.ID), polo.Polorize(
			ReputationInfo{
				NTQ:    v.Ntq,
				Addrs:  v.Address,
				Degree: v.Degree,
			}))
		if err != nil {
			return err
		}
	}

	return writer.Flush()
}
func (r *ReputationEngine) GetInclusivity(id id.KramaID) (int64, error) {
	r.locks.Lock(string(id))
	defer func() {
		if err := r.locks.Unlock(string(id)); err != nil {
			r.logger.Error("Error Unlocking the tuple", "id", id)
		}
	}()

	info, err := r.getInfo(id)
	if err != nil {
		return 0, err
	}

	return info.Degree, nil
}

func (r *ReputationEngine) GetAllEntries() (chan *ktypes.SyncReputationInfo, error) {
	ch := make(chan *ktypes.SyncReputationInfo)

	go func() {
		msg := new(ktypes.SyncReputationInfo)
		entriesChan := r.db.GetEntries([]byte(KeyPrefix))
		count := 0

		for entry := range entriesChan {
			kramaID := id.KramaID(bytes.TrimPrefix(entry.Key, []byte(KeyPrefix)))
			info := new(ReputationInfo)

			if err := polo.Depolorize(info, entry.Value); err != nil {
				r.logger.Error("Error decoding peer info", err)
			}

			msg.Msg = append(msg.Msg, ktypes.PeerInfo{ID: kramaID, Ntq: info.NTQ, Address: info.Addrs, Degree: info.Degree})

			count++

			if count == 40 {
				ch <- msg
				msg = new(ktypes.SyncReputationInfo)
				count = 0
			}
		}
	}()

	return ch, nil
}

func (r *ReputationEngine) SenatusHandler(ctx context.Context, msg *pubsub.Message) error {
	helloMsg := new(ktypes.HelloMsg)

	if err := polo.Depolorize(helloMsg, msg.Data); err != nil {
		return err
	}

	r.logger.Debug("Received Hello Message", "Peer id", helloMsg.Info.ID)

	r.locks.Lock("HelloMsgLock")

	r.messages = append(r.messages, helloMsg)

	if err := r.locks.Unlock("HelloMsgLock"); err != nil {
		r.logger.Error("Error releasing the hello message lock")

		return err
	}

	return nil
}

func (r *ReputationEngine) HandleHelloMessages(msgs []*ktypes.HelloMsg) (int, error) {
	kramaIDs := make([]id.KramaID, len(msgs))

	for index, msg := range msgs {
		kramaIDs[index] = msg.Info.ID
	}

	publicKeys, err := r.state.GetPublicKeys(kramaIDs...)
	if err != nil {
		r.logger.Error("Error fetching public key", "error", err)

		return -1, err
	}

	for index, publicKey := range publicKeys {
		msg := msgs[index]

		verified, err := mudra.Verify(polo.Polorize(msg.Info), msg.Signature, publicKey)
		if err != nil {
			return index, err
		}

		if !verified {
			r.logger.Error("Signature verification failed", "error", err)

			return index, err
		}

		if err := r.AddNewPeer(msg.Info.ID, &ReputationInfo{
			Addrs: msg.Info.Address,
			NTQ:   msg.Info.Ntq,
		}); err != nil {
			return index, err
		}
	}

	return 0, nil
}

func (r *ReputationEngine) startWorkers(ctx context.Context) {
	for {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			r.logger.Info("Closing reputation worker")
		}

		r.locks.Lock("HelloMsgLock")

		var currentLength = len(r.messages)

		helloMsgs := r.messages[0:currentLength]

		if err := r.locks.Unlock("HelloMsgLock"); err != nil {
			r.logger.Error("Error releasing the hello message lock")

			return
		}
		//currentLength := len(r.messages)

		if currentLength < 1 {
			continue
		}

		count, err := r.HandleHelloMessages(helloMsgs)
		if err != nil {
			r.logger.Error("Error handling hello message", "error", err)

			currentLength = count + 1
		}

		r.locks.Lock("HelloMsgLock")

		r.messages = r.messages[currentLength:]

		if err := r.locks.Unlock("HelloMsgLock"); err != nil {
			r.logger.Error("Error releasing the hello message lock")
		}
	}
}

func (r *ReputationEngine) Start() {
	go r.startWorkers(r.ctx)
}
