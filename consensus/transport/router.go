package transport

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
)

const (
	ConnMaintenanceInterval = 100 * time.Millisecond
)

// VoteRegistry represents a collection for storing consensus vote message hashes.
type VoteRegistry struct {
	votes mapset.Set
}

// newVoteRegistry creates a new instance of VoteRegistry.
func newVoteRegistry() *VoteRegistry {
	return &VoteRegistry{
		votes: mapset.NewSet(),
	}
}

// add inserts a vote message hash into the VoteRegistry.
func (rc *VoteRegistry) add(voteHash common.Hash) error {
	if !rc.votes.Contains(voteHash) {
		rc.votes.Add(voteHash)

		return nil
	}

	return errors.New("Votes message already seen")
}

// has checks if a vote message hash is already present in the VoteRegistry.
func (rc *VoteRegistry) has(voteHash common.Hash) bool {
	return rc.votes.Contains(voteHash)
}

// RequestCache represents a cache for storing requested vote message indexes to prevent duplicate requests.
type RequestCache struct {
	mtx      sync.RWMutex
	requests *expirable.LRU[string, interface{}]
}

// newRequestCache creates and returns a new instance of RequestCache.
func newRequestCache() *RequestCache {
	return &RequestCache{
		mtx:      sync.RWMutex{},
		requests: expirable.NewLRU[string, interface{}](5000, nil, 300*time.Millisecond),
	}
}

// requestCacheKey generates a unique cache key based on the round, message type, and index.
func requestCacheKey(cluster common.ClusterID, round int32, msgType types.ConsensusMsgType, index int) string {
	return fmt.Sprintf("%s_%d_%d_%d", cluster, round, msgType, index)
}

// add inserts a requestKey into the RequestCache.
func (rc *RequestCache) add(requestKey string, value interface{}) {
	rc.mtx.Lock()
	defer rc.mtx.Unlock()

	rc.requests.Add(requestKey, value)
}

// has checks whether a given requestKey is present in the RequestCache.
func (rc *RequestCache) has(requestKey string) bool {
	rc.mtx.RLock()
	defer rc.mtx.RUnlock()

	return rc.requests.Contains(requestKey)
}

// PeerVoteSet keeps track of the voting status of peers, indicating whether they have voted for each
// consensus round and message type.
type PeerVoteSet struct {
	mtx     sync.RWMutex
	voteset map[string]*common.ArrayOfBits
}

// newPeerVoteSet creates and returns a new instance of PeerVoteSet.
func newPeerVoteSet() *PeerVoteSet {
	return &PeerVoteSet{
		mtx:     sync.RWMutex{},
		voteset: make(map[string]*common.ArrayOfBits),
	}
}

// PendingVotes contains votes waiting to be forwarded to peers.
type PendingVotes struct {
	mtx   sync.RWMutex
	votes []*types.Vote
}

// newPendingVotes creates and returns a new instance of PendingVotes
func newPendingVotes() *PendingVotes {
	return &PendingVotes{
		mtx:   sync.RWMutex{},
		votes: make([]*types.Vote, 0),
	}
}

// add adds the given votes to the list.
func (mq *PendingVotes) add(votes ...*types.Vote) {
	mq.mtx.Lock()
	defer mq.mtx.Unlock()

	mq.votes = append(mq.votes, votes...)
}

// list returns a copy of the pending votes list
func (mq *PendingVotes) list() []*types.Vote {
	mq.mtx.RLock()
	defer mq.mtx.RUnlock()

	votes := make([]*types.Vote, len(mq.votes))
	copy(votes, mq.votes)

	return votes
}

// isEmpty checks if the list is empty.
func (mq *PendingVotes) isEmpty() bool {
	mq.mtx.RLock()
	defer mq.mtx.RUnlock()

	return len(mq.votes) == 0
}

// GossipPeer keeps track of the peer voteset, pending message queue and connection status.
type GossipPeer struct {
	voteset      *PeerVoteSet
	pendingVotes *PendingVotes
	mtx          sync.RWMutex
	connected    bool
}

// newGossipPeer creates and returns a new instance of GossipPeer.
func newGossipPeer() *GossipPeer {
	return &GossipPeer{
		voteset:      newPeerVoteSet(),
		pendingVotes: newPendingVotes(),
	}
}

