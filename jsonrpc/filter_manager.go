package jsonrpc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
)

var (
	ErrFilterNotFound                   = errors.New("filter not found")
	ErrWSFilterDoesNotSupportGetChanges = errors.New("web socket Filter doesn't support to return a batch of the changes")
	ErrInvalidHeightQuery               = errors.New("Invalid height query")
	ErrInvalidQueryRange                = errors.New("Invalid query range")
	ErrNoWSConnection                   = errors.New("no websocket connection")
	ErrUnknownSubscriptionType          = errors.New("unknown subscription type")
	ErrUnmarshallingData                = errors.New("error unmarshalling data")
)

const (
	// DefaultTimeout is the timeout to remove the filters that don't have a web socket stream
	DefaultTimeout = 1 * time.Minute

	// NoIndexInHeap indicates that the element is not in timeout heap
	NoIndexInHeap = -1
)

type subscriptionType byte

const (
	NewTesseract subscriptionType = iota
	NewTesseractsByAccount
	NewLogsByFilter
	PendingIxns
)

// FilterManager manages all the filters
type FilterManager struct {
	logger              hclog.Logger
	timeout             time.Duration
	lock                sync.RWMutex
	filters             map[string]filter
	timeouts            timeHeapImpl
	updateCh            chan struct{}
	closeCh             chan struct{}
	eventSubscriptions  []*utils.Subscription
	backend             *backend.Backend
	tesseractRangeLimit uint8
}

func NewFilterManager(
	logger hclog.Logger,
	eventMux *utils.TypeMux,
	cfg *config.JSONRPCConfig,
	backend *backend.Backend,
) *FilterManager {
	return &FilterManager{
		logger:              logger.Named("Websocket-Subscription"),
		timeout:             DefaultTimeout,
		filters:             make(map[string]filter),
		timeouts:            timeHeapImpl{},
		updateCh:            make(chan struct{}),
		closeCh:             make(chan struct{}),
		eventSubscriptions:  subscribeToEvents(eventMux),
		backend:             backend,
		tesseractRangeLimit: cfg.TesseractRangeLimit,
	}
}

// subscribeToEvent subscribes to a specific event
func subscribeToEvent(eventMux *utils.TypeMux, event interface{}) *utils.Subscription {
	return eventMux.Subscribe(event)
}

// subscribeToEvents subscribes to both new tesseracts and pending interactions events
func subscribeToEvents(eventMux *utils.TypeMux) []*utils.Subscription {
	events := []interface{}{
		utils.TesseractAddedEvent{},
		utils.AddedInteractionEvent{},
	}

	eventSubscriptions := make([]*utils.Subscription, len(events))

	for i, event := range events {
		eventSubscriptions[i] = subscribeToEvent(eventMux, event)
	}

	return eventSubscriptions
}

// Run starts worker process to handle events
func (f *FilterManager) Run() {
	watchCh := make(chan *utils.TypeMuxEvent)

	// listen to events from all subscribed channels
	for _, eventSubscription := range f.eventSubscriptions {
		// Create a separate goroutine for each subscription
		go func(subscription *utils.Subscription) {
			// TODO: Call stop method on TypeMux.
			// stop method on TypeMux will close subscription channel and exit go routine
			for event := range subscription.Chan() {
				watchCh <- event
			}
		}(eventSubscription)
	}

	var timeoutCh <-chan time.Time

	for {
		// check for the next filter to be removed
		filterID, filterExpiresAt := f.nextTimeoutFilter()

		// set timer to remove filter
		if filterID != "" {
			timeoutCh = time.After(time.Until(filterExpiresAt))
		}

		select {
		case event := <-watchCh:
			if err := f.dispatchEvent(event); err != nil {
				f.logger.Error("Failed to dispatch event", "err", err)
			}

		case <-timeoutCh:
			// timeout for filter
			// if filter still exists
			if !f.Uninstall(filterID) {
				f.logger.Warn("failed to uninstall filter", "id", filterID)
			}

		case <-f.updateCh:
			// filters change, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the filter manager
			return
		}
	}
}

// Close closed closeCh so that terminate worker
func (f *FilterManager) Close() {
	close(f.closeCh)
}

