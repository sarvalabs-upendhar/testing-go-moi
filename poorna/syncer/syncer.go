package syncer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"gitlab.com/sarvalabs/moichain/poorna"
	"gitlab.com/sarvalabs/moichain/poorna/senatus"
	"log"
	"math/big"
	"math/rand"

	"github.com/hashicorp/go-hclog"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"

	"github.com/ipfs/go-bitswap"
	blockstore "github.com/ipfs/go-ipfs-blockstore"

	"sync"
	"sync/atomic"
	"time"

	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	lrpc "github.com/libp2p/go-libp2p-gorpc"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

const (
	SyncRPCProtocol    = protocol.ID("moi/rpc/sync")
	SyncStreamProtocol = protocol.ID("moi/stream/sync")
)

const (
	slotSize   int   = 40
	bucketSize int32 = 1024
)

type Response struct {
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data"`
}

type SyncJob struct {
	address ktypes.Address
	hash    ktypes.Hash
	peer    *SyncPeer
	mode    string
}
type TesseractSyncJob struct {
	tesseract *ktypes.Tesseract
}

type lattice interface {
	AddTesseractWithOutState(
		ts *ktypes.Tesseract,
		sender id.KramaID,
		ics *ktypes.ICSClusterInfo,
	) error
	GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error)
}
type db interface {
	CreateEntry([]byte, []byte) error
	AnnounceCIDEntry(hash ktypes.Hash)
	AnnounceBatchCIDEntries(hash []ktypes.Hash)
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	GetAccountMetaInfo(id []byte) (*ktypes.AccountMetaInfo, error)
	GetAccounts(bucketID int32) (ktypes.Accounts, error)
	UpdateAccounts(acc ktypes.Accounts) (int32, int64)
	GetBucketSizes() (map[int32]*big.Int, error)
	SetTesseractStatus(add []byte, height uint64, hash ktypes.Hash, status bool) error
}

type SyncPeer struct {
	id     peer.ID
	status *Status
	rw     *bufio.ReadWriter
	con    network.Conn
}

type Status struct {
	Accounts    *big.Int //potential memory leak
	bucketSizes sync.Map
	Ntq         float32
}
type Syncer struct {
	ctx              context.Context
	ctxCancel        context.CancelFunc
	node             *poorna.Server
	mux              *kutils.TypeMux
	status           *Status
	peers            sync.Map
	peerCount        uint32
	bsExchange       exchange.Interface
	db               db
	reqQueue         chan *SyncJob
	reqQueue1        chan *TesseractSyncJob
	wrkResults       chan interface{}
	accDetails       *ktypes.AccDetailsQueue
	tesseractSub     *kutils.Subscription
	statusSub        *kutils.Subscription
	newpeerSub       *kutils.Subscription
	mode             string
	rpcClient        *lrpc.Client
	lattice          lattice
	logger           hclog.Logger
	ReputationEngine *senatus.ReputationEngine
	ntqtablesynconce sync.Once
}

