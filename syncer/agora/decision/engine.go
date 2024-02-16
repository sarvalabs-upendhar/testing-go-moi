package decision

import (
	"context"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/storage"

	"github.com/hashicorp/go-hclog"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	message2 "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/agora/db"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

const (
	MaxQueueSize = 1000
)

type ledger interface {
	GetAssociatedPeers(addr identifiers.Address, stateHash cid.CID) ([]kramaid.KramaID, error)
	UpdateAssociatedPeers(addr identifiers.Address, stateHash cid.CID, peers kramaid.KramaID) error
}

type store interface {
	GetData(ctx context.Context, address identifiers.Address, keys []cid.CID) (map[cid.CID][]byte, error)
	DoesStateExists(address identifiers.Address, stateHash cid.CID) bool
	GetBatchWriter() db.BatchWriter
}

type net interface {
	SendAgoraMessage(id kramaid.KramaID, msgType message2.MsgType, msg message.Message) error
}

type Engine struct {
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	logger              hclog.Logger
	requests            *RequestQueue
	workerLock          sync.Mutex
	requestWorkerCount  int
	responseWorkerCount int
	responses           chan *message.Response
	workSignal          chan struct{}
	db                  store
	ledger              ledger
	network             net
	metrics             *Metrics
}

func NewEngine(
	logger hclog.Logger,
	requestWorkerCount,
	responseWorkerCount int,
	db store,
	ledger ledger,
	network net,
	metrics *Metrics,
	requestQueueSize int,
) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		ctx:                 ctx,
		ctxCancel:           cancel,
		logger:              logger.Named("Engine"),
		requests:            NewRequestQueue(requestQueueSize),
		requestWorkerCount:  requestWorkerCount,
		responseWorkerCount: responseWorkerCount,
		responses:           make(chan *message.Response),
		workSignal:          make(chan struct{}),
		db:                  db,
		ledger:              ledger,
		network:             network,
		metrics:             metrics,
	}

	return e
}

func (e *Engine) Start() {
	e.metrics.initMetrics()

	e.workerLock.Lock()
	defer e.workerLock.Unlock()

	for i := 0; i < e.requestWorkerCount; i++ {
		go e.worker()
	}

	for i := 0; i < e.responseWorkerCount; i++ {
		go e.responseWorker()
	}
}

func (e *Engine) nextTask() (*message.Response, error) {
	for {
		req := e.requests.Pop()
		for req == nil {
			select {
			case <-e.ctx.Done():
				return nil, e.ctx.Err()
			case <-e.workSignal:
				req = e.requests.Pop()
			}
		}

		if time.Since(req.ReqTime) > 1000*time.Millisecond {
			e.logger.Info("Skipping request")
			e.metrics.captureTimedOutRequests(1)

			continue
		}

		ids := req.WantList
		numOfIds := len(ids)

		e.metrics.capturePendingRequests(-1)
		e.metrics.captureCidsPerRequest(float64(numOfIds))

		if len(ids) == 0 {
			ids = append(ids, req.StateHash)
		}

		blocks, err := e.db.GetData(context.Background(), req.SessionID, ids)
		if err != nil {
			e.logger.Error("Error fetching blocks from DB", "err", err)

			continue
		}

		resp := &message.Response{
			PeerID:    req.PeerID,
			SessionID: req.SessionID,
			StateHash: req.StateHash,
			Status:    true,
			HaveList:  block.NewHaveList(),
			PeerSet:   nil,
		}

		for cID, v := range blocks {
			if storage.PrefixTag(cID.ContentType()).IsAccountBasedKey() {
				resp.HaveList.AddBlock(block.NewAccountBlockFromRawData(cID.ContentType(), v))
			} else {
				resp.HaveList.AddBlock(block.NewNonAccountBlockFromRawData(cID, v))
			}
		}

		e.metrics.captureRequestProcessTime(req.ReqTime)

		return resp, nil
	}
}

func (e *Engine) worker() {
	defer func() {
		e.workerLock.Lock()
		defer e.workerLock.Unlock()
		e.requestWorkerCount--
	}()

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
			resp, err := e.nextTask()
			if err != nil {
				return // ctx cancelled
			}
			e.responses <- resp
		}
	}
}

func (e *Engine) HandleRequest(req *Request) {
	if req != nil {
		stateHash := req.StateHash
		address := req.SessionID

		if !e.db.DoesStateExists(req.SessionID, stateHash) {
			e.sendResponse(req.PeerID, address, stateHash, false, nil)
			e.metrics.captureRejectedRequests(1)

			return
		}

		if !e.requests.Contains(req.PeerID) {
			if err := e.requests.Push(req); err == nil {
				e.metrics.capturePendingRequests(1)
				e.signalNewWork()

				return
			}
		}

		peerSet, err := e.ledger.GetAssociatedPeers(req.SessionID, req.StateHash)
		if err != nil {
			e.logger.Error("Error fetching associated peers", "err", err)
		}

		e.sendResponse(req.PeerID, address, stateHash, false, peerSet)
		e.metrics.captureRejectedRequests(1)
	}
}

func (e *Engine) sendResponse(
	id kramaid.KramaID,
	sessionID identifiers.Address,
	stateHash cid.CID,
	responseStatus bool,
	peerList []kramaid.KramaID,
) {
	select {
	case e.responses <- &message.Response{
		PeerID:    id,
		StateHash: stateHash,
		SessionID: sessionID,
		Status:    responseStatus,
		PeerSet:   peerList,
	}:
	default:
		go func() {
			e.responses <- &message.Response{
				PeerID:    id,
				StateHash: stateHash,
				SessionID: sessionID,
				Status:    responseStatus,
				PeerSet:   peerList,
			}
		}()
	}
}

func (e *Engine) responseWorker() {
	defer func() {
		e.workerLock.Lock()
		defer e.workerLock.Unlock()
		e.responseWorkerCount--
	}()

	for {
		select {
		case <-e.ctx.Done():
			e.logger.Trace("Context expired closing response worker")

			return
		case resp, ok := <-e.responses:
			if !ok {
				return
			}

			if err := e.network.SendAgoraMessage(resp.PeerID, message2.AGORARESP, resp.GetAgoraMsg()); err != nil {
				e.logger.Error("Error sending response message", "peer-ID", resp.PeerID)

				continue
			}

			if err := e.ledger.UpdateAssociatedPeers(resp.SessionID, resp.StateHash, resp.PeerID); err != nil {
				e.logger.Error("Error updating associated peers information", "err", err)
			}
		}
	}
}

func (e *Engine) signalNewWork() {
	select {
	case e.workSignal <- struct{}{}:
	default:
	}
}

func (e *Engine) Close() {
	e.logger.Info("Closing Agora-Engine")
	e.ctxCancel()
}