// isConnected returns true if the peer is connected, otherwise false.
func (gp *GossipPeer) isConnected() bool {
	gp.mtx.RLock()
	defer gp.mtx.RUnlock()

	return gp.connected
}

// setConnectionStatus updates the connection status of the peer.
func (gp *GossipPeer) setConnectionStatus(connected bool) {
	gp.mtx.Lock()
	defer gp.mtx.Unlock()

	gp.connected = connected
}

// GossipPeers keeps track of the gossip peers.
type GossipPeers struct {
	mtx   sync.RWMutex
	peers map[id.KramaID]*GossipPeer
}

// newGossipPeers creates and returns a new instance of GossipPeers.
func newGossipPeers() *GossipPeers {
	peers := make(map[id.KramaID]*GossipPeer)

	return &GossipPeers{
		mtx:   sync.RWMutex{},
		peers: peers,
	}
}

// add inserts a new peer with a corresponding PeerVoteSet.
func (ap *GossipPeers) add(peerID id.KramaID, connected bool) {
	ap.mtx.Lock()
	defer ap.mtx.Unlock()

	ap.peers[peerID] = newGossipPeer()
	ap.peers[peerID].connected = connected
}

func (ap *GossipPeers) has(peerID id.KramaID) bool {
	ap.mtx.RLock()
	defer ap.mtx.RUnlock()

	_, ok := ap.peers[peerID]

	return ok
}

// get retrieves the GossipPeer associated with the given peerID.
func (ap *GossipPeers) get(peerID id.KramaID) *GossipPeer {
	ap.mtx.RLock()
	defer ap.mtx.RUnlock()

	return ap.peers[peerID]
}

// entries returns a map containing all the peer IDs in the GossipPeers.
func (ap *GossipPeers) entries() map[id.KramaID]struct{} {
	ap.mtx.RLock()
	defer ap.mtx.RUnlock()

	peers := make(map[id.KramaID]struct{})

	for peerID := range ap.peers {
		peers[peerID] = struct{}{}
	}

	return peers
}

// list returns a list of all the peer IDs in the GossipPeers
func (ap *GossipPeers) list() []id.KramaID {
	ap.mtx.RLock()
	defer ap.mtx.RUnlock()

	peers := make([]id.KramaID, 0)

	for peerID := range ap.peers {
		peers = append(peers, peerID)
	}

	return peers
}

// len returns the number of active peers.
func (ap *GossipPeers) len(connectedOnly bool) int {
	ap.mtx.RLock()
	defer ap.mtx.RUnlock()

	peers := make([]id.KramaID, 0)

	for peerID, peer := range ap.peers {
		if connectedOnly && !peer.connected {
			continue
		}

		peers = append(peers, peerID)
	}

	return len(peers)
}

// remove deletes a peer from the GossipPeers.
func (ap *GossipPeers) remove(peerID id.KramaID) {
	ap.mtx.Lock()
	defer ap.mtx.Unlock()

	delete(ap.peers, peerID)
}

// ContextRouter represents a router which handles ics vote messages, and connections.
type ContextRouter struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	mtx          sync.RWMutex
	selfID       id.KramaID
	clusterID    common.ClusterID
	logger       hclog.Logger
	transport    *KramaTransport
	nodeset      *common.ICSNodeSet
	voteset      *kbft.HeightVoteSet
	voteRegistry *VoteRegistry
	requestCache *RequestCache
	gossipPeers  *GossipPeers
	expiresAt    int64
	operator     id.KramaID
}

// NewContextRouter returns a new instance of ContextRouter.
func NewContextRouter(
	ctx context.Context,
	selfID, operator id.KramaID,
	clusterID common.ClusterID,
	logger hclog.Logger,
	nodeset *common.ICSNodeSet,
	voteset *kbft.HeightVoteSet,
	transport *KramaTransport,
) *ContextRouter {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &ContextRouter{
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		mtx:          sync.RWMutex{},
		selfID:       selfID,
		operator:     operator,
		clusterID:    clusterID,
		logger:       logger.Named("Context-Router"),
		nodeset:      nodeset,
		voteset:      voteset,
		voteRegistry: newVoteRegistry(),
		requestCache: transport.requestCache,
		transport:    transport,
		gossipPeers:  newGossipPeers(),
	}
}

