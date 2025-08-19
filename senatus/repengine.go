package senatus

import (
	"bytes"
	"context"
	"log"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

const (
	DefaultPeerNTQ = 0.5
)

type senatusStore interface {
	ReadEntry(key []byte) ([]byte, error)
	DeleteEntry(key []byte) error
	NewBatchWriter() db.BatchWriter
	GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error)
}

type stateManager interface {
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	GetPersistentStorageEntry(logicID identifiers.LogicID, slot []byte, state common.Hash) ([]byte, error)
}

type chainManager interface {
	GetTesseract(hash common.Hash, withInteractions bool, withCommitInfo bool) (*common.Tesseract, error)
}

type ReputationEngine struct {
	kramaID           identifiers.KramaID
	ctx               context.Context
	ctxCancel         context.CancelFunc
	logger            hclog.Logger
	db                senatusStore
	cache             *lru.Cache
	dirtyLock         sync.RWMutex
	dirtyEntries      map[peer.ID]*NodeMetaInfo
	mtx               sync.RWMutex
	peerCount         uint64
	signalChan        chan struct{}
	eventSubscription *utils.Subscription
	sysAccSyncLock    sync.RWMutex
	sysAccSyncDone    bool
	State             stateManager
	Chain             chainManager
}

func NewReputationEngine(
	logger hclog.Logger,
	db senatusStore,
	selfInfo *NodeMetaInfo,
	mux *utils.TypeMux,
	state stateManager,
) (*ReputationEngine, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, errors.Wrap(err, "reputation engine failed")
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &ReputationEngine{
		ctx:               ctx,
		ctxCancel:         cancel,
		logger:            logger.Named("Reputation-Engine"),
		kramaID:           selfInfo.KramaID,
		db:                db,
		cache:             cache,
		signalChan:        make(chan struct{}),
		dirtyEntries:      make(map[peer.ID]*NodeMetaInfo),
		eventSubscription: subscribeToEvent(mux),
		State:             state,
	}

	if err = r.LoadPeerCountFromDB(); err != nil {
		return nil, err
	}

	// Listen to system account sync event
	go r.listenToEvent()

	return r, r.UpdatePeer(selfInfo)
}

// subscribeToEvent subscribes to system accounts sync event
func subscribeToEvent(eventMux *utils.TypeMux) *utils.Subscription {
	return eventMux.Subscribe(utils.SystemAccountsSyncedEvent{})
}

func (r *ReputationEngine) addToDirtyEntriesList(peerID peer.ID, info *NodeMetaInfo) {
	_, ok := r.dirtyEntries[peerID]
	if !ok {
		r.dirtyEntries[peerID] = info
	}
}

func (r *ReputationEngine) nodeMetaInfo(peerID peer.ID) (*NodeMetaInfo, error) {
	data, exists := r.cache.Get(storage.SenatusCacheKey(peerID))
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

	rawData, err := r.db.ReadEntry(storage.SenatusDBKey(peerID))
	if err != nil {
		return nil, common.ErrKramaIDNotFound
	}

	info := new(NodeMetaInfo)
	if err = info.FromBytes(rawData); err != nil {
		return nil, err
	}

	r.cache.Add(storage.SenatusCacheKey(peerID), info)

	return info, nil
}

func (r *ReputationEngine) UpdatePeer(data *NodeMetaInfo) error {
	peerID, err := data.KramaID.DecodedPeerID()
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

		if data.RTT != 0 && info.RTT != data.RTT {
			info.RTT = data.RTT
		}

		if data.WalletCount != 0 && info.WalletCount != data.WalletCount {
			info.WalletCount = data.WalletCount
		}

		if data.KramaID != "" && info.KramaID != data.KramaID {
			info.KramaID = data.KramaID
		}

		if len(data.Addrs) != 0 && !utils.AreSlicesOfStringEqual(data.Addrs, info.Addrs) {
			info.Addrs = data.Addrs
		}

		if len(data.PeerSignature) != 0 && !bytes.Equal(data.PeerSignature, info.PeerSignature) {
			info.PeerSignature = data.PeerSignature
		}

		if data.Registered {
			info.Registered = true
		}
	} else {
		info = data

		r.UpdatePeerCount(1)
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.addToDirtyEntriesList(peerID, info)

	r.logger.Debug("dirty entries count", len(r.dirtyEntries))

	r.cache.Add(storage.SenatusCacheKey(peerID), info)

	r.logger.Trace("Added peer to the NTQ table", "peer-ID", peerID)

	return nil
}

