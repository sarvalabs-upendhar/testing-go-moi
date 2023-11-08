package websocket

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/jsonrpc/api"
)

const (
	// DefaultTimeout is the timeout to remove the subscriptions that don't have a web socket stream
	DefaultTimeout = 1 * time.Minute
	// DefaultHeapIndex indicates that the element is not in heap
	DefaultHeapIndex = -1
)

// SubscriptionManager manages all the subscriptions
type SubscriptionManager struct {
	logger            hclog.Logger
	timeout           time.Duration
	lock              sync.RWMutex
	subscriptions     map[string]subscription
	timeouts          Timeouts
	updateCh          chan struct{}
	closeCh           chan struct{}
	eventSubscription *utils.Subscription
}

func NewSubscriptionManager(logger hclog.Logger, eventMux *utils.TypeMux) *SubscriptionManager {
	subscriptionManager := &SubscriptionManager{
		logger:            logger.Named("Websocket-Subscription"),
		timeout:           DefaultTimeout,
		subscriptions:     make(map[string]subscription),
		timeouts:          Timeouts{},
		updateCh:          make(chan struct{}),
		closeCh:           make(chan struct{}),
		eventSubscription: subscribeEvents(eventMux),
	}

	return subscriptionManager
}

func (f *SubscriptionManager) hasSubscribed(subscriptionID string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if _, ok := f.subscriptions[subscriptionID]; ok {
		return true
	}

	return false
}

// subscribeEvents subscribes for events of the given types
func subscribeEvents(eventMux *utils.TypeMux) *utils.Subscription {
	return eventMux.Subscribe(utils.TesseractAddedEvent{})
}

// Run starts worker process to handle events
func (f *SubscriptionManager) Run() {
	watchCh := make(chan *utils.TypeMuxEvent)

	go func() {
		for event := range f.eventSubscription.Chan() {
			watchCh <- event
		}
	}()

	var timeoutCh <-chan time.Time

	for {
		// check for the next subscription to be removed
		subscriptionBase := f.nextTimeoutSubscription()

		// set timer to remove subscription
		if subscriptionBase != nil {
			timeoutCh = time.After(time.Until(subscriptionBase.expiredAt))
		}

		select {
		case event := <-watchCh:
			if err := f.dispatchEvent(event); err != nil {
				f.logger.Error("Failed to dispatch event", "err", err)
			}

		case <-timeoutCh:
			// timeout for subscription
			// if subscription still exists
			if subscriptionBase != nil && !f.Uninstall(subscriptionBase.id) {
				f.logger.Error("Failed to uninstall subscription", "subscription-ID", subscriptionBase.id)
			}

		case <-f.updateCh:
			// subscriptions change, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the subscription manager
			return
		}
	}
}

func (f *SubscriptionManager) emitSignalToUpdateCh() {
	select {
	// notify worker of new subscription with timeout
	case f.updateCh <- struct{}{}:
	default:
	}
}

// removeSubscriptionByID removes the subscription with given ID, unsafe against race condition
func (f *SubscriptionManager) removeSubscriptionByID(id string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	subscription, ok := f.subscriptions[id]
	if !ok {
		return false
	}

	delete(f.subscriptions, id)

	if removed := f.timeouts.removeSubscription(subscription.getSubscriptionBase()); removed {
		f.emitSignalToUpdateCh()
	}

	return true
}

// NewAccountTesseractSubscription adds new TesseractSubscription based on address
func (f *SubscriptionManager) NewAccountTesseractSubscription(ws ConnManager, addr common.Address) string {
	subscription := &tesseractAccountSubscription{
		subscriptionBase: newSubscriptionBase(ws),
		address:          addr,
	}

	if subscription.hasWSConn() {
		ws.SetSubscriptionID(subscription.id)
	}

	return f.addSubscription(subscription)
}

// NewTesseractSubscription adds new TesseractSubscription
func (f *SubscriptionManager) NewTesseractSubscription(ws ConnManager) string {
	subscription := &tesseractSubscription{
		subscriptionBase: newSubscriptionBase(ws),
	}

	if subscription.hasWSConn() {
		ws.SetSubscriptionID(subscription.id)
	}

	return f.addSubscription(subscription)
}

// addSubscription is an internal method to add given subscription to list and heap
func (f *SubscriptionManager) addSubscription(subscription subscription) string {
	f.lock.Lock()
	defer f.lock.Unlock()

	base := subscription.getSubscriptionBase()

	f.subscriptions[base.id] = subscription

	// Set timeout and add to heap if subscription doesn't have web socket connection
	if !subscription.hasWSConn() {
		base.expiredAt = time.Now().Add(f.timeout)
		f.timeouts.addSubscription(base)
		f.emitSignalToUpdateCh()
	}

	return base.id
}

// Uninstall removes the subscription with given ID from list
func (f *SubscriptionManager) Uninstall(id string) bool {
	return f.removeSubscriptionByID(id)
}

// nextTimeoutSubscription returns the subscription that will be expired next
// nextTimeoutSubscription returns the only subscription with timeout
func (f *SubscriptionManager) nextTimeoutSubscription() *subscriptionBase {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if len(f.timeouts) == 0 {
		return nil
	}

	// peek the first item
	base := f.timeouts[0]

	return base
}

// dispatchEvent is an event handler for new block event
func (f *SubscriptionManager) dispatchEvent(event *utils.TypeMuxEvent) error {
	if data, ok := event.Data.(utils.TesseractAddedEvent); ok {
		// send data to web socket stream
		if err := f.flushWsSubscriptions(data); err != nil {
			return errors.Wrap(err, "failed to flush websocket subscriptions")
		}
	}

	return nil
}

