package syncer

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-polo"

	atypes "github.com/sarvalabs/moichain/poorna/agora/types"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/guna"

	"github.com/sarvalabs/moichain/utils"

	db "github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/poorna"
	"github.com/sarvalabs/moichain/poorna/agora"
	"github.com/sarvalabs/moichain/poorna/agora/session"

	"github.com/sarvalabs/moichain/poorna/moirpc"

	"github.com/hashicorp/go-hclog"
	id "github.com/sarvalabs/moichain/mudra/kramaid"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/moichain/types"
)

const (
	SyncRPCProtocol    = protocol.ID("moi/rpc/sync")
	SyncStreamProtocol = protocol.ID("moi/stream/sync")
)

const (
	slotSize   int   = 40
	bucketSize int32 = 1024
)

const syncerMoirpcStreamTTL = time.Duration(10) * time.Minute

type Response struct {
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data"`
}

type SyncJob struct {
	address types.Address
	hash    types.Hash
	peer    *SyncPeer
	mode    string
}
type TesseractSyncJob struct {
	tesseract    *types.Tesseract
	fetchContext []id.KramaID
}

type lattice interface {
	AddTesseractWithOutState(
		ts *types.Tesseract,
		sender id.KramaID,
		ics *ptypes.ICSClusterInfo,
	) error
	GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
}

type store interface {
	NewBatchWriter() db.BatchWriter
	CreateEntry([]byte, []byte) error
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	SetAccount(addr types.Address, hash types.Hash, data []byte) error
	GetAccountMetaInfo(id []byte) (*types.AccountMetaInfo, error)
	GetAccounts(bucketID int32) (types.Accounts, error)
	GetBucketSizes() (map[int32]*big.Int, error)
	UpdateTesseractStatus(addr types.Address, height uint64, hash types.Hash, status bool) error
}

type SyncPeer struct {
	id     peer.ID
	status *Status
	rw     *bufio.ReadWriter
	con    network.Conn
}

type Status struct {
	Accounts    *big.Int // potential memory leak
	bucketSizes sync.Map
	Ntq         float32
}
type Syncer struct {
	ctx              context.Context
	node             *poorna.Server
	mux              *utils.TypeMux
	status           *Status
	peers            sync.Map
	peerCount        uint32
	agora            *agora.Agora
	db               store
	reqQueue         chan *SyncJob
	reqQueue1        chan *TesseractSyncJob
	wrkResults       chan interface{}
	accDetails       *AccDetailsQueue
	tesseractSub     *utils.Subscription
	statusSub        *utils.Subscription
	newpeerSub       *utils.Subscription
	mode             string
	rpcClient        *moirpc.Client
	lattice          lattice
	logger           hclog.Logger
	ReputationEngine *guna.ReputationEngine
	ntqtablesynconce sync.Once
}