// broadcast periodically broadcasts ICSHAVE votes to all the connected peers.
func (cr *ContextRouter) broadcast(broadcastInterval time.Duration) {
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cr.ctx.Done():
			return
		case <-ticker.C:
			roundVoteBitSet := cr.voteset.GetRoundBitVoteSet()
			if len(roundVoteBitSet) == 0 {
				continue
			}

			msg := &types.ICSHave{
				RoundVoteBitSets: roundVoteBitSet,
			}

			payload, err := msg.Bytes()
			if err != nil {
				continue
			}

			for k, v := range roundVoteBitSet {
				cr.logger.Trace(
					"!!!! Sending IHave message to peer",
					"round", k,
					"vote-set", v.Prevotes,
					"vote-set-1", v.Precommits)
			}

			cr.transport.BroadcastMessage(cr.ctx, &types.ICSMSG{
				Sender:    cr.selfID,
				ClusterID: cr.clusterID,
				MsgType:   message.ICSHAVE,
				Payload:   payload,
			})
		}
	}
}

// handleICSHave handles incoming ICSHAVE messages.
func (cr *ContextRouter) handleICSHave(msg *types.ICSMSG) error {
	icsHave := new(types.ICSHave)

	err := icsHave.FromBytes(msg.Payload)
	if err != nil {
		return err
	}

	updatedICSHave := &types.ICSHave{
		Votes: make([]*types.Vote, 0, len(icsHave.Votes)),
	}

	for _, v := range icsHave.Votes {
		voteHash, err := v.Hash()
		if err != nil {
			return errors.Wrap(err, "failed to compute vote hash")
		}

		cr.logger.Trace("Received ICS have",
			"from", msg.ReceivedFrom,
			"votesHash", voteHash,
		)

		if cr.voteRegistry.has(voteHash) {
			continue
		}

		if err = cr.voteRegistry.add(voteHash); err != nil {
			continue
		}

		updatedICSHave.Votes = append(updatedICSHave.Votes, v)
	}

	if len(updatedICSHave.Votes) > 0 {
		msg.DecodedMsg = updatedICSHave

		cr.transport.forwardMsgToEngine(msg)

		if len(updatedICSHave.Votes) != len(icsHave.Votes) {
			msg.Payload, err = updatedICSHave.Bytes()
			if err != nil {
				return err
			}
		}

		cr.transport.BroadcastMessage(
			cr.ctx,
			msg,
		)
	}

	// If the gossip peer is not active, do not proceed further
	if gossipPeer := cr.gossipPeers.get(msg.Sender); gossipPeer == nil || !gossipPeer.isConnected() {
		return nil
	}

	if len(icsHave.RoundVoteBitSets) == 0 {
		return nil
	}

	requiredVoteSet := make(map[int32]*types.VoteBitSet, len(icsHave.RoundVoteBitSets))

	currentVoteSet := cr.voteset.GetRoundBitVoteSet()

	for round, peerVoteSet := range icsHave.RoundVoteBitSets {
		if peerVoteSet == nil {
			continue
		}

		if !peerVoteSet.Prevotes.IsEmpty() &&
			currentVoteSet[round] != nil &&
			currentVoteSet[round].Prevotes != nil {
			trueIndices := peerVoteSet.Prevotes.GetTrueIndicesMap()
			localIndices := currentVoteSet[round].Prevotes.GetTrueIndicesMap()

			for index := range trueIndices {
				cacheKey := requestCacheKey(msg.ClusterID, round, types.PREVOTE, index)

				if _, ok := localIndices[index]; !ok && !cr.requestCache.has(cacheKey) {
					if requiredVoteSet[round] == nil {
						requiredVoteSet[round] = &types.VoteBitSet{
							Prevotes: common.NewArrayOfBits(peerVoteSet.Prevotes.SizeOf()),
						}
					}

					cr.requestCache.add(cacheKey, struct{}{})

					requiredVoteSet[round].Prevotes.SetIndex(index, true)
				}
			}
		}

		if !peerVoteSet.Precommits.IsEmpty() &&
			currentVoteSet[round] != nil &&
			currentVoteSet[round].Precommits != nil {
			trueIndices := peerVoteSet.Precommits.GetTrueIndicesMap()
			localIndices := currentVoteSet[round].Precommits.GetTrueIndicesMap()

			for index := range trueIndices {
				cacheKey := requestCacheKey(msg.ClusterID, round, types.PRECOMMIT, index)

				if _, ok := localIndices[index]; !ok && !cr.requestCache.has(cacheKey) {
					if requiredVoteSet[round] == nil {
						requiredVoteSet[round] = &types.VoteBitSet{
							Precommits: common.NewArrayOfBits(peerVoteSet.Precommits.SizeOf()),
						}
					}

					cr.requestCache.add(cacheKey, struct{}{})

					requiredVoteSet[round].Precommits.SetIndex(index, true)
				}
			}
		}
	}

	if len(requiredVoteSet) == 0 {
		return nil
	}

	for k, v := range requiredVoteSet {
		cr.logger.Trace(
			"Sending IWANT message to peer",
			"round", k,
			"peer-id", msg.Sender,
			"prevote-set", v.Prevotes,
			"precommit-set", v.Precommits)
	}

	err = cr.transport.SendMessage(
		msg.Sender,
		cr.selfID,
		cr.clusterID,
		message.ICSWANT,
		types.NewICSWant(requiredVoteSet),
	)
	if err != nil {
		return err
	}

	return nil
}