// flushWsSubscriptions make each subscription with web socket connection write the updates to web socket stream
// and also removes the subscriptions if flushWsSubscriptions notices the connection is closed
func (f *SubscriptionManager) flushWsSubscriptions(data interface{}) error {
	closedSubscriptionIDs := make([]string, 0)

	f.lock.RLock()

	for id, subscription := range f.subscriptions {
		if !subscription.hasWSConn() {
			continue
		}

		if err := subscription.sendUpdate(data); err != nil {
			// mark as closed if the connection is closed
			if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, net.ErrClosed) {
				closedSubscriptionIDs = append(closedSubscriptionIDs, id)

				f.logger.Warn("Subscription ID has been closed", "subscription-ID", id)

				continue
			}

			f.logger.Error("Failed to send update", "err", err)
		}
	}

	f.lock.RUnlock()

	// remove subscriptions with closed web socket connections from SubscriptionManager
	if len(closedSubscriptionIDs) > 0 {
		for _, id := range closedSubscriptionIDs {
			f.removeSubscriptionByID(id)
		}

		f.logger.Info(
			"Removed subscriptions due to closed connections",
			"count", len(closedSubscriptionIDs),
		)
	}

	return nil
}

// Close terminates the worker process
func (f *SubscriptionManager) Close() {
	close(f.closeCh)
}

type Timeouts []*subscriptionBase

func (t *Timeouts) addSubscription(subscription *subscriptionBase) {
	heap.Push(t, subscription)
}

func (t *Timeouts) removeSubscription(subscription *subscriptionBase) bool {
	if subscription.heapIndex == DefaultHeapIndex {
		return false
	}

	heap.Remove(t, subscription.heapIndex)

	return true
}

func (t Timeouts) Len() int { return len(t) }

func (t Timeouts) Less(i, j int) bool {
	return t[i].expiredAt.Before(t[j].expiredAt)
}

func (t Timeouts) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].heapIndex = i
	t[j].heapIndex = j
}

func (t *Timeouts) Push(x interface{}) {
	n := len(*t)
	item := x.(*subscriptionBase) //nolint:forcetypeassert
	item.heapIndex = n
	*t = append(*t, item)
}

func (t *Timeouts) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
	*t = old[0 : n-1]

	return item
}

// subscription is an interface that TesseractSubscription implement
type subscription interface {
	// hasWSConn returns the flag indicating the subscription has web socket stream or not
	hasWSConn() bool

	// getSubscriptionBase returns subscriptionBase that has common fields
	getSubscriptionBase() *subscriptionBase

	// sendUpdate write event data to web socket stream
	sendUpdate(i interface{}) error
}

// subscriptionBase holds the common fields of different subscription types
type subscriptionBase struct {
	// subscription id created with UUID
	id string

	// index in the timeouts heap, -1 for non-existing index
	heapIndex int

	// subscription expiry timestamp
	expiredAt time.Time

	// websocket connection manager
	cm ConnManager
}

// newSubscriptionBase initializes subscriptionBase with unique ID
func newSubscriptionBase(cm ConnManager) subscriptionBase {
	return subscriptionBase{
		id:        uuid.New().String(),
		cm:        cm,
		heapIndex: DefaultHeapIndex,
	}
}

// getSubscriptionBase returns its own reference so that child struct can return base
func (f *subscriptionBase) getSubscriptionBase() *subscriptionBase {
	return f
}

// hasWSConn returns the flag indicating this subscription has websocket connection
func (f *subscriptionBase) hasWSConn() bool {
	if f.cm != nil {
		return f.cm.HasConn()
	}

	return false
}

const subscriptionTemplate = `{
	"jsonrpc": "2.0",
	"method": "moi.subscription",
	"params": {
		"subscription":"%s",
		"result": %s
	}
}`

// writeMessage writes the message to websocket stream
func (f *subscriptionBase) writeMessage(msg string) error {
	res := fmt.Sprintf(subscriptionTemplate, f.id, msg)
	if err := f.cm.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
		return err
	}

	return nil
}

// tesseractAccountSubscription is a subscription to store the updates of tesseract based on address
type tesseractAccountSubscription struct {
	subscriptionBase
	sync.Mutex

	address common.Address
}

// tesseractSubscription is a subscription to store the updates of tesseract
type tesseractSubscription struct {
	subscriptionBase
	sync.Mutex
}

func sendTesseract(tesseract *rpcargs.RPCTesseract, subscriptionBase *subscriptionBase) error {
	res, err := json.Marshal(tesseract)
	if err != nil {
		return errors.Wrap(err, "failed to marshal tesseract")
	}

	if err := subscriptionBase.writeMessage(string(res)); err != nil {
		return errors.Wrap(err, "failed to write the message to the websocket stream")
	}

	return nil
}

// sendUpdates writes the new tesseracts to web socket stream, if the subscriber address and tesseract address matches
func (f *tesseractAccountSubscription) sendUpdate(i interface{}) error {
	if data, ok := i.(utils.TesseractAddedEvent); ok {
		tesseract, err := api.CreateRPCTesseract(data.Tesseract)
		if err != nil {
			return errors.Wrap(err, "failed to create rpc tesseract from tesseract")
		}

		if f.address == data.Tesseract.Address() {
			return sendTesseract(tesseract, &f.subscriptionBase)
		}
	}

	return nil
}

// sendUpdates writes the new tesseracts to web socket stream
func (f *tesseractSubscription) sendUpdate(i interface{}) error {
	if data, ok := i.(utils.TesseractAddedEvent); ok {
		tesseract, err := api.CreateRPCTesseract(data.Tesseract)
		if err != nil {
			return errors.Wrap(err, "failed to create rpc tesseract from tesseract")
		}

		return sendTesseract(tesseract, &f.subscriptionBase)
	}

	return nil
}
