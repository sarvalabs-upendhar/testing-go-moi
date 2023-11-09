package senatus

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/config"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

const (
	MaxQueueSize   = 200
	DefaultPeerNTQ = 0.5
	MsgsPerWorker  = 1
)

type senatusStore interface {
	ReadEntry(key []byte) ([]byte, error)
	NewBatchWriter() db.BatchWriter
	GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error)
}

type network interface {
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}

type ReputationEngine struct {
	kramaID             id.KramaID
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	logger              hclog.Logger
	db                  senatusStore
	client              *http.Client
	cache               *lru.Cache
	dirtyLock           sync.RWMutex
	dirtyEntries        map[peer.ID]*NodeMetaInfo
	network             network
	msgQueueLock        sync.Mutex //nolint:unused
	signalChan          chan struct{}
	pendingMessageQueue *RequestQueue
}

func NewReputationEngine(
	logger hclog.Logger,
	network network,
	db senatusStore,
	selfID id.KramaID,
	selfInfo *NodeMetaInfo,
) (*ReputationEngine, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, errors.Wrap(err, "reputation engine failed")
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &ReputationEngine{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger.Named("Reputation-Engine"),
		kramaID:   selfID,
		db:        db,
		network:   network,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:    1024,
			MaxConnsPerHost: 1000,
		}},
		cache:               cache,
		signalChan:          make(chan struct{}),
		dirtyEntries:        make(map[peer.ID]*NodeMetaInfo),
		pendingMessageQueue: NewRequestQueue(MaxQueueSize), // Max message queue limit is 200
	}

	return r, r.UpdatePeer(selfID, selfInfo)
}

func (r *ReputationEngine) nodeMetaInfo(peerID peer.ID) (*NodeMetaInfo, error) {
	data, exists := r.cache.Get(storage.NtqCacheKey(peerID))
	if exists {
		reputationInfo, ok := data.(*NodeMetaInfo)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return reputationInfo, nil
	}

	r.dirtyLock.RLock()
	defer r.dirtyLock.RUnlock()

	if _, ok := r.dirtyEntries[peerID]; ok {
		return r.dirtyEntries[peerID], nil
	}

	rawData, err := r.db.ReadEntry(storage.NtqDBKey(peerID))
	if err != nil {
		return nil, common.ErrKramaIDNotFound
	}

	info := new(NodeMetaInfo)
	if err = info.FromBytes(rawData); err != nil {
		return nil, err
	}

	r.cache.Add(storage.NtqCacheKey(peerID), info)

	return info, nil
}

func (r *ReputationEngine) UpdatePeer(kramaID id.KramaID, data *NodeMetaInfo) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	return r.AddNewPeerWithPeerID(peerID, data)
}

func (r *ReputationEngine) AddNewPeerWithPeerID(peerID peer.ID, data *NodeMetaInfo) error {
	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, common.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		if data.NTQ != 0 && data.NTQ != DefaultPeerNTQ && info.NTQ != data.NTQ {
			info.NTQ = data.NTQ
		}

		if data.WalletCount != 0 && info.WalletCount != data.WalletCount {
			info.WalletCount = data.WalletCount
		}

		if len(data.Addrs) != 0 && !utils.AreSlicesOfStringEqual(data.Addrs, info.Addrs) {
			info.Addrs = data.Addrs
		}

		if len(data.PeerSignature) != 0 && !bytes.Equal(data.PeerSignature, info.PeerSignature) {
			info.PeerSignature = data.PeerSignature
		}

		if len(data.PublicKey) != 0 && !bytes.Equal(data.PublicKey, info.PublicKey) {
			info.PublicKey = data.PublicKey
		}
	} else {
		info = data
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = info

	r.cache.Add(storage.NtqCacheKey(peerID), info)

	r.logger.Trace("Added peer to the NTQ table", "peer-ID", peerID)

	return nil
}

func (r *ReputationEngine) UpdateNTQ(kramaID id.KramaID, ntq float32) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, common.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.UpdateNTQ(ntq)

		r.dirtyLock.Lock()
		defer r.dirtyLock.Unlock()

		if _, ok := r.dirtyEntries[peerID]; !ok {
			r.dirtyEntries[peerID] = info
		}

		return nil
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = &NodeMetaInfo{
		NTQ: ntq,
	}

	return nil
}

func (r *ReputationEngine) UpdateWalletCount(kramaID id.KramaID, delta int32) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, common.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.UpdateWalletCount(delta)

		r.dirtyLock.Lock()
		defer r.dirtyLock.Unlock()

		if _, ok := r.dirtyEntries[peerID]; !ok {
			r.dirtyEntries[peerID] = info
		}

		return nil
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = &NodeMetaInfo{
		WalletCount: delta,
		NTQ:         DefaultPeerNTQ,
	}

	return nil
}

func (r *ReputationEngine) UpdatePublicKey(kramaID id.KramaID, pk []byte) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, common.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.UpdatePublicKey(pk)

		r.cache.Add(storage.NtqCacheKey(peerID), info)

		r.dirtyLock.Lock()
		defer r.dirtyLock.Unlock()

		r.dirtyEntries[peerID] = info

		return nil
	}

	info = &NodeMetaInfo{
		PublicKey: pk,
		NTQ:       DefaultPeerNTQ,
	}

	r.cache.Add(storage.NtqCacheKey(peerID), info)

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = info

	return nil
}

func (r *ReputationEngine) GetAddress(kramaID id.KramaID) ([]multiaddr.Multiaddr, error) {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return nil, common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return nil, err
	}

	return info.GetMultiAddress()
}

