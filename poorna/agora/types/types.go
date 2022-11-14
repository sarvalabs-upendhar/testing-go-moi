package types

import (
	"sync"
	"time"

	"gitlab.com/sarvalabs/moichain/utils"

	"github.com/rs/zerolog/log"
	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
	"golang.org/x/crypto/blake2b"
)

const MaxPeerListSize = 5

type Message interface {
	GetSessionID() types.Address
}

type Response struct {
	PeerID    kramaid.KramaID
	SessionID types.Address
	StateHash types.Hash
	Status    bool
	HaveList  HaveList
	PeerSet   []kramaid.KramaID
}

func (r *Response) GetAgoraMsg() *AgoraResponseMsg {
	return &AgoraResponseMsg{
		SessionID: r.SessionID,
		Status:    r.Status,
		HaveList:  r.HaveList.GetRawBlocks(),
		PeerSet:   r.PeerSet,
	}
}

type AgoraResponseMsg struct {
	SessionID types.Address
	Status    bool
	HaveList  [][]byte
	PeerSet   []kramaid.KramaID
}

func (resp *AgoraResponseMsg) GetBlocks() []Block {
	blocks := make([]Block, 0, len(resp.HaveList))

	for _, data := range resp.HaveList {
		blocks = append(blocks, NewBlock(data))
	}

	return blocks
}

func (resp *AgoraResponseMsg) GetSessionID() types.Address {
	return resp.SessionID
}

type AgoraRequestMsg struct {
	SessionID types.Address
	StateHash types.Hash
	WantList  []types.Hash
}

func (req *AgoraRequestMsg) GetSessionID() types.Address {
	return req.SessionID
}

type Block struct {
	id   types.Hash
	data []byte
}

func NewBlock(data []byte) Block {
	hash := blake2b.Sum256(data)

	return Block{
		id:   hash,
		data: data,
	}
}

func (b *Block) GetData() []byte {
	return b.data
}

func (b *Block) GetID() types.Hash {
	return b.id
}

type HaveList struct {
	blocks []Block
}

func NewHaveList() HaveList {
	return HaveList{
		blocks: make([]Block, 0),
	}
}

func (h *HaveList) Size() int {
	return len(h.blocks)
}

func (h *HaveList) GetKeys() []types.Hash {
	hashes := make([]types.Hash, len(h.blocks))

	for k, v := range h.blocks {
		hashes[k] = v.id
	}

	return hashes
}

func (h *HaveList) GetBlocks() []Block {
	return h.blocks
}

func (h *HaveList) AddBlock(b Block) {
	h.blocks = append(h.blocks, b)
}

func (h *HaveList) GetRawBlocks() [][]byte {
	rawBlocks := make([][]byte, 0, len(h.blocks))

	for _, block := range h.blocks {
		rawBlocks = append(rawBlocks, block.data)
	}

	return rawBlocks
}

type PeerList struct {
	mtx           sync.RWMutex
	lastUpdatedAt int64
	peers         map[kramaid.KramaID]struct{}
}

type CanonicalPeerList struct {
	Peers         []kramaid.KramaID
	LastUpdatedAt int64
}

func (clist *CanonicalPeerList) PeerList() *PeerList {
	peerList := NewPeerList()
	for _, peerID := range clist.Peers {
		peerList.AddPeer(peerID)
	}

	return peerList
}

func NewPeerList() *PeerList {
	p := &PeerList{
		lastUpdatedAt: 0,
		peers:         make(map[kramaid.KramaID]struct{}),
	}

	return p
}

func (plist *PeerList) Peers() []kramaid.KramaID {
	plist.mtx.RLock()
	defer plist.mtx.RUnlock()

	ids := make([]kramaid.KramaID, 0, len(plist.peers))

	for k := range plist.peers {
		ids = append(ids, k)
	}

	return ids
}

func (plist *PeerList) LastUpdatedAt() int64 {
	plist.mtx.Lock()
	defer plist.mtx.Unlock()

	return plist.lastUpdatedAt
}

func (plist *PeerList) AddPeer(peerID kramaid.KramaID) {
	plist.mtx.Lock()
	defer plist.mtx.Unlock()

	if _, ok := plist.peers[peerID]; !ok {
		if len(plist.peers) == MaxPeerListSize {
			for peerID := range plist.peers {
				delete(plist.peers, peerID)

				break
			}
		}

		plist.peers[peerID] = struct{}{}
		plist.lastUpdatedAt = time.Now().UnixNano()
	}
}

func (plist *PeerList) Size() int {
	plist.mtx.RLock()
	defer plist.mtx.RUnlock()

	return len(plist.peers)
}

func (plist *PeerList) CanonicalPeerList() *CanonicalPeerList {
	cp := &CanonicalPeerList{
		Peers:         plist.Peers(),
		LastUpdatedAt: plist.LastUpdatedAt(),
	}

	return cp
}

type WantTracker struct {
	mtx       sync.RWMutex
	fetched   *utils.Queue
	liveWants map[types.Hash]time.Time
}

func NewWantTracker() *WantTracker {
	return &WantTracker{
		fetched:   utils.NewCidQueue(),
		liveWants: make(map[types.Hash]time.Time),
	}
}

func (wt *WantTracker) UpdateLiveWants(keys *utils.HashSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	if err := keys.ForEach(func(c types.Hash) error {
		reqTime, ok := wt.liveWants[c]
		if !ok || time.Since(reqTime) > 200*time.Millisecond {
			wt.liveWants[c] = time.Now()
		}

		return nil
	}); err != nil {
		log.Print("error removing redundant keys")
	}
}

func (wt *WantTracker) RemoveRedundantKeys(cids *utils.HashSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	redundantKeys := make([]types.Hash, 0)

	if err := cids.ForEach(func(c types.Hash) error {
		reqTime, ok := wt.liveWants[c]

		if ok && time.Since(reqTime) < 200*time.Millisecond {
			redundantKeys = append(redundantKeys, c)
		}

		return nil
	}); err != nil {
		log.Print("error removing redundant keys")
	}

	for _, cid := range redundantKeys {
		cids.Remove(cid)
	}
}

func (wt *WantTracker) RemoveCid(cid types.Hash) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	delete(wt.liveWants, cid)
}
