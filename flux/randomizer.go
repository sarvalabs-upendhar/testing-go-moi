package flux

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common/utils"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
)

const (
	SLOTCOUNT       = 20
	PEERSCOUNT      = 20
	MINCONTEXTPOWER = 0
)

type Randomizer struct {
	ctx         context.Context
	ctxCancel   context.CancelFunc
	peers       []*PeerList
	deletePeers chan []identifiers.KramaID
	server      p2pServer
	senatus     reputationEngine
	logger      hclog.Logger
	metrics     *Metrics
}

type PeerList struct {
	mtx           sync.RWMutex
	updatePending bool
	lastRequest   time.Time
	nonUtilized   map[identifiers.KramaID]int
	pendingCount  int
}

type p2pServer interface {
	GetPeersCount() int
}

type reputationEngine interface {
	StreamPeerInfos(ctx context.Context) (chan *senatus.PeerInfo, error)
	TotalPeerCount() uint64
	DeletePeers(ids []identifiers.KramaID) error
}

func NewRandomizer(
	logger hclog.Logger,
	server p2pServer,
	reputationEngine reputationEngine,
	metrics *Metrics,
) *Randomizer {
	ctx, ctxCancel := context.WithCancel(context.Background())
	r := &Randomizer{
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		peers:       make([]*PeerList, SLOTCOUNT),
		deletePeers: make(chan []identifiers.KramaID),
		server:      server,
		senatus:     reputationEngine,
		logger:      logger.Named("Flux-Engine"),
		metrics:     metrics,
	}

	for i := 0; i < SLOTCOUNT; i++ {
		r.peers[i] = &PeerList{
			updatePending: true,
			lastRequest:   time.Now(),
			nonUtilized:   make(map[identifiers.KramaID]int),
			pendingCount:  PEERSCOUNT,
		}
	}

	r.metrics.initMetrics(SLOTCOUNT)

	return r
}

func (r *Randomizer) DeletePeers(ids []identifiers.KramaID) {
	r.deletePeers <- ids
}

func (r *Randomizer) addPeers(slot int) {
	ctx, cancel := context.WithCancel(r.ctx)

	r.peers[slot].mtx.Lock()
	defer func() {
		r.peers[slot].mtx.Unlock()
		cancel()
	}()

	r.peers[slot].pendingCount = PEERSCOUNT

	// Retrieve the keys and values from the db
	peerInfos, err := r.senatus.StreamPeerInfos(ctx)
	if err != nil {
		r.logger.Error("Unable to retrieve keys from the db", "err", err)

		return
	}

	// TODO Fix total peer count logic, it should be changed to include only peers that have minimum context power
	// Retrieve the total number of peers from the db
	peerCount := r.senatus.TotalPeerCount()
	if peerCount == 0 {
		r.logger.Error("Error fetching the total peer count", "err", err)

		return
	}

	minValue := PEERSCOUNT
	if int(peerCount) < minValue {
		minValue = int(peerCount)
	}

	randomNumbers := utils.GetRandomNumbers(minValue, int(peerCount))

	counter := 0
	desiredCount := 0
	nonUtilized := make(map[identifiers.KramaID]int)

	// Read values from the channel
	for peerInfo := range peerInfos {
		if desiredCount == minValue {
			break
		}

		if _, ok := randomNumbers[counter]; ok {
			info := new(senatus.NodeMetaInfo)
			if err = info.FromBytes(peerInfo.Data); err != nil {
				r.logger.Error("Error reading message", "err", err)

				return
			}

			// peer should have minimum context power to be part of random set / context nodes.
			if info.ContextPower < MINCONTEXTPOWER {
				continue
			}

			if _, ok := nonUtilized[info.KramaID]; !ok {
				r.peers[slot].pendingCount--
				desiredCount++

				nonUtilized[info.KramaID] = 1
			}
		}

		counter++
	}

	r.peers[slot].lastRequest = time.Now()
	r.peers[slot].nonUtilized = nonUtilized

	r.updatePeerListStatus(slot)
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
	go func() {
		for {
			select {
			case <-r.ctx.Done():
				r.logger.Info("Closing randomizer")

				return
			case peers := <-r.deletePeers:
				if err := r.senatus.DeletePeers(peers); err != nil {
					r.logger.Error("Error deleting peers in db", "err", err)
				}

				for slot := 0; slot < SLOTCOUNT; slot++ {
					r.peers[slot].mtx.Lock()

					for _, peer := range peers {
						delete(r.peers[slot].nonUtilized, peer)
					}

					r.peers[slot].mtx.Unlock()
				}
			case <-time.After(300 * time.Millisecond):
				if uint32(r.server.GetPeersCount()) >= 3 {
					for k, v := range r.peers {
						v.mtx.RLock()
						lastRequest := v.lastRequest
						updateRequired := v.updatePending
						v.mtx.RUnlock()

						if updateRequired && time.Since(lastRequest).Milliseconds() > PEERSCOUNT*15 {
							r.addPeers(k)
						}
					}
				}
			}
		}
	}()
}

func (r *Randomizer) getPeers(slotNo int, count int, avoidPeers []identifiers.KramaID) []identifiers.KramaID {
	counter := 0
	list := make([]identifiers.KramaID, 0)

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

func (r *Randomizer) GetRandomNodes(
	ctx context.Context,
	count int,
	avoidPeers []identifiers.KramaID,
) (randomPeers []identifiers.KramaID, err error) {
	_, span := tracing.Span(ctx, "Flux.Randomizer", "GetRandomNodes")
	defer span.End()

	requiredNo := count

	for {
		select {
		case <-ctx.Done():
			return nil, common.ErrTimeOut

		default:
			if requiredNo <= 0 {
				return randomPeers, err
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
				return randomPeers, err
			}

			avoidPeers = append(avoidPeers, randomPeers...)
			requiredNo -= len(peers)
		}
	}
}

func (r *Randomizer) Close() {
	r.ctxCancel()
	r.logger.Info("Closing Flux")
}