// NewTesseractFilter subscribes to all new tesseract events
func (f *FilterManager) NewTesseractFilter(ws ConnManager) string {
	filter := &tesseractFilter{
		filterBase: newFilterBase(ws),
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// NewTesseractsByAccountFilter subscribes to all new tesseract events for a given account
func (f *FilterManager) NewTesseractsByAccountFilter(ws ConnManager, addr identifiers.Address) string {
	filter := &tesseractByAccountFilter{
		filterBase: newFilterBase(ws),
		address:    addr,
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// NewLogFilter subscribes to all new tesseract log events for a given filter
func (f *FilterManager) NewLogFilter(ws ConnManager, logQuery *LogQuery) string {
	filter := &logFilter{
		filterBase: newFilterBase(ws),
		query:      logQuery,
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// PendingIxnsFilter subscribes to all pending interactions in ixpool
func (f *FilterManager) PendingIxnsFilter(ws ConnManager) string {
	filter := &pendingIxnsFilter{
		filterBase: newFilterBase(ws),
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// GetLogsForQuery return array of logs for given LogQuery
func (f *FilterManager) GetLogsForQuery(query LogQuery) ([]*rpcargs.RPCLog, error) {
	startHeight, err := f.getNumericTesseractNumber(query.StartHeight, query.Address)
	if err != nil {
		return nil, err
	}

	endHeight, err := f.getNumericTesseractNumber(query.EndHeight, query.Address)
	if err != nil {
		return nil, err
	}

	if endHeight < startHeight {
		return nil, ErrInvalidHeightQuery
	}

	if endHeight-startHeight > uint64(f.tesseractRangeLimit) {
		return nil, ErrInvalidQueryRange
	}

	logs := make([]*rpcargs.RPCLog, 0)

	for i := startHeight; i <= endHeight; i++ {
		hash, err := f.getTesseractHashByHeight(query.Address, int64(i))
		if err != nil {
			break
		}

		tesseract, err := f.getTesseractByHash(hash, true)
		if err != nil {
			break
		}

		if len(tesseract.Interactions()) == 0 {
			// no txs in tesseract, return empty response
			continue
		}

		tesseractLogs := f.getLogsFromTesseract(&query, tesseract)

		logs = append(logs, tesseractLogs...)
	}

	return logs, nil
}

// GetFilterChanges returns the updates of the filter with given ID in string,
// and refreshes the timeout on the filter
func (f *FilterManager) GetFilterChanges(id string) (interface{}, error) {
	filter, res, err := f.getFilterAndChanges(id)

	if err == nil {
		// Refresh the timeout on this filter
		f.lock.Lock()
		f.refreshFilterTimeout(filter.getFilterBase())
		f.lock.Unlock()
	}

	return res, err
}

// Uninstall removes the filter with given ID from list
func (f *FilterManager) Uninstall(id string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.removeFilterByID(id)
}

// addFilter is an internal method to add given filter to list and heap
func (f *FilterManager) addFilter(filter filter) string {
	f.lock.Lock()
	defer f.lock.Unlock()

	base := filter.getFilterBase()

	f.filters[base.id] = filter

	// Set timeout and add to heap if filter doesn't have web socket connection
	if !filter.hasWSConn() {
		f.addFilterTimeout(base)
	}

	return base.id
}

// removeFilterByID removes the filter with given ID [NOT Thread Safe]
func (f *FilterManager) removeFilterByID(id string) bool {
	// Make sure filter exists
	filter, ok := f.filters[id]
	if !ok {
		return false
	}

	delete(f.filters, id)

	if removed := f.timeouts.removeFilter(filter.getFilterBase()); removed {
		f.emitSignalToUpdateCh()
	}

	return true
}

// addFilterTimeout set timeout and add to heap
func (f *FilterManager) addFilterTimeout(filter *filterBase) {
	filter.expiresAt = time.Now().UTC().Add(f.timeout)
	f.timeouts.addFilter(filter)
	f.emitSignalToUpdateCh()
}

// nextTimeoutFilter returns the filter that will be expired next
// nextTimeoutFilter returns the only filter with timeout
func (f *FilterManager) nextTimeoutFilter() (string, time.Time) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if len(f.timeouts) == 0 {
		return "", time.Time{}
	}

	// peek the first item
	base := f.timeouts[0]

	return base.id, base.expiresAt
}

// refreshFilterTimeout updates the timeout for a filter to the current time
func (f *FilterManager) refreshFilterTimeout(filter *filterBase) {
	f.timeouts.removeFilter(filter)
	f.addFilterTimeout(filter)
}

func (f *FilterManager) emitSignalToUpdateCh() {
	select {
	// notify worker of new filter with timeout
	case f.updateCh <- struct{}{}:
	default:
	}
}

// dispatchEvent is an event handler for new tesseract and pending ixns events
func (f *FilterManager) dispatchEvent(event interface{}) error {
	switch evnt := event.(*utils.TypeMuxEvent).Data.(type) {
	case utils.TesseractAddedEvent:
		if err := f.processTesseractAddedEvent(&evnt); err != nil {
			return err
		}

	case utils.AddedInteractionEvent:
		if err := f.processPendingIxnsEvent(&evnt); err != nil {
			return err
		}

	default:
		return ErrUnknownSubscriptionType
	}

	return nil
}

// processTesseractAddedEvent listens to all new tesseract events
func (f *FilterManager) processTesseractAddedEvent(event *utils.TesseractAddedEvent) error {
	ts := event.Tesseract

	rpcTesseract, err := rpcargs.CreateRPCTesseract(ts)
	if err != nil {
		return errors.Wrap(err, "failed to create rpc tesseract from tesseract")
	}

	for _, filter := range f.filters {
		// Based on subscriptionType RPC tesseracts or logs are created
		switch filter.getSubscriptionType() {
		case NewLogsByFilter:
			if s, ok := filter.(*logFilter); ok {
				if ts.HasParticipant(s.query.Address) {
					s.appendLog(s.createRPCLogs(ts, rpcTesseract))
				}
			}

			if err := f.flushWsFilters(NewLogsByFilter); err != nil {
				return errors.Wrap(err, "failed to flush websocket subscriptions")
			}

		case NewTesseract:
			if s, ok := filter.(*tesseractFilter); ok {
				s.appendTesseracts(rpcTesseract)
			}

			if err := f.flushWsFilters(NewTesseract); err != nil {
				return errors.Wrap(err, "failed to flush websocket subscriptions")
			}

		case NewTesseractsByAccount:
			if s, ok := filter.(*tesseractByAccountFilter); ok {
				if ts.HasParticipant(s.address) {
					s.appendTesseracts(rpcTesseract)
				}
			}

			if err := f.flushWsFilters(NewTesseractsByAccount); err != nil {
				return errors.Wrap(err, "failed to flush websocket subscriptions")
			}
		}
	}

	return nil
}

func (f *FilterManager) processPendingIxnsEvent(data *utils.AddedInteractionEvent) error {
	f.lock.RLock()
	defer f.lock.RUnlock()

	for _, filter := range f.filters {
		if pendingIxSub, ok := filter.(*pendingIxnsFilter); ok {
			pendingIxSub.appendPendingIxHashes(data.Ixs)
		}
	}

	// send data to web socket stream
	if err := f.flushWsFilters(PendingIxns); err != nil {
		return errors.Wrap(err, "failed to flush websocket subscriptions")
	}

	return nil
}

// flushWsFilters make each filters with web socket connection write the updates to web socket stream
// flushWsFilters also removes the filters if flushWsFilters notices the connection is closed
func (f *FilterManager) flushWsFilters(subType subscriptionType) error {
	closedFilterIDs := make([]string, 0)

	f.lock.RLock()

	for id, filter := range f.filters {
		if !filter.hasWSConn() || filter.getSubscriptionType() != subType {
			continue
		}

		if err := filter.sendUpdates(); err != nil {
			// mark as closed if the connection is closed
			if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, net.ErrClosed) {
				closedFilterIDs = append(closedFilterIDs, id)

				f.logger.Warn("Subscription has been closed", "ID", id)

				continue
			}

			f.logger.Error("Failed to send update", "err", err)
		}
	}

	f.lock.RUnlock()

	// remove filters with closed web socket connections from FilterManager
	if len(closedFilterIDs) > 0 {
		f.lock.Lock()
		defer f.lock.Unlock()

		for _, id := range closedFilterIDs {
			f.removeFilterByID(id)
		}

		f.logger.Info(
			"Removed filters due to closed connections",
			"count", len(closedFilterIDs),
		)
	}

	return nil
}

// getNumericTesseractNumber returns tesseract height based on current state or query height
func (f *FilterManager) getNumericTesseractNumber(height int64, address identifiers.Address) (uint64, error) {
	switch height {
	case rpcargs.LatestTesseractHeight:
		accMeta, err := f.backend.SM.GetAccountMetaInfo(address)
		if err != nil {
			return 0, common.ErrAccountNotFound
		}

		return accMeta.Height, nil
	default:
		if height < rpcargs.LatestTesseractHeight {
			return 0, common.ErrInvalidHeight
		}

		return uint64(height), nil
	}
}

// getTesseractHashByHeight returns the tesseract hash based on tesseract height
func (f *FilterManager) getTesseractHashByHeight(address identifiers.Address, height int64) (common.Hash, error) {
	if height == rpcargs.LatestTesseractHeight {
		accMetaInfo, err := f.backend.SM.GetAccountMetaInfo(address)
		if err != nil {
			return common.NilHash, err
		}

		return accMetaInfo.TesseractHash, nil
	}

	return f.backend.Chain.GetTesseractHeightEntry(address, uint64(height))
}

// getTesseractByHash returns the tesseract based on tesseract hash and with interaction flag
func (f *FilterManager) getTesseractByHash(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
	return f.backend.Chain.GetTesseract(hash, withInteractions)
}

// getLogsFromTesseract filters tesseract logs based on filter topics, and returns logs to API
func (f *FilterManager) getLogsFromTesseract(
	filter *LogQuery,
	ts *common.Tesseract,
) []*rpcargs.RPCLog {
	logs := make([]*rpcargs.RPCLog, 0)

	// participants are declared here to prevent the repetitive creation of the rpc object
	rpcParticipants := rpcargs.CreateRPCParticipants(ts.Participants())
	tsHash := ts.Hash()

	for _, receipt := range ts.Receipts() {
		for _, log := range receipt.Logs {
			if filter.MatchTopics(log) {
				logs = append(logs, &rpcargs.RPCLog{
					Address:      log.Address,
					LogicID:      log.LogicID,
					Topics:       log.Topics,
					Data:         log.Data,
					IxHash:       ts.InteractionsHash(),
					TSHash:       tsHash,
					Participants: rpcParticipants,
				})
			}
		}
	}

	return logs
}

// getFilterAndChanges returns the updates of the filter with given ID in string (read lock only)
func (f *FilterManager) getFilterAndChanges(id string) (filter, interface{}, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	filter, ok := f.filters[id]
	if !ok {
		return nil, nil, ErrFilterNotFound
	}

	// we cannot get updates from a ws filter with getFilterAndChanges
	if filter.hasWSConn() {
		return nil, nil, ErrWSFilterDoesNotSupportGetChanges
	}

	res, err := filter.getUpdates()
	if err != nil {
		return nil, nil, err
	}

	return filter, res, nil
}

// exists checks if filter exists for given ID
func (f *FilterManager) exists(filterID string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	_, ok := f.filters[filterID]

	return ok
}

func sendTesseract(tesseracts []*rpcargs.RPCTesseract, filterBase *filterBase) error {
	for _, ts := range tesseracts {
		res, err := json.Marshal(ts)
		if err != nil {
			return errors.Wrap(err, "failed to marshal tesseract")
		}

		if err := filterBase.writeMessageToWs(string(res)); err != nil {
			return errors.Wrap(err, "failed to write the message to the websocket stream")
		}
	}

	return nil
}

func sendLogs(logs []*rpcargs.RPCLog, filterBase *filterBase) error {
	for _, log := range logs {
		res, err := json.Marshal(log)
		if err != nil {
			return errors.Wrap(err, "failed to marshal receipt")
		}

		if err := filterBase.writeMessageToWs(string(res)); err != nil {
			return errors.Wrap(err, "failed to write the message to the websocket stream")
		}
	}

	return nil
}

// filter is an interface that Tesseract, Log and Pending Ixns Filters implement
type filter interface {
	// hasWSConn returns the flag indicating the filter has web socket stream
	hasWSConn() bool

	// getFilterBase returns filterBase that has common fields
	getFilterBase() *filterBase

	// getSubscriptionType returns the type of the event the filter is subscribed to
	getSubscriptionType() subscriptionType

	// getUpdates returns stored data in a JSON serializable form
	getUpdates() (interface{}, error)

	// sendUpdates write event data to web socket stream
	sendUpdates() error
}

// filterBase holds the common fields of different filter types
type filterBase struct {
	// UUID, a key of filter for client
	id string

	// index in the timeouts heap, -1 for non-existing index
	heapIndex int

	// filter expiry timestamp
	expiresAt time.Time

	// websocket connection manager
	cm ConnManager
}

// newFilterBase initializes filterBase with unique ID
func newFilterBase(cm ConnManager) filterBase {
	return filterBase{
		id:        uuid.New().String(),
		cm:        cm,
		heapIndex: NoIndexInHeap,
	}
}

// getFilterBase returns its own reference so that child struct can return base
func (f *filterBase) getFilterBase() *filterBase {
	return f
}

// hasWSConn returns the flag indicating this filter has websocket connection
func (f *filterBase) hasWSConn() bool {
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

// writeMessageToWs sends given message to websocket stream
func (f *filterBase) writeMessageToWs(msg string) error {
	if !f.hasWSConn() {
		return ErrNoWSConnection
	}

	res := fmt.Sprintf(subscriptionTemplate, f.id, msg)

	if err := f.cm.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
		return err
	}

	return nil
}