func (r *ReputationEngine) UpdateNTQ(kramaID identifiers.KramaID, ntq float32) error {
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

	info = &NodeMetaInfo{
		KramaID: kramaID,
		NTQ:     ntq,
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = info

	return nil
}

func (r *ReputationEngine) UpdateWalletCount(kramaID identifiers.KramaID, delta int32) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		log.Println("Invalid peer id", "krama-id", kramaID, "peer-id", peerID)

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

	info = &NodeMetaInfo{
		KramaID:     kramaID,
		WalletCount: delta,
		NTQ:         DefaultPeerNTQ,
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = info
	r.UpdatePeerCount(1)

	return nil
}

func (r *ReputationEngine) UpdatePeerCount(count uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.peerCount += count
}

func (r *ReputationEngine) GetAddress(kramaID identifiers.KramaID) ([]multiaddr.Multiaddr, error) {
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

func (r *ReputationEngine) GetRTTByPeerID(peerID peer.ID) (int64, error) {
	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return 0, err
	}

	return info.RTT, nil
}

func (r *ReputationEngine) GetKramaIDByPeerID(peerID peer.ID) (identifiers.KramaID, error) {
	info, err := r.nodeMetaInfo(peerID)
	if err != nil {
		return "", err
	}

	return info.KramaID, nil
}

func (r *ReputationEngine) GetNTQ(kramaID identifiers.KramaID) (float32, error) {
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

func (r *ReputationEngine) GetWalletCount(kramaID identifiers.KramaID) (int32, error) {
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

func (r *ReputationEngine) TotalPeerCount() uint64 {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	return r.peerCount
}

func (r *ReputationEngine) StreamPeerInfos(ctx context.Context) (chan *PeerInfo, error) {
	ch := make(chan *PeerInfo)

	entriesChan, err := r.db.GetEntriesWithPrefix(ctx, storage.SenatusPrefix())
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(ch)

		for entry := range entriesChan {
			peerID, err := peer.IDFromBytes(bytes.TrimPrefix(entry.Key, storage.SenatusPrefix()))
			if err != nil {
				r.logger.Debug("failed to decode peerID", "error", err)

				continue
			}

			select {
			case ch <- &PeerInfo{
				ID:   peerID,
				Data: entry.Value,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func setBytesToDBWriter(peerID peer.ID, nodeMetaInfo *NodeMetaInfo, writer db.BatchWriter) error {
	rawData, err := nodeMetaInfo.Bytes()
	if err != nil {
		return err
	}

	if err = writer.Set(storage.SenatusDBKey(peerID), rawData); err != nil {
		return err
	}

	return nil
}

func (r *ReputationEngine) flushDirtyEntries() error {
	r.dirtyLock.RLock()
	defer r.dirtyLock.RUnlock()

	writer := r.db.NewBatchWriter()

	// check if system accounts are synced
	if !r.isSysAccSynced() {
		return common.ErrSysAccsNotSynced
	}

	for peerID, nodeMetaInfo := range r.dirtyEntries {
		// TODO: The condition which was there previously which is no more required,
		// check and confirm once (if r.isGuardianRegisterd(nodeMetaInfo.GetKramaID()))
		if err := setBytesToDBWriter(peerID, nodeMetaInfo, writer); err != nil {
			return err
		}
	}

	return writer.Flush()
}

// DeletePeers deletes peer ids through batch writer
func (r *ReputationEngine) DeletePeers(ids []identifiers.KramaID) error {
	writer := r.db.NewBatchWriter()

	for _, id := range ids {
		peerID, err := id.DecodedPeerID()
		if err != nil {
			return err
		}

		if err := writer.Delete(storage.SenatusDBKey(peerID)); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func (r *ReputationEngine) isSysAccSynced() bool {
	r.sysAccSyncLock.RLock()
	defer r.sysAccSyncLock.RUnlock()

	return r.sysAccSyncDone
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
			r.logger.Error("Error flushing dirty entries to the database", "err", err)

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

func (r *ReputationEngine) LoadPeerCountFromDB() error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	peerInfos, err := r.StreamPeerInfos(r.ctx)
	if err != nil {
		return err
	}

	for range peerInfos {
		r.peerCount++
	}

	return nil
}

func (r *ReputationEngine) Start() error {
	go r.dbWorker()

	return nil
}

func (r *ReputationEngine) Close() {
	r.logger.Info("Closing Senatus")
	r.ctxCancel()

	if err := r.flushDirtyEntries(); err != nil {
		r.logger.Error("failed to flush dirty entries", "err", err)
	}
}

func (r *ReputationEngine) listenToEvent() {
	for range r.eventSubscription.Chan() {
		r.sysAccSyncLock.Lock()
		r.sysAccSyncDone = true
		r.sysAccSyncLock.Unlock()

		return
	}
}
