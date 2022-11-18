package decision

import (
	"context"
	"sync"
	"time"

	ptypes "gitlab.com/sarvalabs/moichain/poorna/types"

	"github.com/hashicorp/go-hclog"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/db"
	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/moichain/types"
)

const (
	MaxQueueSize = 1000
)

type ledger interface {
	GetAssociatedPeers(addr types.Address, stateHash types.Hash) ([]id.KramaID, error)
	UpdateAssociatedPeers(addr types.Address, stateHash types.Hash, peers id.KramaID) error
}

type store interface {
	GetData(ctx context.Context, keys []types.Hash) ([][]byte, error)
	DoesStateExists(stateHash types.Hash) bool
	Get(key []byte) ([]byte, error)
	GetBatchWriter() db.BatchWriter
}

type network interface {
	SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error
}

type Engine struct {
	ctx                 context.Context
	logger              hclog.Logger
	requests            *RequestQueue
	workerLock          sync.Mutex
	requestWorkerCount  int
	responseWorkerCount int
	responses           chan *atypes.Response
	workSignal          chan struct{}
	db                  store
	ledger              ledger
	network             network
	metrics             *Metrics
}

func NewEngine(
	ctx context.Context,
	logger hclog.Logger,
	requestWorkerCount,
	responseWorkerCount int,
	db store,
	ledger ledger,
	network network,
	metrics *Metrics,
) *Engine {
	e := &Engine{
		ctx:                 ctx,
		logger:              logger.Named("Engine"),
		requests:            NewRequestQueue(MaxQueueSize),
		requestWorkerCount:  requestWorkerCount,
		responseWorkerCount: responseWorkerCount,
		responses:           make(chan *atypes.Response),
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

func (e *Engine) nextTask() (*atypes.Response, error) {
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

		blocks, err := e.db.GetData(context.Background(), ids)
		if err != nil {
			e.logger.Error("Error fetching blocks from db", "error", err)

			continue
		}

		resp := &atypes.Response{
			PeerID:    req.PeerID,
			SessionID: req.SessionID,
			StateHash: req.StateHash,
			Status:    true,
			HaveList:  atypes.NewHaveList(),
			PeerSet:   nil,
		}

		for _, v := range blocks {
			resp.HaveList.AddBlock(atypes.NewBlock(v))
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

		if !e.db.DoesStateExists(stateHash) {
			e.sendResponse(req.PeerID, address, stateHash, false, nil)
			e.metrics.captureRejectedRequests(1)

			return
		}

		if !e.requests.Contains(req.PeerID) {
			if err := e.requests.Push(req); err == nil {
				e.metrics.capturePendingRequests(1)
				e.workSignal <- struct{}{}

				return
			}
		}

		peerSet, err := e.ledger.GetAssociatedPeers(req.SessionID, req.StateHash)
		if err != nil {
			e.logger.Error("Error fetching associated peers")
		}

		e.sendResponse(req.PeerID, address, stateHash, false, peerSet)
		e.metrics.captureRejectedRequests(1)
	}
}

func (e *Engine) sendResponse(
	id id.KramaID,
	sessionID types.Address,
	stateHash types.Hash,
	responseStatus bool,
	peerList []id.KramaID,
) {
	select {
	case e.responses <- &atypes.Response{
		PeerID:    id,
		StateHash: stateHash,
		SessionID: sessionID,
		Status:    responseStatus,
		PeerSet:   peerList,
	}:
	default:
		go func() {
			e.responses <- &atypes.Response{
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
			e.logger.Info("Context expired closing response worker")

			return
		case resp, ok := <-e.responses:
			if !ok {
				return
			}

			if err := e.network.SendAgoraMessage(resp.PeerID, ptypes.AGORARESP, resp.GetAgoraMsg()); err != nil {
				e.logger.Error("Error sending response message", "peer", resp.PeerID)

				continue
			}

			if err := e.ledger.UpdateAssociatedPeers(resp.SessionID, resp.StateHash, resp.PeerID); err != nil {
				e.logger.Error("Error updating associated peers info", "error", err)
			}
		}
	}
}
