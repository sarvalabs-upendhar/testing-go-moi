package flux

import (
	"bufio"
	"context"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna"
	"gitlab.com/sarvalabs/polo/go-polo"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

const (
	SLOTCOUNT  = 20
	PEERSCOUNT = 6
)
const (
	FluxProtocol = protocol.ID("moi/stream/flux")
)

type Randomizer struct {
	ctx        context.Context
	ctxCancel  context.CancelFunc
	peers      []*PeerList
	requestIDs []int64
	topic      string
	server     *poorna.Server
	logger     hclog.Logger
}

type PeerList struct {
	mtx           sync.RWMutex
	updatePending bool
	lastUpdated   time.Time
	nonUtilized   map[id.KramaID]int
	pendingCount  int
}

func NewRandomizer(ctx context.Context, logger hclog.Logger, p2pServer *poorna.Server) *Randomizer {
	ctx, ctxCancel := context.WithCancel(ctx)
	r := &Randomizer{
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		peers:      make([]*PeerList, SLOTCOUNT),
		requestIDs: make([]int64, SLOTCOUNT),
		server:     p2pServer,
		logger:     logger.Named("Flux-Engine"),
	}

	for i := 0; i < SLOTCOUNT; i++ {
		r.peers[i] = &PeerList{
			updatePending: true,
			lastUpdated:   time.Now(),
			nonUtilized:   make(map[id.KramaID]int),
			pendingCount:  6,
		}
		r.peers[i].updatePending = true
		r.requestIDs[i] = -1
	}

	r.server.SetupStreamHandler(FluxProtocol, r.messageHandler)

	return r
}
func (r *Randomizer) messageHandler(stream network.Stream) {
	//r.logger.Debug("Got a new flux Stream", stream.Protocol(), stream.Conn().RemotePeer())
	defer func() {
		if err := stream.Close(); err != nil {
			r.logger.Error("Error closing flux stream", "error", err)
		}
	}()
	// Create a new read/write buffer
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	buffer := make([]byte, 4096)

	count, err := rw.Reader.Read(buffer)
	if err != nil {
		r.logger.Error("Error reading buffer", "err", err)

		return
	}

	message := new(ktypes.Message)

	err = polo.Depolorize(message, buffer[0:count])
	if err != nil {
		r.logger.Error("Error reading message", "err", err)

		return
	}

	msg := new(ktypes.RandomWalkReq)

	if err = polo.Depolorize(msg, message.Payload); err != nil {
		r.logger.Error("Error reading message", "err", err)

		return
	}

	if err = r.HandleReqMsg(msg); err != nil {
		r.logger.Error("Unable to handle random walk request", "err", err)

		return
	}
}

// addPeer
func (r *Randomizer) addPeer(slot int, id id.KramaID) error {
	r.peers[slot].mtx.Lock()
	defer r.peers[slot].mtx.Unlock()
	//log.Println("Add peer", slot, id)
	if v, ok := r.peers[slot].nonUtilized[id]; !ok || v != 1 {
		r.peers[slot].pendingCount--

		r.peers[slot].nonUtilized[id] = 1
		r.peers[slot].lastUpdated = time.Now()

		for k, v := range r.peers[slot].nonUtilized {
			if v == -1 {
				delete(r.peers[slot].nonUtilized, k)

				break
			}
		}

		r.updatePeerListStatus(slot)
	}

	return nil
}

func (r *Randomizer) updatePeerListStatus(slot int) {
	//log.Println("In update slot status", slot, r.peers[slot].pendingCount, int(math.Ceil(0.6*PEERSCOUNT)))
	if r.peers[slot].pendingCount >= int(math.Ceil(0.4*PEERSCOUNT)) {
		r.peers[slot].updatePending = true
	} else {
		r.peers[slot].updatePending = false
	}
}
func (r *Randomizer) Start() {
	r.topic = kutils.RandString(64)
	if err := r.server.Subscribe(r.topic, r.pubSubHandler); err != nil {
		r.logger.Error("Error subscribing to flux topic", err)

		log.Panic(err)
	}

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)

		for {
			select {
			case <-r.ctx.Done():
				r.logger.Info("Closing randomizer")

				return
			case <-ticker.C:
				if r.server.Peers.Len() > 8 {
					for k, v := range r.peers {
						v.mtx.RLock()
						lastUpdates := v.lastUpdated
						updateRequired := v.updatePending
						v.mtx.RUnlock()

						if updateRequired && time.Since(lastUpdates).Milliseconds() > 800 {
							//	log.Println("Populating the pool for slot", k)
							r.PopulatePool(k)
						}
					}
				}
			}
		}
	}()
}

func (r *Randomizer) getPeers(slotNo int, count int, avoidPeers []id.KramaID) (list []id.KramaID) {
	//log.Println("Querying for random peers", slotNo, count)
	//	log.Println("Avoid peers", avoidPeers)
	counter := 0

	r.peers[slotNo].mtx.Lock()
	defer r.peers[slotNo].mtx.Unlock()

	for _, v := range avoidPeers {
		if status, ok := r.peers[slotNo].nonUtilized[v]; ok && status != -1 {
			r.peers[slotNo].nonUtilized[v] = 0
		}
	}

	for k, v := range r.peers[slotNo].nonUtilized {
		if counter == count {
			break
		}

		if v == 1 {
			list = append(list, k)
			r.peers[slotNo].nonUtilized[k] = -1

			r.peers[slotNo].pendingCount++
			counter++
		}
	}

	for _, v := range avoidPeers {
		if status, ok := r.peers[slotNo].nonUtilized[v]; ok && status == 0 {
			r.peers[slotNo].nonUtilized[v] = 1
		}
	}

	r.updatePeerListStatus(slotNo)

	return
}

