package flux

import (
	"bufio"
	"context"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	id "github.com/sarvalabs/moichain/common/kramaid"
	networkmsg "github.com/sarvalabs/moichain/network/message"

	"github.com/libp2p/go-msgio"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/config"
	"github.com/sarvalabs/moichain/common/utils"
	"github.com/sarvalabs/moichain/network/p2p"
	"github.com/sarvalabs/moichain/telemetry/tracing"
)

const (
	SLOTCOUNT  = 20
	PEERSCOUNT = 20
)

type Randomizer struct {
	ctx        context.Context
	ctxCancel  context.CancelFunc
	bootNodes  map[peer.ID]bool
	peers      []*PeerList
	requestIDs []int64
	topic      string
	server     *p2p.Server
	logger     hclog.Logger
	metrics    *Metrics
}

type PeerList struct {
	mtx           sync.RWMutex
	updatePending bool
	lastRequest   time.Time
	nonUtilized   map[id.KramaID]int
	pendingCount  int
}

func NewRandomizer(
	ctx context.Context,
	logger hclog.Logger,
	p2pServer *p2p.Server,
	metrics *Metrics,
) *Randomizer {
	ctx, ctxCancel := context.WithCancel(ctx)
	r := &Randomizer{
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		peers:      make([]*PeerList, SLOTCOUNT),
		requestIDs: make([]int64, SLOTCOUNT),
		server:     p2pServer,
		logger:     logger.Named("Flux-Engine"),
		metrics:    metrics,
	}

	r.bootNodes, _ = p2pServer.GetBootstrapPeerIDs()

	for i := 0; i < SLOTCOUNT; i++ {
		r.peers[i] = &PeerList{
			updatePending: true,
			lastRequest:   time.Now(),
			nonUtilized:   make(map[id.KramaID]int),
			pendingCount:  PEERSCOUNT,
		}
		r.requestIDs[i] = -1
	}

	r.metrics.initMetrics(SLOTCOUNT)
	r.server.SetupStreamHandler(config.FluxProtocolStream, r.messageHandler)

	return r
}

func (r *Randomizer) isBootstrapNode(peerID peer.ID) bool {
	_, ok := r.bootNodes[peerID]

	return ok
}

func (r *Randomizer) messageHandler(stream network.Stream) {
	// r.logger.Trace(
	//	"Got a new flux stream.",
	//	"protocol-ID", stream.Protocol(),
	//	"remotepeer-ID", stream.Conn().RemotePeer())
	r.metrics.captureNumOfRequests(1)

	defer func() {
		if err := stream.Close(); err != nil {
			r.logger.Error("Error closing flux stream", "err", err)
		}
	}()
	// Create a new read/write buffer

	reader := msgio.NewReader(stream)

	buffer, err := reader.ReadMsg()
	if err != nil {
		r.logger.Error("Error reading buffer", "err", err)

		return
	}

	message := new(networkmsg.Message)

	if err := message.FromBytes(buffer); err != nil {
		r.logger.Error("Error reading message", "err", err)

		return
	}

	randomWalkReqMsg := new(networkmsg.RandomWalkReq)

	if err := randomWalkReqMsg.FromBytes(message.Payload); err != nil {
		r.logger.Error("Error reading message", "err", err)

		return
	}

	if err = r.HandleReqMsg(randomWalkReqMsg); err != nil {
		r.logger.Error("Unable to handle random walk request", "err", err)

		return
	}
}

// addPeer
func (r *Randomizer) addPeer(slot int, id id.KramaID) error {
	r.peers[slot].mtx.Lock()
	defer r.peers[slot].mtx.Unlock()

	if v, ok := r.peers[slot].nonUtilized[id]; !ok || v != 1 {
		r.peers[slot].pendingCount--

		r.peers[slot].nonUtilized[id] = 1

		if len(r.peers[slot].nonUtilized) > PEERSCOUNT {
			randIndex := rand.Intn(len(r.peers[slot].nonUtilized))
			count := 0

			for k := range r.peers[slot].nonUtilized {
				count++
				if randIndex != count {
					continue
				}

				delete(r.peers[slot].nonUtilized, k)

				break
			}
		}

		r.updatePeerListStatus(slot)
	}

	return nil
}

func (r *Randomizer) updatePeerListStatus(slot int) {
	if !r.peers[slot].updatePending && r.peers[slot].pendingCount >= int(math.Ceil(0.8*PEERSCOUNT)) {
		r.peers[slot].updatePending = true
		r.metrics.capturePendingSlots(1)
	} else if r.peers[slot].updatePending && r.peers[slot].pendingCount < int(math.Ceil(0.8*PEERSCOUNT)) {
		r.peers[slot].updatePending = false
		r.metrics.capturePendingSlots(-1)
	}
}

func (r *Randomizer) Start() {
	r.topic = utils.RandString(64)
	if err := r.server.Subscribe(r.ctx, r.topic, r.pubSubHandler); err != nil {
		r.logger.Error("Error subscribing to flux topic", "err", err)

		log.Panic(err)
	}

	go func() {
		for {
			select {
			case <-r.ctx.Done():
				r.logger.Info("Closing randomizer")

				return
			case <-time.After(300 * time.Millisecond):
				if uint32(r.server.Peers.Len()) >= 3 {
					for k, v := range r.peers {
						v.mtx.RLock()
						lastRequest := v.lastRequest
						updateRequired := v.updatePending
						v.mtx.RUnlock()

						if updateRequired && time.Since(lastRequest).Milliseconds() > PEERSCOUNT*150 {
							//	log.Println("Populating the pool for slot", k)
							r.PopulatePool(k)
						}
					}
				}
			}
		}
	}()
}

