package types

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/rs/zerolog/log"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"golang.org/x/crypto/blake2b"
)

const MaxPeerListSize = 5

type Message interface {
	GetSessionID() types.Address
}

type Response struct {
	PeerID    kramaid.KramaID
	SessionID types.Address
	StateHash CID
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
		blocks = append(blocks, NewBlockFromMessage(data))
	}

	return blocks
}

func (resp *AgoraResponseMsg) GetSessionID() types.Address {
	return resp.SessionID
}

func (resp *AgoraResponseMsg) FromBytes(bytes []byte) error {
	err := polo.Depolorize(resp, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize agora response message")
	}

	return nil
}

type AgoraRequestMsg struct {
	SessionID types.Address
	StateHash CID
	WantList  []CID
}

func (req *AgoraRequestMsg) GetSessionID() types.Address {
	return req.SessionID
}

func (req *AgoraRequestMsg) FromBytes(bytes []byte) error {
	err := polo.Depolorize(req, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize agora request message")
	}

	return nil
}

type Block struct {
	cid  CID
	data []byte
}

func NewBlockFromMessage(data []byte) Block {
	hash := blake2b.Sum256(data[1:])

	return Block{
		cid:  ContentID(data[0], hash),
		data: data[1:],
	}
}

func NewBlockFromRawData(contentType byte, data []byte) Block {
	hash := blake2b.Sum256(data)

	return Block{
		cid:  ContentID(contentType, hash),
		data: data,
	}
}

func (b Block) GetData() []byte {
	return b.data
}

func (b Block) GetCid() CID {
	return b.cid
}

func (b Block) BytesForMessage() []byte {
	rawBytes := make([]byte, 0, len(b.data)+1)

	rawBytes = append(rawBytes, b.cid.ContentType())
	rawBytes = append(rawBytes, b.data...)

	return rawBytes
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

func (h *HaveList) GetKeys() []CID {
	cIDs := make([]CID, len(h.blocks))

	for k, v := range h.blocks {
		cIDs[k] = v.cid
	}

	return cIDs
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
		rawBlocks = append(rawBlocks, block.BytesForMessage())
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

func (clist *CanonicalPeerList) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(clist)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical peer list")
	}

	return rawData, nil
}

func (clist *CanonicalPeerList) FromBytes(bytes []byte) error {
	err := polo.Depolorize(clist, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize canonical peer list")
	}

	return nil
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
	fetched   *Queue
	liveWants map[CID]time.Time
}

func NewWantTracker() *WantTracker {
	return &WantTracker{
		fetched:   NewCidQueue(),
		liveWants: make(map[CID]time.Time),
	}
}

func (wt *WantTracker) UpdateLiveWants(keys *CIDSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	if err := keys.ForEach(func(c CID) error {
		reqTime, ok := wt.liveWants[c]
		if !ok || time.Since(reqTime) > 200*time.Millisecond {
			wt.liveWants[c] = time.Now()
		}

		return nil
	}); err != nil {
		log.Print("error removing redundant keys")
	}
}

func (wt *WantTracker) RemoveRedundantKeys(cids *CIDSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	redundantKeys := make([]CID, 0)

	if err := cids.ForEach(func(c CID) error {
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

func (wt *WantTracker) RemoveCid(cid CID) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	delete(wt.liveWants, cid)
}