func (r *Randomizer) HandleReqMsg(reqMsg *ktypes.RandomWalkReq) error {
	requesterID := reqMsg.PeerID

	for {
		randomPeer := r.server.GetRandomNode()

		peerID, err := requesterID.PeerID()
		if err != nil {
			r.logger.Error("Error parsing krama peerID", "error", err)

			return err
		}

		if randomPeer != peer.ID(peerID) {
			if reqMsg.Count-1 > 0 {
				msg := &ktypes.RandomWalkReq{
					ReqID:  reqMsg.ReqID,
					Count:  reqMsg.Count - 1,
					PeerID: requesterID,
					Topic:  reqMsg.Topic,
				}

				// forward the request
				if err := r.SendFluxMessage(randomPeer, ktypes.RANDOMWALKREQ, msg); err != nil {
					//log.Println("Unable to forward the random walk request", err, randomPeer.String())
					continue
				}
			}
		}

		responseMsg := ktypes.RandomWalkResp{
			ReqID:    reqMsg.ReqID,
			PeerAddr: kutils.MultiAddrToString(r.server.GetAddrs()...),
			ID:       r.server.GetKramaID(),
		}

		//log.Println("Address",responseMsg,polo.Polorize(responseMsg),polo.Polorize(&responseMsg))
		err = r.server.Broadcast(reqMsg.Topic, polo.Polorize(responseMsg))
		if err != nil {
			log.Panicln(err)
		}

		return nil
	}
}
func (r *Randomizer) pubSubHandler(ctx context.Context, msg *pubsub.Message) error {
	data := msg.GetData()
	randomPeerMsg := new(ktypes.RandomWalkResp)
	//log.Println("Here",msg.ReceivedFrom,randomPeerMsg,data)
	err := polo.Depolorize(randomPeerMsg, data)
	if err != nil {
		r.logger.Error("Error depolarising randomWalk Request", "error", err)

		return err
	}

	if slot, ok := r.isValidRequestID(randomPeerMsg.ReqID); ok {
		if err := r.addPeer(slot, randomPeerMsg.ID); err != nil {
			log.Println("unable to add peer to the slot")
		}
	} else {
		log.Println("Invalid request id")
	}

	return nil
}
func (r *Randomizer) isValidRequestID(reqID int64) (int, bool) {
	for slot, v := range r.requestIDs {
		if reqID != -1 && reqID == v {
			return slot, true
		}
	}

	return -1, false
}
func (r *Randomizer) getRequestID(slot int) int64 {
	if r.requestIDs[slot] != -1 {
		return r.requestIDs[slot]
	}
	s1 := rand.NewSource(time.Now().UnixNano())
	reg := rand.New(s1) //nolint
	reqID := reg.Int63()
	r.requestIDs[slot] = reqID

	return reqID
}

func (r *Randomizer) PopulatePool(slotID int) {
	// Step 1: Select some random peer from random table and
	for {
		randomPeer := r.server.GetRandomNode()
		// Step 2:
		msg := &ktypes.RandomWalkReq{
			ReqID:  r.getRequestID(slotID),
			Count:  PEERSCOUNT,
			Topic:  r.topic,
			PeerID: r.server.GetKramaID(),
		}
		//log.Println("Sending random request", r.getRequestID(slotID), slotID)
		if err := r.SendFluxMessage(randomPeer, ktypes.RANDOMWALKREQ, msg); err != nil {
			continue
		}

		return
	}
}

func (r *Randomizer) GetRandomNodes(
	ctx context.Context,
	count int,
	avoidPeers []id.KramaID,
) (randomPeers []id.KramaID, err error) {
	requiredNo := count

	for {
		select {
		case <-ctx.Done():
			return nil, ktypes.ErrTimeOut
		default:
			if requiredNo <= 0 {
				return
			}
			s1 := rand.NewSource(time.Now().UnixNano())
			reg := rand.New(s1) //nolint
			slotNo := reg.Intn(SLOTCOUNT)

			peers := r.getPeers(slotNo, requiredNo, avoidPeers)
			if len(peers) <= 0 {
				continue
			}

			randomPeers = append(randomPeers, peers...)
			if len(randomPeers) >= count {
				return
			}

			avoidPeers = append(avoidPeers, randomPeers...)
			requiredNo = requiredNo - len(randomPeers)
		}
	}
}

func (r *Randomizer) Close() {
	defer r.ctxCancel()
}

func (r *Randomizer) SendFluxMessage(peerID peer.ID, msgType ktypes.MsgType, msg interface{}) error {
	//if s.Peers.ContainsPeer(peerID) {
	//	p := s.Peers.Peer(peerID)
	//	return p.(s.id, msgType, msg)
	//}
	bytes := polo.Polorize(msg)
	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into a slice of bytes
	m := ktypes.Message{
		MsgType: msgType,
		Payload: bytes,
		Sender:  r.server.GetKramaID(),
	}

	stream, err := r.server.NewStream(context.Background(), FluxProtocol, peerID)
	if err != nil {
		// Return error if stream setup fails
		return err
	}

	defer func() {
		if err := stream.Close(); err != nil {
			r.logger.Error("Error closing flux stream")
		}
	}()

	// Create a new read/write buffer
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	// Create a NewPeerEvent

	rawData := polo.Polorize(m)

	// Write the message bytes into the peer's io buffer
	_, err = rw.Writer.Write(rawData)
	if err != nil {
		return err
	}

	// Flush the peer's io buffer. This will push the message to the network
	return rw.Flush()
}