func (r *Randomizer) getPeers(slotNo int, count int, avoidPeers []id.KramaID) []id.KramaID {
	// log.Println("Querying for random peers", slotNo, count)
	//	log.Println("Avoid peers", avoidPeers)
	counter := 0
	list := make([]id.KramaID, 0)

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

	return list
}

func (r *Randomizer) HandleReqMsg(reqMsg *networkmsg.RandomWalkReq) error {
	requesterID := reqMsg.PeerID

	peerID, err := requesterID.PeerID()
	if err != nil {
		r.logger.Error("Error parsing krama peer ID", "err", err)

		return err
	}

	for {
		randomPeer := r.server.GetRandomNode()

		// if the random peer is either request or bootstrap node, don't send request
		if randomPeer == peer.ID(peerID) || r.isBootstrapNode(randomPeer) {
			continue
		}

		if reqMsg.Count-1 > 0 {
			msg := &networkmsg.RandomWalkReq{
				ReqID:  reqMsg.ReqID,
				Count:  reqMsg.Count - 1,
				PeerID: requesterID,
				Topic:  reqMsg.Topic,
			}

			// forward the request
			if err = r.SendFluxMessage(randomPeer, networkmsg.RANDOMWALKREQ, msg); err != nil {
				r.logger.Error(
					"Unable to forward the random walk request",
					"err", err,
					"peer", randomPeer.String(),
				)

				continue
			}
		}

		responseMsg := networkmsg.RandomWalkResp{
			ReqID:    reqMsg.ReqID,
			PeerAddr: utils.MultiAddrToString(r.server.GetAddrs()...),
			ID:       r.server.GetKramaID(),
		}

		// log.Println("Address",responseMsg,polo.Polorize(responseMsg),polo.Polorize(&responseMsg))
		rawData, err := responseMsg.Bytes()
		if err != nil {
			return err
		}

		_, err = r.server.JoinPubSubTopic(reqMsg.Topic)
		if err != nil {
			return err
		}

		err = r.server.Broadcast(reqMsg.Topic, rawData)
		if err != nil {
			r.logger.Error("Failed to broadcast", "err", err)
		}

		return nil
	}
}

func (r *Randomizer) pubSubHandler(msg *pubsub.Message) error {
	data := msg.GetData()
	randomPeerMsg := new(networkmsg.RandomWalkResp)

	if err := randomPeerMsg.FromBytes(data); err != nil {
		r.logger.Error("Error depolarising random walk request", "err", err)

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
	reg := rand.New(s1)
	reqID := reg.Int63()

	r.requestIDs[slot] = reqID

	return reqID
}

func (r *Randomizer) PopulatePool(slotID int) {
	msg := &networkmsg.RandomWalkReq{
		ReqID:  r.getRequestID(slotID),
		Count:  2 * PEERSCOUNT,
		Topic:  r.topic,
		PeerID: r.server.GetKramaID(),
	}

	// Step 1: Select some random peer from random table and
	for {
		randomPeer := r.server.GetRandomNode()

		if r.isBootstrapNode(randomPeer) {
			continue
		}

		if err := r.SendFluxMessage(randomPeer, networkmsg.RANDOMWALKREQ, msg); err != nil {
			continue
		}

		r.peers[slotID].mtx.Lock()
		r.peers[slotID].lastRequest = time.Now()
		r.peers[slotID].mtx.Unlock()

		return
	}
}

func (r *Randomizer) GetRandomNodes(
	ctx context.Context,
	count int,
	avoidPeers []id.KramaID,
) (randomPeers []id.KramaID, err error) {
	_, span := tracing.Span(ctx, "Flux.Randomizer", "GetRandomNodes")
	defer span.End()

	requiredNo := count

	for {
		select {
		case <-ctx.Done():
			return nil, common.ErrTimeOut
		default:
			if requiredNo <= 0 {
				return
			}

			s1 := rand.NewSource(time.Now().UnixNano())
			reg := rand.New(s1)
			slotNo := reg.Intn(SLOTCOUNT)

			peers := r.getPeers(slotNo, requiredNo, avoidPeers)
			if len(peers) == 0 {
				continue
			}

			randomPeers = append(randomPeers, peers...)
			if len(randomPeers) >= count {
				return
			}

			avoidPeers = append(avoidPeers, randomPeers...)
			requiredNo -= len(peers)
		}
	}
}

func (r *Randomizer) Close() {
	defer r.ctxCancel()
}

func (r *Randomizer) SendFluxMessage(peerID peer.ID, msgType networkmsg.MsgType, msg interface{}) error {
	rawData, err := polo.Polorize(msg)
	if err != nil {
		return errors.Wrap(err, "failed to polorize message payload")
	}
	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into a slice of bytes
	m := networkmsg.Message{
		MsgType: msgType,
		Payload: rawData,
		Sender:  r.server.GetKramaID(),
	}

	stream, err := r.server.NewStream(context.Background(), peerID, config.FluxProtocolStream)
	if err != nil {
		// Return error if stream setup fails
		return err
	}

	defer func() {
		if err := stream.Close(); err != nil {
			r.logger.Error("Error closing flux stream", "err", err)
		}
	}()

	rawData, err = m.Bytes()
	if err != nil {
		return err
	}

	wr := bufio.NewWriter(stream)
	// Write the message bytes into the peer's io buffer
	writer := msgio.NewWriter(wr)
	if err := writer.WriteMsg(rawData); err != nil {
		return err
	}

	return wr.Flush()
}