func (r *ReputationEngine) GetAddressByPeerID(peerID peer.ID) ([]multiaddr.Multiaddr, error) {
	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return nil, err
	}

	return info.GetMultiAddress()
}

func (r *ReputationEngine) GetNTQ(kramaID id.KramaID) (float32, error) {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return 0, common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return 0, err
	}

	return info.GetNTQ(), nil
}

func (r *ReputationEngine) GetWalletCount(kramaID id.KramaID) (int32, error) {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return 0, common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return 0, err
	}

	return info.GetWalletCount(), nil
}

func (r *ReputationEngine) GetPublicKey(kramaID id.KramaID) ([]byte, error) {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return nil, common.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return nil, err
	}

	if info.PublicKey == nil {
		return nil, errors.New("public key not found")
	}

	return info.PublicKey, nil
}

func (r *ReputationEngine) StreamPeerInfos(ctx context.Context) (chan *networkmsg.PeerInfo, error) {
	ch := make(chan *networkmsg.PeerInfo)

	entriesChan, err := r.db.GetEntriesWithPrefix(ctx, []byte{storage.NTQ.Byte()})
	if err != nil {
		return nil, err
	}

	go func() {
		for entry := range entriesChan {
			peerID := peer.ID(bytes.TrimPrefix(entry.Key, []byte{storage.NTQ.Byte()}))

			ch <- &networkmsg.PeerInfo{
				ID:   peerID,
				Data: entry.Value,
			}
		}

		close(ch)
	}()

	return ch, nil
}

func (r *ReputationEngine) flushDirtyEntries() error {
	r.dirtyLock.RLock()
	defer r.dirtyLock.RUnlock()

	writer := r.db.NewBatchWriter()

	for peerID, nodeMetaInfo := range r.dirtyEntries {
		rawData, err := nodeMetaInfo.Bytes()
		if err != nil {
			return err
		}

		if err = writer.Set(storage.NtqDBKey(peerID), rawData); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func (r *ReputationEngine) signalNewMessages() {
	select {
	case r.signalChan <- struct{}{}:
	default:
	}
}

func (r *ReputationEngine) senatusHandler(msg *pubsub.Message) error {
	helloMsg := new(networkmsg.HelloMsg)

	if err := helloMsg.FromBytes(msg.Data); err != nil {
		return err
	}

	r.logger.Trace("Received hello message", "krama-ID", helloMsg.KramaID)

	if err := r.pendingMessageQueue.Push(&NodeMetaInfoMsg{
		KramaID:       helloMsg.KramaID,
		Address:       helloMsg.Address,
		PeerSignature: helloMsg.Signature,
		NTQ:           DefaultPeerNTQ,
	}); err != nil {
		r.signalNewMessages()
	}

	return nil
}

func (r *ReputationEngine) verifyHelloMsg(msg *NodeMetaInfoMsg) error {
	rawData, err := msg.HelloMessageBytes()
	if err != nil {
		return errors.Wrapf(err, "Failed to fetch hello message bytes")
	}

	if err := crypto.VerifySignatureUsingKramaID(msg.KramaID, rawData, msg.PeerSignature); err != nil {
		return errors.Wrap(err, "failed to verify hello msg signature")
	}

	return nil
}

func (r *ReputationEngine) handleMessages(msgs []*NodeMetaInfoMsg) {
	if len(msgs) == 0 {
		return
	}

	for _, msg := range msgs {
		if msg.KramaID == "" {
			continue
		}

		if err := r.verifyHelloMsg(msg); err != nil {
			r.logger.Error("Failed to verify hello message", "err", err)

			continue
		}

		if err := r.UpdatePeer(msg.KramaID, &NodeMetaInfo{
			Addrs:         msg.Address,
			NTQ:           msg.NTQ,
			WalletCount:   msg.WalletCount,
			PeerSignature: msg.PeerSignature,
		}); err != nil {
			r.logger.Error("Failed to add node meta information", "err", err, "krama-ID", msg.KramaID)

			continue
		}
	}
}

func (r *ReputationEngine) messageWorker() {
	for {
		select {
		case <-time.After(2 * time.Second):
		case <-r.signalChan:
		case <-r.ctx.Done():
			return
		}

		r.handleMessages(r.pendingMessageQueue.Pop(MsgsPerWorker))
	}
}

func (r *ReputationEngine) dbWorker() {
	for {
		select {
		case <-time.After(5 * time.Second):
		case <-r.signalChan:
		case <-r.ctx.Done():
			r.logger.Debug("Closing reputation worker")

			return
		}

		if err := r.flushDirtyEntries(); err != nil {
			r.logger.Error("Error flushing dirty entries from the database", "err", err)

			continue
		}

		r.cleanUpDirtyStorage()
	}
}

func (r *ReputationEngine) cleanUpDirtyStorage() {
	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries = make(map[peer.ID]*NodeMetaInfo)
}

func (r *ReputationEngine) Start() error {
	if err := r.network.Subscribe(r.ctx, config.SenatusTopic, r.senatusHandler); err != nil {
		return errors.Wrap(err, "failed to subscribe senatus topic")
	}

	go r.messageWorker()
	go r.dbWorker()

	return nil
}

func (r *ReputationEngine) Close() {
	r.logger.Info("Closing Senatus")
	r.ctxCancel()

	if err := r.flushDirtyEntries(); err != nil {
		r.logger.Error("Failed to flush dirty entries", "error", err)
	}
}
