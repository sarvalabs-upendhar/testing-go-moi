package guna

import (
	"bytes"
	"context"
	"time"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/moby/locker"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	// "go/build"
)

type store interface {
	CreateEntry(key []byte, value []byte) error
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	UpdateEntry(key []byte, newValue []byte) error
	NewBatchWriter() db.BatchWriter
	GetEntries(prefix []byte) chan types.DBEntry
}
type state interface {
	GetPublicKeyFromContract(ctx context.Context, ids ...id.KramaID) (keys [][]byte, err error)
}

type ReputationInfo struct {
	Addrs      []string
	NTQ        int32
	Degree     int64
	PublickKey []byte
}

func (ri *ReputationInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ri)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize reputation info")
	}

	return rawData, nil
}

func (ri *ReputationInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ri, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize reputation info")
	}

	return nil
}

type ReputationEngine struct {
	kramaID  id.KramaID
	ctx      context.Context
	logger   hclog.Logger
	db       store
	cache    *lru.Cache
	locks    *locker.Locker
	state    state
	messages []*ptypes.HelloMsg
}

func NewReputationEngine(
	ctx context.Context,
	logger hclog.Logger,
	state state,
	db store,
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
		messages: make([]*ptypes.HelloMsg, 0, 200), // Max message queue limit is 200
	}

	return r, nil
}

func (r *ReputationEngine) AddNewPeer(key id.KramaID, data *ReputationInfo) error {
	r.logger.Debug("Added peer to NTQ table", "id")

	contains, err := r.db.Contains(dhruva.NtqDBKey(key))
	if err != nil {
		return err
	}

	if !contains {
		rawData, err := data.Bytes()
		if err != nil {
			return err
		}

		if err := r.db.CreateEntry(dhruva.NtqDBKey(key), rawData); err != nil {
			return err
		}

		r.cache.Add(dhruva.NtqCacheKey(key), data)

		return nil
	}

	if err := r.UpdateAddress(key, data.Addrs); err != nil {
		return err
	}

	if err := r.UpdateNTQ(key, data.NTQ); err != nil {
		return err
	}

	if err := r.UpdatePublicKey(key, data.PublickKey); err != nil {
		return err
	}

	return nil
}

func (r *ReputationEngine) UpdateAddress(key id.KramaID, addrs []string) error {
	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.Addrs = addrs

		r.cache.Add(dhruva.NtqCacheKey(key), info)

		rawData, err := info.Bytes()
		if err != nil {
			return err
		}

		return r.db.UpdateEntry(dhruva.NtqDBKey(key), rawData)
	}

	info = &ReputationInfo{
		Addrs: addrs,
		NTQ:   0,
	}
	r.cache.Add(dhruva.NtqCacheKey(key), info)

	rawData, err := info.Bytes()
	if err != nil {
		return err
	}

	return r.db.CreateEntry(dhruva.NtqDBKey(key), rawData)
}

func (r *ReputationEngine) UpdatePublicKey(key id.KramaID, pk []byte) error {
	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.PublickKey = pk

		r.cache.Add(dhruva.NtqCacheKey(key), info)

		rawData, err := info.Bytes()
		if err != nil {
			return err
		}

		return r.db.UpdateEntry(dhruva.NtqDBKey(key), rawData)
	}

	info = &ReputationInfo{
		PublickKey: pk,
		NTQ:        0,
	}
	r.cache.Add(dhruva.NtqCacheKey(key), info)

	rawData, err := info.Bytes()
	if err != nil {
		return err
	}

	return r.db.CreateEntry(dhruva.NtqDBKey(key), rawData)
}

func (r *ReputationEngine) UpdateNTQ(key id.KramaID, ntq int32) error {
	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.NTQ = ntq

		r.cache.Add(dhruva.NtqCacheKey(key), info)

		rawData, err := info.Bytes()
		if err != nil {
			return err
		}

		return r.db.UpdateEntry(dhruva.NtqDBKey(key), rawData)
	}

	info = &ReputationInfo{
		Addrs: nil,
		NTQ:   ntq,
	}

	r.cache.Add(dhruva.NtqCacheKey(key), info)

	rawData, err := info.Bytes()
	if err != nil {
		return err
	}

	return r.db.CreateEntry(dhruva.NtqDBKey(key), rawData)
}

func (r *ReputationEngine) UpdateInclusivity(key id.KramaID, delta int64) error {
	r.locks.Lock(string(key))
	defer func() {
		if err := r.locks.Unlock(string(key)); err != nil {
			r.logger.Error("Error Unlocking the tuple", "id", key)
		}
	}()

	info, err := r.getInfo(key)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.Degree += delta

		r.cache.Add(dhruva.NtqCacheKey(key), info)

		rawData, err := info.Bytes()
		if err != nil {
			return err
		}

		return r.db.UpdateEntry(dhruva.NtqDBKey(key), rawData)
	}

	info = &ReputationInfo{
		Addrs:  nil,
		Degree: delta,
	}

	r.cache.Add(dhruva.NtqCacheKey(key), info)

	rawData, err := info.Bytes()
	if err != nil {
		return err
	}

	return r.db.CreateEntry(dhruva.NtqDBKey(key), rawData)
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