func NewSyncer(
	ctx context.Context,
	node *poorna.Server,
	mux *kutils.TypeMux,
	blockStore blockstore.Blockstore,
	db db,
	mode string,
	lattice lattice,
	logger hclog.Logger,
) (*Syncer, error) {
	ctx, ctxCancel := context.WithCancel(ctx)
	s := &Syncer{
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		node:       node,
		mux:        mux,
		bsExchange: bitswap.New(context.Background(), node.BsNetwork, blockStore),
		db:         db,
		mode:       mode,
		lattice:    lattice,
		peerCount:  0,
		logger:     logger.Named("Syncer"),
		reqQueue:   make(chan *SyncJob),
		reqQueue1:  make(chan *TesseractSyncJob),
		wrkResults: make(chan interface{}),
		accDetails: new(ktypes.AccDetailsQueue),
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
	currentValue, ok := sy.bucketSizes.Load(id)
	if ok && v.Cmp(currentValue.(*big.Int)) <= 0 {
		return false
	}

	sy.bucketSizes.Store(id, v)

	return true
}
func (sy *Status) incrementBucketCount(id int32, v int64) {
	currentSize, ok := sy.bucketSizes.Load(id)
	if !ok {
		sy.bucketSizes.Store(id, big.NewInt(v))
	} else {
		sy.bucketSizes.Store(id, new(big.Int).Add(currentSize.(*big.Int), big.NewInt(v)))
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

	msg := &ktypes.AccountsStatusMsg{
		TotalAccounts: s.status.Accounts.Bytes(), //FIXME: Race at status
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
	if err := s.rpcClient.Call(remotePeer, "SYNCRPC", "StatusUpdate", msg, resp); err != nil {
		log.Println("RPC Call panic", err)

		return
	}
}

// sendAccSyncRequest sends an account sync request to the remote peer
func (s *Syncer) sendAccSyncRequest(peer *SyncPeer) error { //nolint
	msg := &ktypes.AccountSyncRequest{
		BulkSync: true,
	}
	log.Println("Sending account sync request to peer:", peer.id)

	return peer.Send(s.node.GetKramaID(), ktypes.ACCSYNCREQ, msg)
}

// StatusUpdate is an rpc handler method used to update the status of a sync peer
func (s *Syncer) StatusUpdate(peerID peer.ID, msg *ktypes.AccountsStatusMsg) error {
	syncPeer, ok := s.peers.Load(peerID)
	if !ok {
		return errors.New("peer Not Found")
	}

	accountCount := new(big.Int).SetBytes(msg.TotalAccounts)
	if accountCount.Cmp(s.status.Accounts) <= 0 {
		return nil
	}
	//TODO: leaving NTQ
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

	msgs := make([]*ktypes.AccountSyncResponse, 0)
	msg := new(ktypes.AccountSyncResponse)
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
		if err := peer.Send(s.node.GetKramaID(), ktypes.ACCSYNCRRESP, v); err != nil {
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
func (p *SyncPeer) Send(id id.KramaID, code ktypes.MsgType, msg interface{}) error {
	// Marshal the proto message into slice of bytes and log and return if an error occurs
	bytes := polo.Polorize(msg)
	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into into a slice of bytes
	m := ktypes.Message{
		MsgType: code,
		Payload: bytes,
		Sender:  id,
	}

	bytes = polo.Polorize(&m)

	// Write the message bytes into the peer's iobuffer
	_, err := p.rw.Writer.Write(bytes)
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
		//TODO:Improve this
		if bytecount == 1 {
			continue
		}

		message := new(ktypes.Message)
		if err = polo.Depolorize(message, buffer[0:bytecount]); err != nil {
			log.Panicln("unmarshalling error", err, bytecount)
		}

		switch message.MsgType {
		case ktypes.NTQTABLESYNCREQ:

		case ktypes.NTQTABLESYNCRESP:
		case ktypes.ACCSYNCREQ:
			s.logger.Debug("Async message received from ", message.Sender)
			msg := new(ktypes.AccountSyncRequest)

			err = polo.Depolorize(msg, message.Payload)
			if err != nil {
				s.logger.Error("Error depolarizing account sync request", "error", err)
			}

			if msg.BulkSync {
				go func() {
					if err := s.accSync(p.id); err != nil {
						s.logger.Error("Error syncing address space", "error", err)
					}
				}()
			} else {
				go func() {
					if err := s.syncBucket(msg.Bucket, p); err != nil {
						s.logger.Error("Error syncing the bucket", "error", err)
					}
				}()
			}

		case ktypes.ACCSYNCRRESP:
			msg := new(ktypes.AccountSyncResponse)

			err = polo.Depolorize(msg, message.Payload)
			if err != nil {
				s.logger.Error("Error depolarising AccountSycResp message", "error", err)
			}
			s.logger.Debug("Address space messaged received from peer", message.Sender)

			s.accDetails.Push(msg.Accounts)
		}
	}
}

/*
// syncLattice will fetch the latest info of an account from the bestPeer and creates a lattice sync job
func (s *Syncer) syncLattice(addr []byte, mode string) error {
	accDetails, err := s.db.GetAccountDetails(addr)
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
		acc := ktypes.ProtoToAccDetails(resp)
		bucketNo, count := s.db.AddAddress(ktypes.Accounts{acc})
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

// fetchData fetches the complete state information associated with the given state CID,this uses bitswap to fetch the CID's.
func (s *Syncer) fetchData(id ktypes.Hash) (bool, error) {
	if id == ktypes.NilHash {
		return true, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if ok, err := s.db.Contains(id.Bytes()); ok || err != nil {
		return ok, err
	}
	CID, err := ktypes.HashToCid(id)
	if err != nil {
		return false, err
	}
	block, err := s.bsExchange.GetBlock(ctx, CID)
	if err != nil {
		return false, err
	}
	if err := s.db.CreateEntry(id.Bytes(), block.RawData()); err != nil {
		return false, err
	}

	return true, nil
}

// getContextObjects recursively fetches the context objects associated with the given CID using bitswap
func (s *Syncer) getContextObjects(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, h ktypes.Hash) {
	defer wg.Done()

	if ok, err := s.db.Contains(h.Bytes()); err != nil {
		cancel()

		return
	} else if ok {
		return
	}

	CID, err := ktypes.HashToCid(h)
	if err != nil {
		cancel()
	}
	block, err := s.bsExchange.GetBlock(ctx, CID)
	if err != nil {
		cancel()

		return
	}
	contextObject := new(ktypes.ContextObject)
	if err := polo.Depolorize(contextObject, block.RawData()); err != nil {
		cancel()

		return
	}
	if err := s.db.CreateEntry(h.Bytes(), block.RawData()); err != nil {
		cancel()

		return
	}
}

// getContextData fetches the behvaioural context and random context associated with the given CID using Bitswap
func (s *Syncer) getContextData(hash ktypes.Hash) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok, err := s.db.Contains(hash.Bytes())
	if ok || err != nil {
		return ok, err
	}

	cID, err := ktypes.HashToCid(hash)
	if err != nil {
		return false, err
	}

	block, err := s.bsExchange.GetBlock(ctx, cID)
	if err != nil {
		return false, err
	}

	metaContextObject := new(ktypes.MetaContextObject)
	if err := polo.Depolorize(metaContextObject, block.RawData()); err != nil {
		return false, err
	}
	writeFailed := false

	var wg sync.WaitGroup
	wg.Add(2)
	//FIXME
	go func() {
		select {
		case <-ctx.Done():
			writeFailed = true

			return
		}

	}()

	go s.getContextObjects(ctx, cancel, &wg, metaContextObject.BehaviouralContext)
	go s.getContextObjects(ctx, cancel, &wg, metaContextObject.RandomContext)
	wg.Wait()
	if writeFailed {
		return false, nil
	}
	if err := s.db.CreateEntry(hash.Bytes(), block.RawData()); err != nil && grpcStatus.Code(err) != codes.AlreadyExists {
		return false, err
	}

	return true, nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using bitswap
func (s *Syncer) fetchTesseractState(tesseract *ktypes.Tesseract) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // TODO: Timeout duration to be optimized
	defer cancel()
	CID, err := ktypes.HashToCid(tesseract.Body.StateHash)
	if err != nil {
		return err
	}
	block, err := s.bsExchange.GetBlock(ctx, CID)
	if err != nil {
		return err
	}

	data := block.RawData()
	acc := new(ktypes.Account)
	if err := polo.Depolorize(acc, data); err != nil {
		return err
	}
	//TODO:Fetch storage and logic trie as well
	if ok, err := s.fetchData(acc.Balance); !ok || err != nil {
		return err
	}
	if ok, err := s.fetchData(acc.StorageRoot); !ok || err != nil {
		return err
	}
	if ok, err := s.fetchData(acc.AssetApprovals); !ok || err != nil {
		return err
	}
	if ok, err := s.getContextData(acc.ContextHash); !ok || err != nil {
		return err
	}

	if err := s.db.CreateEntry(tesseract.Body.StateHash.Bytes(), block.RawData()); err != nil {
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

		peerCount := value.(*big.Int).Uint64()
		selfCount := cValue.(*big.Int).Uint64()

		if float64(selfCount) <= 0.6*float64(peerCount) {
			isRequired = true

			return false
		}

		return true
	})

	return isRequired
}

// tesseractWorker handles the tesseract Sync Jobs, it fetches the tesseract state using bitswap and updates the state in the db accordingly
func (s *Syncer) tesseractWorker(id int, reqQueue chan *TesseractSyncJob, respQueue chan interface{}) {
	for {
		select {
		case <-s.ctx.Done():
			log.Println("Closing tesseract worker", id)

			return
		case job, ok := <-reqQueue:
			if !ok {
				return
			}

			//TODO:Check whether tesseract data exsist

			log.Printf("Got a new tesseract info sync JOB for address %s,Height %d \n", job.tesseract.Header.Address.Hex(), job.tesseract.Header.Height)
			if err := s.fetchTesseractState(job.tesseract); err != nil {
				respQueue <- err
				log.Print(err)
			} else {
				if err := s.db.SetTesseractStatus(
					job.tesseract.Header.Address[:], job.tesseract.Header.Height, job.tesseract.Hash(), true); err != nil {
					log.Panic(err)
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
		go s.latticeWorker(i, s.reqQueue, s.wrkResults)
		go s.tesseractWorker(i, s.reqQueue1, s.wrkResults)
	}
	go func() {
		for {
			resp, ok := <-s.wrkResults
			if !ok {
				return
			}
			fmt.Println(resp)
		}
	}()
}

// handleSyncEvents handles the TesseractSyncJob created by the lattice manager
func (s *Syncer) handleSyncEvents() {
	for obj := range s.tesseractSub.Chan() {
		if t, ok := obj.Data.(kutils.TesseractSyncEvent); ok {
			go func() {
				s.reqQueue1 <- &TesseractSyncJob{tesseract: t.Tesseract}
			}()
		}
	}
}

// handleStatusEvents handles the status update created by the lattice manager
func (s *Syncer) handleStatusEvents() {
	for obj := range s.statusSub.Chan() {
		if t, ok := obj.Data.(kutils.SyncStatusUpdate); ok {
			s.status.incrementBucketCount(t.BucketID, t.Count)
			s.status.Accounts = new(big.Int).Add(s.status.Accounts, big.NewInt(t.Count))
			x, y := s.status.bucketSizes.Load(t.BucketID)
			log.Println("after updating the status", "Bucket No", x, "Count", y)
		}

	}
}

// handleNewPeer opens a new stream for the sync sub protocol and makes a  status update call to the newly discovered peer
func (s *Syncer) handleNewPeer() {
	// Read events from a newpeer channel
	for obj := range s.newpeerSub.Chan() {
		if p, ok := obj.Data.(kutils.PeerDiscoveredEvent); ok {
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

			msg := &ktypes.AccountsStatusMsg{
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
			if err := s.rpcClient.Call(p.ID, "SYNCRPC", "StatusUpdate", msg, resp); err != nil {
				log.Println("RPC Call panic", err)
			}
		}
	}
}

// Start  starts all event handlers and workers associated with sync sub protocol
func (s *Syncer) Start() {
	fmt.Println("Starting the syncer")
	s.node.SetupStreamHandler(SyncStreamProtocol, s.StreamHandler)
	s.tesseractSub = s.mux.Subscribe(kutils.TesseractSyncEvent{})
	s.statusSub = s.mux.Subscribe(kutils.SyncStatusUpdate{})
	s.newpeerSub = s.mux.Subscribe(kutils.PeerDiscoveredEvent{})

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
	//update the status from db
	if err := s.statusInit(); err != nil {
		log.Panicln(err)
	}
	stop := true

	for stop {
		//var bestPeer *SyncPeer
		if atomic.LoadUint32(&s.peerCount) == poorna.MinimumPeerCount {
			bestPeer := s.BestPeer()

			time.Sleep(1 * time.Second)

			s.ntqtablesynconce.Do(func() {
				if err := bestPeer.Send(s.node.GetKramaID(), ktypes.NTQTABLESYNCREQ, nil); err != nil {
					s.logger.Error("Error sending NTQ sync request", "error", err)
				}
			})
			if bestPeer.status.Accounts.Cmp(s.status.Accounts) > 0 {
				//This following logic will be replaced with sendAccsyncReq
				for k, v := range bestPeer.con.GetStreams() {
					log.Println(k, v.Protocol(), v.ID(), bestPeer.id, bestPeer.id)

					if v.Protocol() == protocol.ID(SyncStreamProtocol) {
						msg := &ktypes.AccountSyncRequest{
							BulkSync: true,
						}

						bytes := polo.Polorize(msg)

						finalMsg := ktypes.Message{
							MsgType: ktypes.ACCSYNCREQ,
							Sender:  s.node.GetKramaID(),
							Payload: bytes,
						}
						rawBytes := polo.Polorize(&finalMsg)

						rw := bufio.NewReadWriter(bufio.NewReader(v), bufio.NewWriter(v))
						_, err := rw.Writer.Write(rawBytes)
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

// latticeWorker recursively sync the complete tesseract lattice from thee best peer using RPC
func (s *Syncer) latticeWorker(id int, job <-chan *SyncJob, result chan<- interface{}) {
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
				tesseractStack := new(ktypes.TesseractStack)

				for !exists {
					log.Println(" Lattice Sync Job Received of address ", job.address.Hex(), "Hash", hash)
					t, delta, err := s.getTesseract(job.peer, hash, nil)
					if err != nil {
						result <- fmt.Sprintf("Unable to get tesseract %s from %s", job.peer.id, hash)

						return
					} else {
						tesseractStack.Push(&ktypes.Item{Tesseract: t, Delta: delta, Sender: "job.peer"}) //FIXME:
					}

					if t.Header.PrevHash != ktypes.NilHash {
						exists, err = s.db.Contains(t.Header.PrevHash.Bytes())
						if err != nil {
							result <- fmt.Sprintf("Unable to get tesseract  previous hash %s", hash)

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
					log.Printf("Adding %s tesseract to lattice %v", item.Tesseract.Header.Address.Hex(), item.Tesseract.Hash())

					icsClusterInfo := new(ktypes.ICSClusterInfo)
					if err := polo.Depolorize(icsClusterInfo, item.Delta[item.Tesseract.Body.ConsensusProof.ICSHash]); err != nil {
						s.logger.Error("Error depolarising ics cluster Info", "err", err)
					}

					if err := s.lattice.AddTesseractWithOutState(item.Tesseract, item.Sender, icsClusterInfo); err != nil {
						log.Fatal("Unable to add tesseract", err)
					}
				}
			} else {
				if t, delta, err := s.getTesseract(job.peer, job.hash, nil); err != nil {
					log.Printf("Unable to get tesseract %s from %s Error %s", job.peer.id, job.hash.Hex(), err)
				} else {
					icsClusterInfo := new(ktypes.ICSClusterInfo)
					if err := polo.Depolorize(icsClusterInfo, delta[t.Body.ConsensusProof.ICSHash]); err != nil {
						s.logger.Error("Error depolarising ics cluster Info", "err", err)
					}

					//FIXME: Fix the peer id
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
		syncPeer := peer.(*SyncPeer)
		if bestPeer == nil {
			bestPeer = syncPeer
		} else if syncPeer.status.Accounts.Cmp(bestPeer.status.Accounts) > 0 {
			bestPeer = syncPeer
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
func (s *Syncer) getTesseract(bestPeer *SyncPeer, hash ktypes.Hash, number *big.Int) (*ktypes.Tesseract, map[ktypes.Hash][]byte, error) {
	req := new(ktypes.TesseractReq)

	log.Println("get tesseract call", hash)

	if hash != ktypes.NilHash {
		req.Hash = hash
	}
	if number != nil {
		req.Number = number.Uint64()
	}

	resp := new(ktypes.TesseractResponse)
	if err := s.rpcClient.Call(bestPeer.id, "SYNCRPC", "GetTesseract", req, resp); err != nil {
		return nil, nil, err
	}

	msg := new(ktypes.Tesseract)

	err := polo.Depolorize(msg, resp.Data)
	if err != nil {
		return nil, nil, err
	}

	return msg, resp.Delta, nil
}

func (s *Syncer) GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	ts, err := s.lattice.GetTesseract(hash)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func (s *Syncer) Close() {
	s.ctxCancel()
	log.Println("Closing syncer")
}