// handleICSWant handles incoming ICSWANT messages.
func (cr *ContextRouter) handleICSWant(msg *types.ICSMSG) error {
	var icsWant types.ICSWant

	err := icsWant.FromBytes(msg.Payload)
	if err != nil {
		return err
	}

	cr.logger.Trace("Handling ICSWant", "peer", msg.Sender)

	votes := cr.voteset.GetVotes(icsWant.RoundVoteBitSets)
	cr.logger.Trace("Sending Votes", "peer", msg.Sender)

	icsHave := types.NewICSHave(nil, votes...)

	err = cr.transport.SendMessage(msg.Sender, cr.selfID, cr.clusterID, message.ICSHAVE, icsHave)
	if err != nil {
		cr.logger.Error("Failed to send ics have message", err)
	}

	return nil
}

// handleMessage handles incoming ICS votes based on their type.
func (cr *ContextRouter) handleMessage(msg *types.ICSMSG) {
	switch msg.MsgType {
	case message.ICSHAVE:
		if err := cr.handleICSHave(msg); err != nil {
			cr.logger.Error("Failed to handle ics have message", err)
		}
	case message.ICSWANT:
		if err := cr.handleICSWant(msg); err != nil {
			cr.logger.Error("Failed to handle ics want message", err)
		}
	default:
		cr.logger.Error("Invalid message type")
	}
}

// getAvailableNodes returns available peers excluding the self and known peers.
func (cr *ContextRouter) getAvailableNodes() []id.KramaID {
	availableNodes := make([]id.KramaID, 0)

	for _, kramaID := range cr.nodeset.GetNodes(true) {
		if kramaID == cr.selfID {
			continue
		}

		if gossipPeer := cr.gossipPeers.get(kramaID); gossipPeer == nil || !gossipPeer.isConnected() {
			availableNodes = append(availableNodes, kramaID)
		}
	}

	return availableNodes
}

// getRandomNode returns a random node from the nodeset excluding the node itself and known peers.
func (cr *ContextRouter) getRandomNode(nodes []id.KramaID) (id.KramaID, error) {
	if len(nodes) == 0 {
		return "", errors.New("no random peer available")
	}

	s1 := rand.NewSource(time.Now().UnixNano())
	randGen := rand.New(s1)
	index := randGen.Intn(len(nodes))

	return nodes[index], nil
}

func (cr *ContextRouter) getRandomPeers() []id.KramaID {
	nodes := cr.nodeset.GetNodes(false)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	randomPeers := make([]id.KramaID, 0, cr.transport.minGossipPeers)

	if len(nodes) <= cr.transport.minGossipPeers {
		return []id.KramaID{}
	}

	existingPeer := make(map[id.KramaID]bool)

	for i := 0; i < cr.transport.minGossipPeers; i++ {
		randIndex := r.Intn(len(nodes))

		if cr.selfID == nodes[randIndex] || existingPeer[nodes[randIndex]] || cr.gossipPeers.has(nodes[randIndex]) {
			i--

			continue
		}

		existingPeer[nodes[randIndex]] = true

		randomPeers = append(randomPeers, nodes[randIndex])
	}

	return randomPeers
}

