package decision

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/db"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"sync"
	"time"
)

/*
Metrics to be collected

ActiveRequests
Timed out requests
Request process time
Avg cid count per request

*/
const (
	MaxQueueSize = 1000
)

type ledger interface {
	GetAssociatedPeers(addr ktypes.Address, stateHash ktypes.Hash) ([]id.KramaID, error)
	UpdateAssociatedPeers(addr ktypes.Address, stateHash ktypes.Hash, peers id.KramaID) error
}

type store interface {
	GetData(ctx context.Context, keys []ktypes.Hash) ([][]byte, error)
	DoesStateExists(stateHash ktypes.Hash) bool
	Get(key []byte) ([]byte, error)
	GetBatchWriter() db.BatchWriter
}

type network interface {
	SendAgoraMessage(id id.KramaID, msgType ktypes.MsgType, msg types.Message) error
}

type Engine struct {
	ctx                 context.Context
	logger              hclog.Logger
	requests            *RequestQueue
	workerLock          sync.Mutex
	requestWorkerCount  int
	responseWorkerCount int
	responses           chan *types.Response
	workSignal          chan struct{}
	db                  store
	ledger              ledger
	network             network
}

func NewEngine(
	ctx context.Context,
	logger hclog.Logger,
	requestWorkerCount,
	responseWorkerCount int,
	db store,
	ledger ledger,
	network network,
) *Engine {
	e := &Engine{
		ctx:                 ctx,
		logger:              logger.Named("Engine"),
		requests:            NewRequestQueue(MaxQueueSize),
		requestWorkerCount:  requestWorkerCount,
		responseWorkerCount: responseWorkerCount,
		responses:           make(chan *types.Response),
		workSignal:          make(chan struct{}),
		db:                  db,
		ledger:              ledger,
		network:             network,
	}

	return e
}

func (e *Engine) Start() {
	e.workerLock.Lock()
	defer e.workerLock.Unlock()

	for i := 0; i < e.requestWorkerCount; i++ {
		go e.worker()
	}

	for i := 0; i < e.responseWorkerCount; i++ {
		go e.responseWorker()
	}
}

func (e *Engine) nextTask() (*types.Response, error) {
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

			continue
		}

		ids := req.WantList

		if len(ids) == 0 {
			ids = append(ids, req.StateHash)
		}

		blocks, err := e.db.GetData(context.Background(), ids)
		if err != nil {
			e.logger.Error("Error fetching blocks from db", "error", err)

			continue
		}

		resp := &types.Response{
			PeerID:    req.PeerID,
			SessionID: req.SessionID,
			StateHash: req.StateHash,
			Status:    true,
			HaveList:  types.NewHaveList(),
			PeerSet:   nil,
		}

		for _, v := range blocks {
			resp.HaveList.AddBlock(types.NewBlock(v))
		}

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
	stateHash := req.StateHash
	address := req.SessionID

	if !e.db.DoesStateExists(stateHash) {
		e.sendResponse(req.PeerID, address, stateHash, false, nil)
	}

	if !e.requests.Contains(req.PeerID) {
		if err := e.requests.Push(req); err == nil {
			e.workSignal <- struct{}{}

			return
		}
	}

	peerSet, err := e.ledger.GetAssociatedPeers(req.SessionID, req.StateHash)
	if err != nil {
		e.logger.Error("Error fetching associated peers")
	}

	e.sendResponse(req.PeerID, address, stateHash, false, peerSet)
}

func (e *Engine) sendResponse(
	id id.KramaID,
	sessionID ktypes.Address,
	stateHash ktypes.Hash,
	responseStatus bool,
	peerList []id.KramaID,
) {
	select {
	case e.responses <- &types.Response{
		PeerID:    id,
		StateHash: stateHash,
		SessionID: sessionID,
		Status:    responseStatus,
		PeerSet:   peerList,
	}:
	default:
		go func() {
			e.responses <- &types.Response{
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

			if err := e.network.SendAgoraMessage(resp.PeerID, ktypes.AGORARESP, resp.GetAgoraMsg()); err != nil {
				e.logger.Error("Error sending response message", "peer", resp.PeerID)

				continue
			}

			if err := e.ledger.UpdateAssociatedPeers(resp.SessionID, resp.StateHash, resp.PeerID); err != nil {
				e.logger.Error("Error updating associated peers info", "error", err)
			}
		}
	}
}