func NewSyncer(
	ctx context.Context,
	node *poorna.Server,
	mux *utils.TypeMux,
	db store,
	mode string,
	lattice lattice,
	logger hclog.Logger,
	metrics *agora.Metrics,
) (*Syncer, error) {
	agoraInstance, err := agora.NewAgora(ctx, logger, db, node, metrics)
	if err != nil {
		return nil, errors.Wrap(err, "error initiating agora")
	}

	s := &Syncer{
		ctx:        ctx,
		node:       node,
		mux:        mux,
		agora:      agoraInstance,
		db:         db,
		mode:       mode,
		lattice:    lattice,
		peerCount:  0,
		logger:     logger.Named("Syncer"),
		reqQueue:   make(chan *SyncJob),
		reqQueue1:  make(chan *TesseractSyncJob),
		wrkResults: make(chan interface{}),
		accDetails: new(AccDetailsQueue),
		status: &Status{
			Accounts: big.NewInt(0),
		},
	}

	if err := s.RegisterRPCService(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Syncer) RegisterRPCService() error {
	s.rpcClient = s.node.InitNewRPCServer(SyncRPCProtocol)

	return s.node.RegisterNewRPCService(SyncRPCProtocol, "SYNCRPC", NewSyncRPCService(s))
}

func (sy *Status) updateBucketCount(id int32, v *big.Int) bool {
	if currentValue, ok := sy.bucketSizes.Load(id); ok {
		if currentValue, ok := currentValue.(*big.Int); ok {
			if v.Cmp(currentValue) <= 0 {
				return false
			}
		}
	}

	sy.bucketSizes.Store(id, v)

	return true
}

func (sy *Status) incrementBucketCount(id int32, v int64) {
	currentSize, ok := sy.bucketSizes.Load(id)
	if !ok {
		sy.bucketSizes.Store(id, big.NewInt(v))
	} else {
		if currentSize, ok := currentSize.(*big.Int); ok {
			sy.bucketSizes.Store(id, new(big.Int).Add(currentSize, big.NewInt(v)))
		}
	}
}

// StreamHandler handles the sync protocol streams
func (s *Syncer) StreamHandler(stream network.Stream) {
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	remotePeer := stream.Conn().RemotePeer()
	sp := &SyncPeer{
		id: remotePeer,
		rw: rw,
		status: &Status{
			Accounts: big.NewInt(0),
			Ntq:      1,
		},
		con: stream.Conn(),
	}

	if _, ok := s.peers.Load(remotePeer); !ok {
		s.peers.Store(remotePeer, sp)
		atomic.AddUint32(&s.peerCount, 1)

		go s.handleSyncPeer(sp)

		s.logger.Info("[StreamHandler]", "Current Peer Count", atomic.LoadUint32(&s.peerCount))
	}

	msg := &ptypes.AccountsStatusMsg{
		TotalAccounts: s.status.Accounts.Bytes(), // FIXME: Race at status
		BucketSizes:   make(map[int32][]byte),
		NTQ:           s.status.Ntq,
	}

	dbBuckets, err := s.db.GetBucketSizes()
	if err != nil {
		log.Panicln(err)
	}

	for k, v := range dbBuckets {
		msg.BucketSizes[k] = v.Bytes()
	}

	kipPeer := s.node.Peers.Peer(remotePeer)
	peerKramaID := kipPeer.GetKramaID()
	resp := new(Response)

	if err := s.rpcClient.MoiCall(peerKramaID,
		"SYNCRPC",
		"StatusUpdate",
		msg,
		resp,
		syncerMoirpcStreamTTL); err != nil {
		log.Println("RPC Call panic", err)

		return
	}
}

// sendAccSyncRequest sends an account sync request to the remote peer
func (s *Syncer) sendAccSyncRequest(peer *SyncPeer) error { //nolint
	msg := &ptypes.AccountSyncRequest{
		BulkSync: true,
	}

	log.Println("Sending account sync request to peer:", peer.id)

	return peer.Send(s.node.GetKramaID(), ptypes.ACCSYNCREQ, msg)
}

// StatusUpdate is an rpc handler method used to update the status of a sync peer
func (s *Syncer) StatusUpdate(peerID peer.ID, msg *ptypes.AccountsStatusMsg) error {
	syncPeer, ok := s.peers.Load(peerID)
	if !ok {
		return errors.New("peer Not Found")
	}

	accountCount := new(big.Int).SetBytes(msg.TotalAccounts)
	if accountCount.Cmp(s.status.Accounts) <= 0 {
		return nil
	}
	// TODO: leaving NTQ
	for k, v := range msg.BucketSizes {
		newSize := new(big.Int).SetBytes(v)

		syncPeer, ok := syncPeer.(*SyncPeer)
		if !ok {
			s.logger.Error("Error type assertion failed ")
		}

		updated := syncPeer.status.updateBucketCount(k, newSize)
		if updated {
			syncPeer.status.Accounts = new(big.Int).Add(syncPeer.status.Accounts, newSize)
		}

		log.Println("Received bucketID", k, "Count", v, updated)
	}

	return nil
}

// syncBucket will send the address in the given bucket to the requested peer
func (s *Syncer) syncBucket(bucket int32, peer *SyncPeer) error {
	accountMetaInfos, err := s.db.GetAccounts(bucket)
	if err != nil {
		return err
	}

	msgs := make([]*ptypes.AccountSyncResponse, 0)
	msg := new(ptypes.AccountSyncResponse)
	slot := int32(0)

	for len(accountMetaInfos) > slotSize {
		slot++

		msg.Accounts = accountMetaInfos[0:slotSize]
		msg.Bucket = bucket
		msg.Slot = slot
		msgs = append(msgs, msg)
		accountMetaInfos = accountMetaInfos[slotSize:]
	}

	slot++

	msg.Accounts = accountMetaInfos
	msg.Bucket = bucket
	msg.Slot = slot
	msgs = append(msgs, msg)

	for _, v := range msgs {
		if err := peer.Send(s.node.GetKramaID(), ptypes.ACCSYNCRRESP, v); err != nil {
			return err
		}
	}

	return nil
}

// accSync will send the complete address space to the requested peer by dividing buckets into slots
// and each message can have upto 40 slots
func (s *Syncer) accSync(peerID peer.ID) error {
	syncPeer, ok := s.peers.Load(peerID)
	if !ok {
		return errors.New("unable to find syncPeer")
	}

	for i := int32(0); i < bucketSize; i++ {
		syncPeer, ok := syncPeer.(*SyncPeer)
		if !ok {
			s.logger.Error("Error asserting syncPeer type")
		}

		if err := s.syncBucket(i, syncPeer); err != nil {
			s.logger.Error("Error syncing bucket", "bucket id", i)
		}
	}

	return nil
}

// Send is a method of KipPeer that emits an arbitrary proto message to the network
// Accepts the sender id, the message type and message itself.
func (p *SyncPeer) Send(id id.KramaID, code ptypes.MsgType, msg interface{}) error {
	var (
		rawData []byte
		err     error
	)

	if msg != nil {
		// Marshal the proto message into slice of bytes and return if an error occurs
		rawData, err = polo.Polorize(msg)
		if err != nil {
			return errors.Wrap(err, "failed to polorize message payload")
		}
	}
	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into into a slice of bytes
	m := ptypes.Message{
		MsgType: code,
		Payload: rawData,
		Sender:  id,
	}

	rawData, err = m.Bytes()
	if err != nil {
		return err
	}

	// Write the message bytes into the peer's iobuffer
	_, err = p.rw.Writer.Write(rawData)
	if err != nil {
		return err
	}
	// Flush the peer's iobuffer. This will push the message to the network
	return p.rw.Flush()
}

// handle sync peer handles the messages received from a syncPeer
func (s *Syncer) handleSyncPeer(p *SyncPeer) {
	defer func() {
		s.peers.Delete(p.id)
		atomic.AddUint32(&s.peerCount, ^uint32(0))
	}()

	for {
		buffer := make([]byte, 4096)

		bytecount, err := p.rw.Reader.Read(buffer)
		if err != nil {
			return
		}
		// TODO:Improve this
		if bytecount == 1 {
			continue
		}

		message := new(ptypes.Message)
		if err := message.FromBytes(buffer[0:bytecount]); err != nil {
			s.logger.Error("unmarshalling error", "error", err, "byte-count", bytecount)

			return
		}

		switch message.MsgType {
		case ptypes.NTQTABLESYNCREQ:

		case ptypes.NTQTABLESYNCRESP:
		case ptypes.ACCSYNCREQ:
			s.logger.Debug("Async message received from ", message.Sender)

			accSyncReq := new(ptypes.AccountSyncRequest)

			if err := accSyncReq.FromBytes(message.Payload); err != nil {
				s.logger.Error("Error depolarizing account sync request", "error", err)

				continue
			}

			if accSyncReq.BulkSync {
				go func() {
					if err := s.accSync(p.id); err != nil {
						s.logger.Error("Error syncing address space", "error", err)
					}
				}()
			} else {
				go func() {
					if err := s.syncBucket(accSyncReq.Bucket, p); err != nil {
						s.logger.Error("Error syncing the bucket", "error", err)
					}
				}()
			}

		case ptypes.ACCSYNCRRESP:
			accSyncRes := new(ptypes.AccountSyncResponse)

			if err := accSyncRes.FromBytes(message.Payload); err != nil {
				s.logger.Error("Error depolarising AccountSycResp message", "error", err)

				continue
			}

			s.logger.Debug("Address space messaged received from peer", message.Sender)

			s.accDetails.Push(accSyncRes.Accounts)
		}
	}
}

/*
// syncLattice will fetch the latest info of an account from the bestPeer and creates a lattice sync job
func (s *Syncer) syncLattice(addr []byte, mode string) error {
	accDetails, err := s.store.GetAccountDetails(addr)
	bestPeer := s.BestPeer()
	if err != nil && grpcStatus.Code(err) != codes.NotFound {
		return err
	} else {
		//Make an rpc call to the peers to get acc details
		req := &netprotos.ACCSYNCMSG{
			BulkSync: false,
			Address:  addr,
		}
		resp := new(netprotos.AccountDetails)
		if err := s.rpcClient.Call(bestPeer.id, "SYNCRPC", "SYNCACCINFO", req, resp); err != nil {
			return err
		}
		acc := types.ProtoToAccDetails(resp)
		bucketNo, count := s.store.AddAddress(types.Accounts{acc})
		s.status.incrementBucketCount(bucketNo, count)
		s.status.Accounts = new(big.Int).Add(s.status.Accounts, big.NewInt(count))
		accDetails = &acc
	}

	s.reqQueue <- &SyncJob{
		address: accDetails.id,
		peer:    bestPeer,
	}

	return nil
}
*/

// fetchData fetches the complete state information associated with the given state CID
// this uses agora to fetch the hashes.
func (s *Syncer) fetchData(ctx context.Context, session *session.Session, ids ...atypes.CID) error {
	keySet := atypes.NewHashSet()

	for _, cid := range ids {
		if !cid.IsNil() {
			if ok, err := s.db.Contains(dbKeyFromCID(session.ID(), cid)); !ok && err == nil {
				keySet.Add(cid)
			}
		}
	}

	if keySet.Len() <= 0 {
		s.logger.Debug("Returning from get blocks : keySet is empty")

		return nil
	}

	var (
		receivedBlocksCount = 0
		returnErr           error
	)

	blocksChan := session.GetBlocks(ctx, keySet.Keys())

	for block := range blocksChan {
		if err := s.db.CreateEntry(dbKeyFromCID(session.ID(), block.GetCid()), block.GetData()); err != nil {
			s.logger.Error("Error writing to db", "error", err)

			returnErr = err

			continue
		}

		receivedBlocksCount++
	}

	if receivedBlocksCount == keySet.Len() {
		return nil
	}

	return returnErr
}

// getContextData fetches the behavioural context and random context associated with the given hash using agora
func (s *Syncer) getContextData(ctx context.Context, session *session.Session, cid atypes.CID) error {
	ok, err := s.db.Contains(dbKeyFromCID(session.ID(), cid))
	if ok || err != nil {
		return err
	}

	block, err := session.GetBlock(ctx, cid)
	if err != nil {
		return err
	}

	metaContextObject := new(gtypes.MetaContextObject)
	if err := metaContextObject.FromBytes(block.GetData()); err != nil {
		return err
	}

	if err = s.fetchData(
		ctx,
		session,
		contextCID(metaContextObject.RandomContext),
		contextCID(metaContextObject.BehaviouralContext),
	); err != nil {
		return err
	}

	if err := s.db.CreateEntry(dbKeyFromCID(session.ID(), cid), block.GetData()); err != nil {
		return err
	}

	return nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using bitswap
func (s *Syncer) fetchTesseractState(tesseract *types.Tesseract, fetchContext []id.KramaID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // TODO: Timeout duration to be optimized
	defer cancel()

	newSession, err := s.agora.NewSession(ctx, fetchContext, tesseract.Address(), accountCID(tesseract.StateHash()))
	if err != nil {
		return err
	}

	block, err := newSession.GetBlock(ctx, accountCID(tesseract.StateHash()))
	if err != nil {
		return err
	}
	defer newSession.Close()

	acc := new(types.Account)
	if err := acc.FromBytes(block.GetData()); err != nil {
		return err
	}

	if err := s.fetchData(
		ctx,
		newSession,
		balanceCID(acc.Balance),
		//	acc.StorageRoot,
		approvalsCID(acc.AssetApprovals),
		// tesseract.Body.ReceiptHash,
	); err != nil {
		s.logger.Error("Error fetching balance data", "error", err)

		return err
	}

	if err = s.getContextData(ctx, newSession, contextCID(acc.ContextHash)); err != nil {
		s.logger.Error("Error fetching context data", "error", err)

		return err
	}

	if err = s.db.SetAccount(tesseract.Address(), tesseract.StateHash(), block.GetData()); err != nil {
		return err
	}

	return nil
}

// is SyncRequired checks if sync is required based on the Account count
func (s *Syncer) isSyncRequired(bestPeer *SyncPeer) bool {
	var isRequired bool

	bestPeer.status.bucketSizes.Range(func(key, value interface{}) bool {
		cValue, ok := s.status.bucketSizes.Load(key)
		if !ok {
			isRequired = true

			return false
		}

		log.Printf("bucketID %d ----->> count %d", key, cValue)

		if peerCount, ok := value.(*big.Int); ok {
			if selfCount, ok := cValue.(*big.Int); ok {
				if float64(selfCount.Uint64()) <= 0.6*float64(peerCount.Uint64()) {
					isRequired = true

					return false
				}
			}
		}

		return true
	})

	return isRequired
}

/*
tesseractWorker handles the tesseract Sync Jobs, it fetches the tesseract state using agora
and updates the state in the db accordingly
*/
func (s *Syncer) tesseractWorker(id int, reqQueue chan *TesseractSyncJob) {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Closing tesseract worker", id)

			return
		case job, ok := <-reqQueue:
			if !ok {
				return
			}

			// TODO:Check whether tesseract data exsist

			ts := job.tesseract

			s.logger.Debug(
				"Got a new tesseract info sync JOB",
				"address",
				job.tesseract.Header.Address,
				"height",
				job.tesseract.Header.Height,
			)

			if err := s.fetchTesseractState(ts, job.fetchContext); err != nil {
				s.logger.Error("Error fetching tesseract state", "err", err)

				continue
			} else {
				tsHash, err := ts.Hash()
				if err != nil {
					s.logger.Error("Error creating tesseract hash", "err", err)

					continue
				}

				if err = s.db.UpdateTesseractStatus(
					ts.Address(),
					ts.Height(),
					tsHash,
					true,
				); err != nil {
					s.logger.Error("Error updating the lattice status")

					continue
				}
			}
		default:
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// startWorkers will start latticeWorkers and tesseractWorkers 5 each, this can be configured down the line..
func (s *Syncer) startWorkers() {
	for i := 0; i < 5; i++ {
		go s.latticeWorker(i, s.reqQueue)
		go s.tesseractWorker(i, s.reqQueue1)
	}
}

// handleSyncEvents handles the TesseractSyncJob created by the lattice manager
func (s *Syncer) handleSyncEvents() {
	for obj := range s.tesseractSub.Chan() {
		if t, ok := obj.Data.(utils.TesseractSyncEvent); ok {
			go func() {
				s.reqQueue1 <- &TesseractSyncJob{tesseract: t.Tesseract, fetchContext: t.Context}
			}()
		}
	}
}

// handleStatusEvents handles the status update created by the lattice manager
func (s *Syncer) handleStatusEvents() {
	for obj := range s.statusSub.Chan() {
		if t, ok := obj.Data.(utils.SyncStatusUpdate); ok {
			s.status.incrementBucketCount(t.BucketID, t.Count)
			s.status.Accounts = new(big.Int).Add(s.status.Accounts, big.NewInt(t.Count))
			x, y := s.status.bucketSizes.Load(t.BucketID)
			log.Println("after updating the status", "Bucket No", x, "Count", y)
		}
	}
}

// handleNewPeer opens a new stream for the sync sub protocol and makes status update call to the newly discovered peer
func (s *Syncer) handleNewPeer() {
	// Read events from a newpeer channel
	for obj := range s.newpeerSub.Chan() {
		if p, ok := obj.Data.(utils.PeerDiscoveredEvent); ok {
			fmt.Println("Identified new peer sending sync request", p.ID)

			stream, err := s.node.NewStream(context.Background(), SyncStreamProtocol, p.ID)
			if err != nil {
				s.logger.Error("Error opening sync stream", "error", err)

				continue
			}

			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
			if err = rw.WriteByte(1); err != nil {
				s.logger.Error("Error writing the message", "error", err)

				continue
			}

			if err := rw.Flush(); err != nil {
				s.logger.Error("Error writing the message", "error", err)

				continue
			}

			sp := &SyncPeer{
				id: p.ID,
				rw: rw,
				status: &Status{
					Accounts: big.NewInt(0),
					Ntq:      1,
				},
				con: stream.Conn(),
			}

			s.peers.Store(p.ID, sp)

			go s.handleSyncPeer(sp)

			atomic.AddUint32(&s.peerCount, 1)

			r := rand.Intn(500)
			time.Sleep(time.Duration(r) * time.Millisecond)

			msg := &ptypes.AccountsStatusMsg{
				TotalAccounts: s.status.Accounts.Bytes(),
				BucketSizes:   make(map[int32][]byte),
				NTQ:           s.status.Ntq,
			}

			dbBuckets, err := s.db.GetBucketSizes()
			if err != nil {
				log.Panicln(err)
			}

			for k, v := range dbBuckets {
				msg.BucketSizes[k] = v.Bytes()
			}

			resp := new(Response)
			kipPeer := s.node.Peers.Peer(p.ID)
			peerKramaID := kipPeer.GetKramaID()

			if err := s.rpcClient.MoiCall(peerKramaID,
				"SYNCRPC",
				"StatusUpdate",
				msg, resp,
				time.Duration(10)*time.Minute); err != nil {
				log.Println("RPC Call panic", err)
			}
		}
	}
}

// Start  starts all event handlers and workers associated with sync sub protocol
func (s *Syncer) Start() {
	s.agora.Start()

	fmt.Println("Starting the syncer")
	s.node.SetupStreamHandler(SyncStreamProtocol, s.StreamHandler)
	s.tesseractSub = s.mux.Subscribe(utils.TesseractSyncEvent{})
	s.statusSub = s.mux.Subscribe(utils.SyncStatusUpdate{})
	s.newpeerSub = s.mux.Subscribe(utils.PeerDiscoveredEvent{})

	defer func() {
		s.tesseractSub.Unsubscribe()
		s.statusSub.Unsubscribe()
		s.newpeerSub.Unsubscribe()
		s.node.RemoveStreamHandler(SyncStreamProtocol)
	}()

	go s.handleStatusEvents()
	go s.handleSyncEvents()
	go s.handleNewPeer()
	go s.startWorkers()
	// update the status from store
	if err := s.statusInit(); err != nil {
		log.Panicln(err)
	}

	stop := true
	for stop {
		// var bestPeer *SyncPeer
		if atomic.LoadUint32(&s.peerCount) == poorna.MinimumPeerCount {
			bestPeer := s.BestPeer()

			time.Sleep(1 * time.Second)

			s.ntqtablesynconce.Do(func() {
				if err := bestPeer.Send(s.node.GetKramaID(), ptypes.NTQTABLESYNCREQ, nil); err != nil {
					s.logger.Error("Error sending NTQ sync request", "error", err)
				}
			})

			if bestPeer.status.Accounts.Cmp(s.status.Accounts) > 0 {
				// This following logic will be replaced with sendAccsyncReq
				for k, v := range bestPeer.con.GetStreams() {
					log.Println(k, v.Protocol(), v.ID(), bestPeer.id, bestPeer.id)

					if v.Protocol() == SyncStreamProtocol {
						msg := &ptypes.AccountSyncRequest{
							BulkSync: true,
						}

						rawData, err := msg.Bytes()
						if err != nil {
							log.Panic(errors.Wrap(err, "failed to polorize message payload"))
						}

						finalMsg := ptypes.Message{
							MsgType: ptypes.ACCSYNCREQ,
							Sender:  s.node.GetKramaID(),
							Payload: rawData,
						}

						rawData, err = finalMsg.Bytes()
						if err != nil {
							log.Panic(err)
						}

						rw := bufio.NewReadWriter(bufio.NewReader(v), bufio.NewWriter(v))

						_, err = rw.Writer.Write(rawData)
						if err != nil {
							log.Panic(err)
						}

						if err = rw.Flush(); err != nil {
							log.Panic(err)
						}
					}
				}

				stop = false
			}
		}
	}

	go func() {
		for {
			select {
			case <-time.After(60 * time.Second):
			case <-s.ctx.Done():
				return
			}

			bestPeer := s.BestPeer()
			if s.isSyncRequired(bestPeer) {
				for i := 0; i < s.accDetails.Len(); i++ {
					accInfo, err := s.accDetails.Pop()
					if err != nil {
						log.Panic(err)
					}

					log.Printf("Syncing lattice of account %s \n", accInfo.Address.Hex())

					s.reqQueue <- &SyncJob{
						address: accInfo.Address,
						peer:    bestPeer,
						hash:    accInfo.TesseractHash,
						mode:    s.mode,
					}
				}
			} else {
				log.Println("sync not required ")
			}
		}
	}()
}

// latticeWorker recursively sync the complete tesseract lattice from the best peer using RPC
func (s *Syncer) latticeWorker(id int, job <-chan *SyncJob) {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Closing lattice worker")

			return
		case job, ok := <-job:
			if !ok {
				return
			}

			if job.mode == "full" {
				exists, err := s.db.Contains(job.hash.Bytes())
				if err != nil {
					log.Fatal(err)
				}

				hash := job.hash
				tesseractStack := new(types.TesseractStack)

				for !exists {
					log.Println(" Lattice Sync Job Received of address ", job.address.Hex(), "Hash", hash)

					t, delta, err := s.getTesseract(job.peer, hash, nil, true)
					if err != nil {
						s.logger.Error("Unable to fetch tesseract", "hash", hash, "from", job.peer.id)

						return
					} else {
						tesseractStack.Push(&types.Item{Tesseract: t, Delta: delta, Sender: "job.peer"}) // FIXME:
					}

					if !t.Header.PrevHash.IsNil() {
						exists, err = s.db.Contains(t.Header.PrevHash.Bytes())
						if err != nil {
							s.logger.Error("Unable to fetch previous tesseract", "hash", hash)

							return
						}

						hash = t.Header.PrevHash
					} else {
						exists = true
					}
				}

				stackSize := tesseractStack.Len()
				for i := 0; i < int(stackSize); i++ {
					item := tesseractStack.Pop()

					tsHash, err := item.Tesseract.Hash()
					if err != nil {
						log.Fatal("Error creating tesseract hash", err)
					}

					log.Printf("Adding %s tesseract to lattice %v", item.Tesseract.Header.Address.Hex(), tsHash)

					icsClusterInfo := new(ptypes.ICSClusterInfo)
					if err := icsClusterInfo.FromBytes(
						item.Delta[item.Tesseract.Body.ConsensusProof.ICSHash],
					); err != nil {
						s.logger.Error("Error depolarising ics cluster Info", "err", err)

						continue
					}

					if err := s.lattice.AddTesseractWithOutState(
						item.Tesseract,
						item.Sender,
						icsClusterInfo,
					); err != nil {
						log.Fatal("Unable to add tesseract", err)
					}
				}
			} else {
				if t, delta, err := s.getTesseract(job.peer, job.hash, nil, true); err != nil {
					log.Printf("Unable to get tesseract %s from %s Error %s", job.peer.id, job.hash, err)
				} else {
					icsClusterInfo := new(ptypes.ICSClusterInfo)
					if err := icsClusterInfo.FromBytes(delta[t.Body.ConsensusProof.ICSHash]); err != nil {
						s.logger.Error("Error depolarising ics cluster Info", "err", err)

						continue
					}

					// FIXME: Fix the peer id
					if err := s.lattice.AddTesseractWithOutState(t, "job.peer", icsClusterInfo); err != nil {
						log.Fatal(err)
					}
				}
			}
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// BestPeer selects the best peer among the available sync peers based on the account count
func (s *Syncer) BestPeer() *SyncPeer {
	var bestPeer *SyncPeer

	s.peers.Range(func(id, peer interface{}) bool {
		if syncPeer, ok := peer.(*SyncPeer); ok {
			if bestPeer == nil {
				bestPeer = syncPeer
			} else if syncPeer.status.Accounts.Cmp(bestPeer.status.Accounts) > 0 {
				bestPeer = syncPeer
			}
		}

		return true
	})

	return bestPeer
}

// statusInit fetches the current status of the node and updated the in memory status
func (s *Syncer) statusInit() error {
	buckets, err := s.db.GetBucketSizes()
	if err != nil {
		return err
	}

	for k, v := range buckets {
		s.status.bucketSizes.Store(k, v)
		s.status.Accounts = new(big.Int).Add(s.status.Accounts, v)
	}

	return nil
}

// getTesseract makes an RPC call to fetch the tesseract from best peer
func (s *Syncer) getTesseract(
	bestPeer *SyncPeer,
	hash types.Hash,
	number *big.Int,
	withInteractions bool,
) (*types.Tesseract, map[types.Hash][]byte, error) {
	req := new(ptypes.TesseractReq)

	log.Println("get tesseract call", hash)

	if !hash.IsNil() {
		req.Hash = hash
	}

	if number != nil {
		req.Number = number.Uint64()
	}

	req.WithInteractions = withInteractions

	resp := new(TesseractResponse)
	kipPeer := s.node.Peers.Peer(bestPeer.id)
	kramaPeerID := kipPeer.GetKramaID()

	if err := s.rpcClient.MoiCall(
		kramaPeerID,
		"SYNCRPC",
		"GetTesseract",
		req,
		resp,
		time.Duration(10)*time.Minute); err != nil {
		return nil, nil, err
	}

	msg := new(types.Tesseract)

	if err := msg.FromBytes(resp.Data); err != nil {
		return nil, nil, err
	}

	return msg, resp.Delta, nil
}

func (s *Syncer) GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	ts, err := s.lattice.GetTesseract(hash, withInteractions)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func (s *Syncer) Close() {
	log.Println("Closing syncer")
}
