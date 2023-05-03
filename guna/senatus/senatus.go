package senatus

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const (
	MaxQueueSize   = 200
	DefaultPeerNTQ = 0.5
	MsgsPerWorker  = 1
	GossipTopic    = "MOI_PUBSUB_SENATUS"
)

type store interface {
	ReadEntry(key []byte) ([]byte, error)
	NewBatchWriter() db.BatchWriter
	GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *types.DBEntry, error)
}

type network interface {
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}

type ReputationEngine struct {
	kramaID             id.KramaID
	ctx                 context.Context
	logger              hclog.Logger
	db                  store
	client              *http.Client
	cache               *lru.Cache
	dirtyLock           sync.RWMutex
	dirtyEntries        map[peer.ID]*gtypes.NodeMetaInfo
	network             network
	msgQueueLock        sync.Mutex //nolint:unused
	signalChan          chan struct{}
	pendingMessageQueue *gtypes.RequestQueue
}

func NewReputationEngine(
	ctx context.Context,
	logger hclog.Logger,
	network network,
	db store,
	selfID id.KramaID,
	selfInfo *gtypes.NodeMetaInfo,
) (*ReputationEngine, error) {
	cache, err := lru.New(100)
	if err != nil {
		return nil, errors.Wrap(err, "reputation engine failed")
	}

	r := &ReputationEngine{
		ctx:     ctx,
		logger:  logger,
		kramaID: selfID,
		db:      db,
		network: network,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:    1024,
			MaxConnsPerHost: 1000,
		}},
		cache:               cache,
		signalChan:          make(chan struct{}),
		dirtyEntries:        make(map[peer.ID]*gtypes.NodeMetaInfo),
		pendingMessageQueue: gtypes.NewRequestQueue(MaxQueueSize), // Max message queue limit is 200
	}

	return r, r.AddNewPeer(selfID, selfInfo)
}

func (r *ReputationEngine) nodeMetaInfo(peerID peer.ID) (*gtypes.NodeMetaInfo, error) {
	data, exists := r.cache.Get(dhruva.NtqCacheKey(peerID))
	if exists {
		reputationInfo, ok := data.(*gtypes.NodeMetaInfo)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return reputationInfo, nil
	}

	r.dirtyLock.RLock()
	defer r.dirtyLock.RUnlock()

	if _, ok := r.dirtyEntries[peerID]; ok {
		return r.dirtyEntries[peerID], nil
	}

	rawData, err := r.db.ReadEntry(dhruva.NtqDBKey(peerID))
	if err != nil {
		return nil, types.ErrKramaIDNotFound
	}

	info := new(gtypes.NodeMetaInfo)
	if err = info.FromBytes(rawData); err != nil {
		return nil, err
	}

	r.cache.Add(dhruva.NtqCacheKey(peerID), info)

	return info, nil
}

func (r *ReputationEngine) AddNewPeer(kramaID id.KramaID, data *gtypes.NodeMetaInfo) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return types.ErrInvalidKramaID
	}

	return r.AddNewPeerWithPeerID(peerID, data)
}

func (r *ReputationEngine) AddNewPeerWithPeerID(peerID peer.ID, data *gtypes.NodeMetaInfo) error {
	info, err := r.nodeMetaInfo(peerID)
	if info != nil {
		return nil
	}

	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = data

	r.cache.Add(dhruva.NtqCacheKey(peerID), data)

	r.logger.Debug("Added peer to NTQ table", "id", peerID)

	return nil
}

func (r *ReputationEngine) hasRequiredNodeMetaInfo(info *gtypes.NodeMetaInfo) bool {
	if info.NTQ != 0 && len(info.Addrs) != 0 && len(info.PublicKey) != 0 && len(info.PeerSignature) != 0 {
		return true
	}

	return false
}

func (r *ReputationEngine) UpdatePeer(kramaID id.KramaID, data *gtypes.NodeMetaInfo) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return types.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		if r.hasRequiredNodeMetaInfo(info) {
			return types.ErrAlreadyKnown
		}

		if data.NTQ == 0 || data.NTQ == DefaultPeerNTQ {
			data.NTQ = info.NTQ
		}

		if data.WalletCount == 0 {
			data.WalletCount = info.WalletCount
		}

		if len(data.Addrs) == 0 {
			data.Addrs = info.Addrs
		}

		if len(data.PeerSignature) == 0 {
			data.PeerSignature = info.PeerSignature
		}

		if len(data.PublicKey) == 0 {
			data.PublicKey = info.PublicKey
		}
	}

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = data

	r.cache.Add(dhruva.NtqCacheKey(peerID), data)

	return nil
}

func (r *ReputationEngine) UpdateNTQ(kramaID id.KramaID, ntq float32) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return types.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
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

	r.dirtyEntries[peerID] = &gtypes.NodeMetaInfo{
		NTQ: ntq,
	}

	return nil
}

func (r *ReputationEngine) UpdateWalletCount(kramaID id.KramaID, delta int32) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return types.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
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

	r.dirtyEntries[peerID] = &gtypes.NodeMetaInfo{
		WalletCount: delta,
		NTQ:         DefaultPeerNTQ,
	}

	return nil
}