func (r *ReputationEngine) GetPublicKey(ctx context.Context, id id.KramaID) ([]byte, error) {
	info, err := r.getInfo(id)
	if err != nil {
		return nil, err
	}

	if info.PublickKey == nil {
		return nil, errors.New("public key not found")
	}

	return info.PublickKey, nil
}

func (r *ReputationEngine) getInfo(id id.KramaID) (*ReputationInfo, error) {
	if id == "" {
		return nil, types.ErrInvalidKramaID
	}

	data, exists := r.cache.Get(dhruva.NtqCacheKey(id))
	if exists {
		reputationInfo, ok := data.(*ReputationInfo)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return reputationInfo, nil
	}

	rawData, err := r.db.ReadEntry(dhruva.NtqDBKey(id))
	if err != nil {
		return nil, types.ErrKramaIDNotFound
	}

	info := new(ReputationInfo)
	if err = info.FromBytes(rawData); err != nil {
		return nil, err
	}

	r.cache.Add(dhruva.NtqCacheKey(id), info)

	return info, nil
}

func (r *ReputationEngine) AddEntries(msg ptypes.SyncReputationInfo) error {
	writer := r.db.NewBatchWriter()

	for _, v := range msg.Msg {
		reputationInfo := ReputationInfo{
			NTQ:    v.Ntq,
			Addrs:  v.Address,
			Degree: v.Degree,
		}

		rawData, err := reputationInfo.Bytes()
		if err != nil {
			return err
		}

		err = writer.Set(dhruva.NtqDBKey(v.ID), rawData)
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

func (r *ReputationEngine) GetAllEntries() (chan *ptypes.SyncReputationInfo, error) {
	ch := make(chan *ptypes.SyncReputationInfo)

	go func() {
		msg := new(ptypes.SyncReputationInfo)
		entriesChan := r.db.GetEntries([]byte{dhruva.NTQ.Byte()})
		count := 0

		for entry := range entriesChan {
			kramaID := id.KramaID(bytes.TrimPrefix(entry.Key, []byte{dhruva.NTQ.Byte()}))
			info := new(ReputationInfo)

			if err := info.FromBytes(entry.Value); err != nil {
				r.logger.Error("Error decoding peer info", err)
			}

			msg.Msg = append(msg.Msg, ptypes.PeerInfo{ID: kramaID, Ntq: info.NTQ, Address: info.Addrs, Degree: info.Degree})

			count++

			if count == 40 {
				ch <- msg
				msg = new(ptypes.SyncReputationInfo)
				count = 0
			}
		}
	}()

	return ch, nil
}

func (r *ReputationEngine) SenatusHandler(msg *pubsub.Message) error {
	helloMsg := new(ptypes.HelloMsg)

	if err := helloMsg.FromBytes(msg.Data); err != nil {
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

func (r *ReputationEngine) HandleHelloMessages(msgs []*ptypes.HelloMsg) (int, error) {
	kramaIDs := make([]id.KramaID, len(msgs))

	for index, msg := range msgs {
		kramaIDs[index] = msg.Info.ID
	}

	publicKeys, err := r.state.GetPublicKeyFromContract(context.Background(), kramaIDs...)
	if err != nil {
		r.logger.Error("Error fetching public key", "error", err)

		return -1, err
	}

	for index, publicKey := range publicKeys {
		msg := msgs[index]

		rawData, err := msg.Info.Bytes()
		if err != nil {
			return index, err
		}

		verified, err := mudra.Verify(rawData, msg.Signature, publicKey)
		if err != nil {
			return index, err
		}

		if !verified {
			r.logger.Error("Signature verification failed", "error", err)

			return index, err
		}

		if err := r.AddNewPeer(msg.Info.ID, &ReputationInfo{
			Addrs:      msg.Info.Address,
			NTQ:        msg.Info.Ntq,
			PublickKey: publicKey,
		}); err != nil {
			return index, err
		}
	}

	return 0, nil
}

func (r *ReputationEngine) startWorkers() {
	for {
		select {
		case <-time.After(5 * time.Second):
		case <-r.ctx.Done():
			r.logger.Info("Closing reputation worker")
		}

		r.locks.Lock("HelloMsgLock")

		currentLength := len(r.messages)

		helloMsgs := r.messages[0:currentLength]

		if err := r.locks.Unlock("HelloMsgLock"); err != nil {
			r.logger.Error("Error releasing the hello message lock")

			return
		}

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

func (r *ReputationEngine) Start(id id.KramaID, ntq int32, publicKey []byte, address []multiaddr.Multiaddr) error {
	r.kramaID = id
	// Add self reputation info to DB
	info := &ReputationInfo{
		Addrs:      utils.MultiAddrToString(address...),
		NTQ:        ntq,
		PublickKey: publicKey,
	}

	if err := r.AddNewPeer(r.kramaID, info); err != nil {
		r.logger.Error("Error starting reputation engine", "error", err)

		return err
	}

	go r.startWorkers()

	return nil
}