// connectToGossipPeers initiates connection attempts to gossip peers.
func (cr *ContextRouter) connectToGossipPeers(ctx context.Context) {
	waitGroup := sync.WaitGroup{}

	for _, peerID := range cr.transport.getTransitPeers(cr.clusterID) {
		cr.gossipPeers.add(peerID, false)
	}

	for _, peerID := range cr.getRandomPeers() {
		cr.gossipPeers.add(peerID, false)
	}

	cr.transport.removeTransitPeers(cr.clusterID)

	cr.logger.Trace("!!!!!.....Gossip peers.....!!!!!", cr.gossipPeers.entries())

	for kramaID := range cr.gossipPeers.entries() {
		waitGroup.Add(1)

		// TODO: Can we improve this
		go func(kramaID id.KramaID) {
			defer waitGroup.Done()

			if err := cr.transport.ConnectToMeshPeer(ctx, kramaID, cr.clusterID); err != nil {
				cr.logger.Trace("Failed to connect random peer", "krama-id", kramaID, "err", err)
			}
		}(kramaID)
	}

	waitGroup.Wait()

	go cr.maintainConnections(ConnMaintenanceInterval, cr.transport.maxGossipPeers)
}

// maintainConnections periodically attempts to maintain a minimum number of connections by
// connecting to random peers.
func (cr *ContextRouter) maintainConnections(connMaintenanceInterval time.Duration, minConnCount int) {
	ticker := time.NewTicker(connMaintenanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cr.ctx.Done():
			return
		case <-ticker.C:
			waitGroup := new(sync.WaitGroup)
			availableNodes := cr.getAvailableNodes()

			for i := cr.gossipPeers.len(true); i <= minConnCount; i++ {
				kramaID, err := cr.getRandomNode(availableNodes)
				if err != nil {
					cr.logger.Trace("Failed to get random peer", err)

					break
				}

				waitGroup.Add(1)

				go func(kramaID id.KramaID) {
					defer waitGroup.Done()

					if err := cr.transport.ConnectToMeshPeer(cr.ctx, kramaID, cr.clusterID); err != nil {
						cr.logger.Error("Failed to connect random peer", "krama-id", kramaID, "err", err)

						return
					}
				}(kramaID)
			}

			waitGroup.Wait()
		}
	}
}

// getVotesetBits returns the voteset for a specific consensus round and message type.
func (cr *ContextRouter) getRoundVoteSetBits() (map[int32]*types.VoteBitSet, error) {
	voteset := cr.voteset.GetRoundBitVoteSet()
	if voteset == nil {
		return nil, errors.New("vote set not found")
	}

	return voteset, nil
}

// getExpiryTime returns the expiry timestamp of the context router.
func (cr *ContextRouter) getExpiryTime() int64 {
	cr.mtx.RLock()
	defer cr.mtx.RUnlock()

	return cr.expiresAt
}

// setExpiryTime updates the expiry timestamp of the context router.
func (cr *ContextRouter) setExpiryTime() {
	cr.mtx.Lock()
	defer cr.mtx.Unlock()

	cr.expiresAt = time.Now().Add(2 * time.Minute).Unix()
}

// close cancels the context, terminates the router.
func (cr *ContextRouter) close() {
	cr.ctxCancel()
}

func (cr *ContextRouter) getBroadcastPeers(msgType message.MsgType) []id.KramaID {
	if msgType == message.ICSSUCCESS || msgType == message.ICSFAILURE {
		return cr.nodeset.GetNodes(true)
	}

	return cr.gossipPeers.list()
}

// sendPendingMessages sends the pending votes to specific peer if exists
func (cr *ContextRouter) sendPendingMessages(clusterID common.ClusterID, kramaID id.KramaID) {
	gossipPeer := cr.gossipPeers.get(kramaID)
	if gossipPeer == nil || gossipPeer.pendingVotes.isEmpty() {
		return
	}

	icsHave := types.NewICSHave(nil, gossipPeer.pendingVotes.list()...)

	err := cr.transport.SendMessage(kramaID, cr.selfID, clusterID, message.ICSHAVE, icsHave)
	if err != nil {
		cr.logger.Error("Failed to send pending message", "krama-id", kramaID, "err", err)

		return
	}
}
