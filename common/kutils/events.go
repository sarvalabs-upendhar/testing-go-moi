package kutils

import (
	"bufio"
	"errors"
	"fmt"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"reflect"
	"sync"
	"time"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

// NewIxsEvent occurs when new transactions enter the transaction pool
type NewIxsEvent struct {
	Ixs []*ktypes.Interaction
}

// NewPeerEvent occurs when a new peer is discovered in KIP network
type NewPeerEvent struct {
	PeerID peer.ID
}

type FCSPeerEvent struct {
	Rw           bufio.ReadWriter
	ID           peer.ID
	IP           multiaddr.Multiaddr
	ClusterID    string
	Participants []ktypes.Address
	CHash        []ktypes.Hash
}
type PeerDiscoveredEvent struct {
	ID peer.ID
}

// NewMinedTesseractEvent occurs when new block is generated
type NewMinedTesseractEvent struct {
	Tesseract *ktypes.Tesseract
	Delta     map[ktypes.Hash][]byte
}

// TesseractReceivedEvent occurs when a new block is received from the peer
type TesseractReceivedEvent struct {
	Tesseract   *ktypes.Tesseract
	ClusterInfo *ktypes.ICSClusterInfo
	Sender      id.KramaID
}

// TesseractSyncEvent is fired when a new tesseract received and needs to be synced up.
type TesseractSyncEvent struct {
	Tesseract *ktypes.Tesseract
}

type SyncStatusUpdate struct {
	BucketID int32
	Count    int64
}

// TypeMuxEvent is a time-tagged notification pushed to subscribers.
type TypeMuxEvent struct {
	Time time.Time
	Data interface{}
}

// Subscription is a subscription established through TypeMux.
type Subscription struct {
	mux              *TypeMux
	created          time.Time
	closeMultiplexer sync.Mutex
	closing          chan struct{}
	closed           bool
	postMu           sync.RWMutex
	readChannel      <-chan *TypeMuxEvent
	postChannel      chan<- *TypeMuxEvent
}

// A TypeMux dispatches events to registered receivers.
type TypeMux struct {
	mutex      sync.RWMutex
	submapping map[reflect.Type][]*Subscription
	stopped    bool
}

// ErrMuxClosed is returned when Posting on a closed TypeMux.
var ErrMuxClosed = errors.New("event: mux closed")

// Subscribe creates a subscription for events of the given types.
func (mux *TypeMux) Subscribe(types ...interface{}) *Subscription {
	sub := newsubsciption(mux)

	mux.mutex.Lock()
	defer mux.mutex.Unlock()

	if mux.stopped {
		sub.closed = true
		close(sub.postChannel)
	} else {
		if mux.submapping == nil {
			mux.submapping = make(map[reflect.Type][]*Subscription)
		}

		for _, t := range types {
			rtyp := reflect.TypeOf(t)
			oldsubs := mux.submapping[rtyp]
			if search(oldsubs, sub) != -1 {
				panic(fmt.Sprintf("event: duplicate type %s in Subscribe", rtyp))
			}
			subs := make([]*Subscription, len(oldsubs)+1)
			copy(subs, oldsubs)
			subs[len(oldsubs)] = sub
			mux.submapping[rtyp] = subs
		}
	}

	return sub
}

// Post sends an event to all receivers registered for the given type.
func (mux *TypeMux) Post(ev interface{}) error {
	event := &TypeMuxEvent{
		Time: time.Now(),
		Data: ev,
	}

	rtyp := reflect.TypeOf(ev)

	mux.mutex.RLock()

	if mux.stopped {
		mux.mutex.RUnlock()

		return ErrMuxClosed
	}

	subs := mux.submapping[rtyp]

	mux.mutex.RUnlock()

	for _, s := range subs {
		s.sendAll(event)
	}

	return nil
}

// Stop closes a mux. The mux can no longer be used.
func (mux *TypeMux) Stop() {
	mux.mutex.Lock()

	for _, subs := range mux.submapping {
		for _, sub := range subs {
			sub.closeWait()
		}
	}

	mux.submapping = nil
	mux.stopped = true

	mux.mutex.Unlock()
}

// del is used to delete the Subscription from the TypeMux
func (mux *TypeMux) delete(s *Subscription) {
	mux.mutex.Lock()
	for typ, subs := range mux.submapping {
		if pos := search(subs, s); pos >= 0 {
			if len(subs) == 1 {
				delete(mux.submapping, typ)
			} else {
				mux.submapping[typ] = posdelete(subs, pos)
			}
		}
	}
	s.mux.mutex.Unlock()
}

// search is used to search for a subscription object
func search(slice []*Subscription, item *Subscription) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}

	return -1
}

// posdelete is used to delete the subscription at the particular position
func posdelete(slice []*Subscription, pos int) []*Subscription {
	news := make([]*Subscription, len(slice)-1)
	copy(news[:pos], slice[:pos])
	copy(news[pos:], slice[pos+1:])

	return news
}

// newsubscription is used to create a new subscription used the TypeMux
func newsubsciption(mux *TypeMux) *Subscription {
	c := make(chan *TypeMuxEvent)

	return &Subscription{
		mux:         mux,
		created:     time.Now(),
		readChannel: c,
		postChannel: c,
		closing:     make(chan struct{}),
	}
}

// Chan return the readChannel channel
func (s *Subscription) Chan() <-chan *TypeMuxEvent {
	return s.readChannel
}

// Unsubscribe removes the specified subscription from Typemux
func (s *Subscription) Unsubscribe() {
	s.mux.delete(s)
	s.closeWait()
}

// isClosed returns the status of the subscription
func (s *Subscription) isClosed() bool { //nolint
	s.closeMultiplexer.Lock()
	defer s.closeMultiplexer.Unlock()

	return s.closed
}

// closeWait will wait  and close the subscription
func (s *Subscription) closeWait() {
	s.closeMultiplexer.Lock()
	defer s.closeMultiplexer.Unlock()

	if s.closed {
		return
	}

	close(s.closing)
	s.closed = true

	s.postMu.Lock()
	close(s.postChannel)
	s.postChannel = nil
	s.postMu.Unlock()
}

// sendAll sends the event to all the subscribed process
func (s *Subscription) sendAll(event *TypeMuxEvent) {
	// Short circuit delivery if stale event
	if s.created.After(event.Time) {
		return
	}
	// Otherwise, deliver the event
	s.postMu.RLock()
	defer s.postMu.RUnlock()

	select {
	case s.postChannel <- event:
	case <-s.closing:
	}
}