func (r *ReputationEngine) UpdatePublicKey(kramaID id.KramaID, pk []byte) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return types.ErrInvalidKramaID
	}

	info, err := r.nodeMetaInfo(peerID)
	if err != nil && !errors.Is(err, types.ErrKramaIDNotFound) {
		return err
	}

	if info != nil {
		info.UpdatePublicKey(pk)

		r.cache.Add(dhruva.NtqCacheKey(peerID), info)

		r.dirtyLock.Lock()
		defer r.dirtyLock.Unlock()

		r.dirtyEntries[peerID] = info

		return nil
	}

	info = &gtypes.NodeMetaInfo{
		PublicKey: pk,
		NTQ:       DefaultPeerNTQ,
	}

	r.cache.Add(dhruva.NtqCacheKey(peerID), info)

	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries[peerID] = info

	return nil
}

func (r *ReputationEngine) GetAddress(kramaID id.KramaID) ([]multiaddr.Multiaddr, error) {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return nil, types.ErrInvalidKramaID
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
		return 0, types.ErrInvalidKramaID
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
		return 0, types.ErrInvalidKramaID
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
		return nil, types.ErrInvalidKramaID
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

func (r *ReputationEngine) getPublicKeyFromContract(ids ...id.KramaID) (keys [][]byte, err error) {
	return RetrievePublicKeys(ids, r.client, r.logger)
}

func (r *ReputationEngine) StreamPeerInfos(ctx context.Context) (chan *ptypes.PeerInfo, error) {
	ch := make(chan *ptypes.PeerInfo)

	entriesChan, err := r.db.GetEntriesWithPrefix(ctx, []byte{dhruva.NTQ.Byte()})
	if err != nil {
		return nil, err
	}

	go func() {
		for entry := range entriesChan {
			peerID := peer.ID(bytes.TrimPrefix(entry.Key, []byte{dhruva.NTQ.Byte()}))

			ch <- &ptypes.PeerInfo{
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

		if err = writer.Set(dhruva.NtqDBKey(peerID), rawData); err != nil {
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
	helloMsg := new(ptypes.HelloMsg)

	if err := helloMsg.FromBytes(msg.Data); err != nil {
		return err
	}

	r.logger.Debug("Received Hello Message", "Peer id", helloMsg.KramaID)

	if err := r.pendingMessageQueue.Push(&gtypes.NodeMetaInfoMsg{
		KramaID:       helloMsg.KramaID,
		Address:       helloMsg.Address,
		PeerSignature: helloMsg.Signature,
		NTQ:           DefaultPeerNTQ,
	}); err != nil {
		r.signalNewMessages()
	}

	return nil
}

func (r *ReputationEngine) handleMessages(msgs []*gtypes.NodeMetaInfoMsg) {
	if len(msgs) == 0 {
		return
	}

	kramaIDs := make([]id.KramaID, len(msgs))

	for index, msg := range msgs {
		if msg.KramaID != "" {
			kramaIDs[index] = msg.KramaID
		}
	}

	publicKeys, err := r.getPublicKeyFromContract(kramaIDs...)
	if err != nil {
		r.logger.Error("Error fetching public key", "error", err)

		return
	}

	for index, publicKey := range publicKeys {
		msg := msgs[index]

		rawData, err := msg.HelloMessageBytes()
		if err != nil {
			r.logger.Error("Failed to fetch address bytes from message", "error", err)

			continue
		}

		verified, err := mudra.Verify(rawData, msg.PeerSignature, publicKey)
		if !verified || err != nil {
			r.logger.Error("Signature verification failed", "error", err)

			continue
		}

		if err = r.UpdatePeer(msg.KramaID, &gtypes.NodeMetaInfo{
			Addrs:         msg.Address,
			NTQ:           msg.NTQ,
			WalletCount:   msg.WalletCount,
			PublicKey:     publicKey,
			PeerSignature: msg.PeerSignature,
		}); err != nil {
			r.logger.Error("Failed to add node meta info", "error", err, "krama id", msg.KramaID)

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
			r.logger.Info("Closing reputation worker")

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
			r.logger.Info("Closing reputation worker")

			return
		}

		if err := r.flushDirtyEntries(); err != nil {
			r.logger.Error("Error flushing dirty entries", "error", err)

			continue
		}

		r.cleanUpDirtyStorage()
	}
}

func (r *ReputationEngine) cleanUpDirtyStorage() {
	r.dirtyLock.Lock()
	defer r.dirtyLock.Unlock()

	r.dirtyEntries = make(map[peer.ID]*gtypes.NodeMetaInfo)
}

func (r *ReputationEngine) Start() error {
	if err := r.network.Subscribe(r.ctx, GossipTopic, r.senatusHandler); err != nil {
		return errors.Wrap(err, "failed to subscribe senatus topic")
	}

	go r.messageWorker()
	go r.dbWorker()

	return nil
}

type Response struct {
	Data []string `json:"data"`
}
type Request struct {
	Ids []string `json:"kramaIDs"`
}

var RetrievePublicKeys = func(ids []id.KramaID, client *http.Client, logger hclog.Logger) (keys [][]byte, err error) {
	data, err := json.Marshal(Request{utils.KramaIDToString(ids)})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "http://91.107.196.74/api/fetchPublicKeys", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	response, err := client.Do(req)
	if err != nil {
		logger.Error("Api fetch failed", "error", err, "kramaIDs", ids)

		return nil, err
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Panicln(err)
	}

	defer response.Body.Close()

	if response.StatusCode != 200 {
		logger.Error("Http request failed", response.StatusCode, string(body))
	}

	data1 := new(Response)

	if err = json.Unmarshal(body, data1); err != nil {
		log.Panicln(err)
	}

	for _, v := range data1.Data {
		str, err := hex.DecodeString(v)
		if err != nil {
			return nil, err
		}

		keys = append(keys, str)
	}

	return keys, nil
}
