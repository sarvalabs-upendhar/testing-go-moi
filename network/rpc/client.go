package rpc

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-gorpc/stats"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/network/rpc/ttlmap"
)

// ClientOption allows for functional setting of options on a Client.
type ClientOption func(*Client)

func WithLogLevel(level hclog.Level) ClientOption {
	return func(client *Client) {
		client.logger.SetLevel(level)
	}
}

// WithClientStatsHandler provides an implementation of stats.Handler to be
// used by the Client.
func WithClientStatsHandler(h stats.Handler) ClientOption {
	return func(c *Client) {
		c.statsHandler = h
	}
}

// WithMultiStreamBufferSize sets the channel sizes for multiStream calls.
// Reading from the argument channel will proceed as long as none of the
// destinations have filled their buffer. See MultiStream().
func WithMultiStreamBufferSize(size int) ClientOption {
	return func(c *Client) {
		c.multiStreamBufferSize = size
	}
}

type senatus interface {
	GetAddress(key kramaid.KramaID) (multiAddrs []multiaddr.Multiaddr, err error)
}

type ConnectionManager interface {
	NewStream(ctx context.Context, id peer.ID, protocol protocol.ID, tag string) (network.Stream, error)
	SetupStreamHandler(protocolID protocol.ID, tag string, handle func(network.Stream))
	ResetStream(stream network.Stream, tag string) error
	CloseStream(stream network.Stream, tag string) error
	GetAddrsFromPeerStore(peerID peer.ID) []multiaddr.Multiaddr
	AddToPeerStore(peerInfo *peer.AddrInfo)
	GetHostPeerID() peer.ID
}

// Client represents an RPC client which can perform calls to a remote
// (or local, see below) Server.
type Client struct {
	connManager           ConnectionManager
	protocol              protocol.ID
	server                *Server
	statsHandler          stats.Handler
	multiStreamBufferSize int
	senatus               senatus
	streamMap             *ttlmap.Map
	logger                hclog.Logger
}

// NewClient returns a new Client which uses the given p2p connection manager,
// tag and protocol ID, which must match the one used by the server.
// The Host must be correctly configured to be able to open streams
// to the server (addresses and keys in Peerstore etc.).
//
// The client returned will not be able to run any "local" requests (to its
// own peer ID) if a server is configured with the same Host becase libp2p
// hosts cannot open streams to themselves. For this, pass the server directly
// using NewClientWithServer.
func NewClient(
	logger hclog.Logger,
	connManager ConnectionManager,
	p protocol.ID,
	senatus senatus,
	opts ...ClientOption,
) *Client {
	c := &Client{
		connManager: connManager,
		protocol:    p,
		senatus:     senatus,
		logger:      logger.Named("MOIRPC-Client"),
	}

	for _, opt := range opts {
		opt(c)
	}

	// creating a map where key's can have timeout
	options := &ttlmap.Options{
		InitialCapacity: 1024,
		// TTL of a value expired in map
		OnWillExpire: func(key string, item ttlmap.Item) {
		},
		// key is removed from the map
		OnWillEvict: func(key string, item ttlmap.Item) {
			s, ok := (item.Value()).(network.Stream)
			if !ok {
				c.logger.Error("Type assertion failed")
			}
			// closing the stream

			s.Close()
		},
	}
	m := ttlmap.New(options)
	c.streamMap = m

	return c
}

// NewClientWithServer takes an additional RPC Server and returns a Client.
//
// Unlike the normal client, this one will be able to perform any requests to
// itself by using the given directly (and way more efficiently). It is
// assumed that Client and Server share the same libp2p host in this case.
func NewClientWithServer(
	logger hclog.Logger,
	connManager ConnectionManager,
	p protocol.ID,
	senatus senatus,
	s *Server,
	opts ...ClientOption,
) *Client {
	c := NewClient(logger, connManager, p, senatus, opts...)
	c.server = s

	return c
}

// ID returns the peer.ID of the host associated with this client.
func (c *Client) ID() peer.ID {
	if c.connManager == nil {
		return ""
	}

	return c.connManager.GetHostPeerID()
}

// Call performs an RPC call to a registered Server service and blocks until
// completed, returning any errors.
//
// The args parameter represents the service's method args and must be of
// exported or builtin type. The reply type will be used to provide a response
// and must be a pointer to an exported or builtin type. Otherwise a panic
// will occur.
//
// If dest is empty ("") or matches the Client's host ID, it will
// attempt to use the local configured Server when possible.
func (c *Client) Call(
	dest peer.ID,
	svcName, svcMethod string,
	args, reply interface{},
) error {
	ctx := context.Background()

	return c.CallContext(ctx, dest, svcName, svcMethod, args, reply, 0)
}

// MoiCall performs an RPC call to a registered Server service and blocks until
// completed, returning any errors.
// The kramaID parameter represents the unique ID given to each p2p host node
// The args parameter represents the service's method args and must be of
// exported or builtin type. The reply type will be used to provide a response
// and must be a pointer to an exported or builtin type. Otherwise, a panic
// will occur.
func (c *Client) MoiCall(
	ctx context.Context,
	kramaID kramaid.KramaID,
	svcName, svcMethod string,
	args, reply interface{},
	ttl time.Duration,
) error {
	// 1. fetch the peerID from the kramaID
	// 2. check if entry exist in peer store, if entry exist , make the call
	// 3. if entry does not exist in peer store, query senatus to get the multiaddress
	// if we find this, add it to peerstore and  make the rpc call.
	destPeerID, err := kramaID.PeerID()
	if err != nil {
		c.logger.Error("Failed to get peer ID from krama ID", "krama-id", kramaID, "err", err)

		return common.ErrInvalidKramaID
	}

	p, _ := peer.Decode(destPeerID)

	var addrsInPeerStore []multiaddr.Multiaddr
	if !peerIsNil(p) {
		addrsInPeerStore = c.connManager.GetAddrsFromPeerStore(p)
	}

	if len(addrsInPeerStore) == 0 {
		c.logger.Warn("Entry not present in peer store, checking senatus...")
		// check in senatus
		if c.senatus != nil {
			mAddr, err := c.senatus.GetAddress(kramaID)
			if err != nil {
				c.logger.Error("Failed to find address in senatus", "err", err)
			} else {
				c.logger.Info("Entry found in senatus, adding to peer store")
				c.connManager.AddToPeerStore(&peer.AddrInfo{ID: p, Addrs: mAddr})
			}
		} else {
			c.logger.Warn("By-passing senatus")
		}
	}

	return c.CallContext(ctx, p, svcName, svcMethod, args, reply, ttl)
}

func peerIsNil(p peer.ID) bool {
	var nilPeerID peer.ID

	return p == nilPeerID
}

// CallContext performs an RPC call to a registered Server service and blocks
// until completed, returning any errors. It takes a context which can be used
// to abort the call at any point.
//
// The args parameter represents the service's method args and must be of
// exported or builtin type. The reply type will be used to provide a response
// and must be a pointer to an exported or builtin type. Otherwise a panic
// will occur.
//
// If dest is empty ("") or matches the Client's host ID, it will
// attempt to use the local configured Server when possible.
func (c *Client) CallContext(
	ctx context.Context,
	dest peer.ID,
	svcName, svcMethod string,
	args, reply interface{},
	ttl time.Duration,
) error {
	done := make(chan *Call, 1)
	// create a new call object and do authentication of passed value
	call := newCall(c.logger, ctx, dest, svcName, svcMethod, args, reply, done)
	// create a libp2p stream to the destination peer id.
	go c.makeCall(call, ttl)
	<-done

	return call.getError()
}

// Go performs an RPC call asynchronously. The associated Call will be placed
// in the provided channel upon completion, holding any Reply or Errors.
//
// The args parameter represents the service's method args and must be of
// exported or builtin type. The reply type will be used to provide a response
// and must be a pointer to an exported or builtin type. Otherwise a panic
// will occur.
//
// The provided done channel must be nil, or have capacity for 1 element
// at least, or a panic will be triggered.
//
// If dest is empty ("") or matches the Client's host ID, it will
// attempt to use the local configured Server when possible.
func (c *Client) Go(
	dest peer.ID,
	svcName, svcMethod string,
	args, reply interface{},
	done chan *Call,
) error {
	ctx := context.Background()

	return c.GoContext(ctx, dest, svcName, svcMethod, args, reply, done)
}

// GoContext performs an RPC call asynchronously. The provided context can be
// used to cancel the operation. The associated Call will be placed in the
// provided channel upon completion, holding any Reply or Errors.
//
// The args parameter represents the service's method args and must be of
// exported or builtin type. The reply type will be used to provide a response
// and must be a pointer to an exported or builtin type. Otherwise a panic
// will occur.
//
// The provided done channel must be nil, or have capacity for 1 element
// at least, or a panic will be triggered.
//
// If dest is empty ("") or matches the Client's host ID, it will
// attempt to use the local configured Server when possible.
func (c *Client) GoContext(
	ctx context.Context,
	dest peer.ID,
	svcName, svcMethod string,
	args, reply interface{},
	done chan *Call,
) error {
	if done == nil {
		done = make(chan *Call, 1)
	} else if cap(done) == 0 {
		panic("done channel has no capacity")
	}

	call := newCall(c.logger, ctx, dest, svcName, svcMethod, args, reply, done)

	go c.makeCall(call, 0)

	return nil
}

// MultiCall performs a CallContext() to multiple destinations, using the same
// service name, method and arguments. It will not return until all calls have
// done so. The contexts, destinations and replies must match in length and
// will be used in order (ctxs[i] is used for dests[i] which obtains
// replies[i] and error[i]).
//
// The calls will be triggered in parallel (with one goroutine for each).
func (c *Client) MultiCall(
	ctxs []context.Context,
	dests []peer.ID,
	svcName, svcMethod string,
	args interface{},
	replies []interface{},
) []error {
	ok := checkMatchingLengths(
		len(ctxs),
		len(dests),
		len(replies),
	)

	if !ok {
		panic("ctxs, dests and replies must match in length")
	}

	var wg sync.WaitGroup

	errs := make([]error, len(dests))

	for i := range dests {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			err := c.CallContext(
				ctxs[i],
				dests[i],
				svcName,
				svcMethod,
				args,
				replies[i], 0)
			errs[i] = err
		}(i)
	}

	wg.Wait()

	return errs
}

// MultiGo performs a GoContext() call to multiple destinations, using the same
// service name, method and arguments. MultiGo will return as right after
// performing all the calls. See the Go() documentation for more information.
//
// The provided done channels must be nil, or have capacity for 1 element
// at least, or a panic will be triggered.
//
// The contexts, destinations, replies and done channels  must match in length
// and will be used in order (ctxs[i] is used for dests[i] which obtains
// replies[i] with dones[i] signalled upon completion).
func (c *Client) MultiGo(
	ctxs []context.Context,
	dests []peer.ID,
	svcName, svcMethod string,
	args interface{},
	replies []interface{},
	dones []chan *Call,
) error {
	ok := checkMatchingLengths(
		len(ctxs),
		len(dests),
		len(replies),
		len(dones),
	)
	if !ok {
		panic("ctxs, dests, replies and dones must match in length")
	}

	for i := range ctxs {
		err := c.GoContext(
			ctxs[i],
			dests[i],
			svcName,
			svcMethod,
			args,
			replies[i],
			dones[i],
		)
		if err != nil {
			c.logger.Error("Error in Go-context", "err", err)
		}
	}

	return nil
}

// Stream performs a streaming RPC call. It receives two arguments which both
// must be channels of exported or builtin types. The first is a channel from
// which objects are read and sent on the wire. The second is a channel to
// receive the replies from. Calling with the wrong types will cause a panic.
//
// The sending channel should be closed by the caller for successful
// completion of the call. The replies channel is closed by us when there is
// nothing else to receive (call finished or an error happened). The sending
// channel is drained in the background in case of error, so it is recommended
// that senders diligently close when an error happens to be able to free
// resources.
//
// The function only returns when the operation is finished successful (both
// channels are closed) or when an error has occurred.
func (c *Client) Stream(
	ctx context.Context,
	dest peer.ID,
	svcName, svcMethod string,
	argsChan interface{},
	repliesChan interface{},
	tag string,
) error {
	vArgsChan := reflect.ValueOf(argsChan)
	vRepliesChan := reflect.ValueOf(repliesChan)

	done := make(chan *Call, 1)
	call := newStreamingCall(c.logger, ctx, dest, svcName, svcMethod, vArgsChan, vRepliesChan, done, tag)

	go c.makeStream(call)
	<-done

	return call.getError()
}

// MultiStream performs parallel Stream() calls to multiple peers using a
// single arguments channel for arguments and a single replies channel that
// aggregates all replies. Errors from each destination are provided in the
// response. Channel types should be exported or builtin, otherwise a panic
// will be triggered.
//
// In order to replicate the argsChan values to several destinations and sed
// the replies into a single channel, intermediary channels for each call are
// created. These channels are buffered per the WithMultiStreamBufferSize()
// option. If the buffers is exausted for one of the sending or the receiving
// channels, the sending or receiving stalls. Therefore it is recommended to
// have enough buffering to allow that slower destinations do not delay
// everyone else.
func (c *Client) MultiStream(
	ctx context.Context,
	dests []peer.ID,
	svcName, svcMethod string,
	argsChan interface{},
	repliesChan interface{},
	tag string,
) []error {
	n := len(dests)
	sID := ServiceID{svcName, svcMethod}

	vArgsChan := reflect.ValueOf(argsChan)
	vRepliesChan := reflect.ValueOf(repliesChan)
	argsChanType := reflect.TypeOf(argsChan)
	repliesChanType := reflect.TypeOf(repliesChan)

	checkChanTypesValid(sID, vArgsChan, reflect.RecvDir)
	checkChanTypesValid(sID, vRepliesChan, reflect.SendDir)

	// Make slices of N channels of the same type as the argsChan and
	// repliesChan. We will use them for every Stream() call. They are
	// buffered per multiStreamBufferSize.
	vArgsChannels := makeChanSliceOf(argsChanType, n, c.multiStreamBufferSize)
	vRepliesChannels := makeChanSliceOf(repliesChanType, n, c.multiStreamBufferSize)

	// Make slices of contexts and cancels. We will use them to cancel
	// sending to channels when a Stream() call has failed.
	teeCtxs := make([]context.Context, n)
	teeCancels := make([]context.CancelFunc, n)

	for i := 0; i < n; i++ {
		teeCtxs[i], teeCancels[i] = context.WithCancel(ctx)
	}

	// To hold responses.
	errs := make([]error, n)

	var wg sync.WaitGroup

	// First, launch N stream calls to every destination using the
	// channels we created and everything else provided by the caller.
	// Collect errors in errs, and if they happen, cancel associated
	// context.
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()

			err := c.Stream(
				ctx,
				dests[i],
				svcName,
				svcMethod,
				vArgsChannels.Index(i).Interface(),
				vRepliesChannels.Index(i).Interface(),
				tag,
			)

			errs[i] = err

			if err != nil {
				teeCancels[i]() // cancel context for this so that we close the send channel
			}
		}(i)
	}

	// Second, "tee" anything received from the argsChan into the channels
	// we created.
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Close all sending channels when done.
		defer func() {
			for i := 0; i < n; i++ {
				vArgsChannels.Index(i).Close()
			}
		}()

		// Make a select with N cases (one per channel).
		cases := make([]reflect.SelectCase, n)
		for i := range cases {
			cases[i].Dir = reflect.SelectSend
		}

		// While there is something to receive from the argsChan.
		for {
			arg, ok := vArgsChan.Recv()
			// We continue reading from the channel.
			// And repeat until no valid cases left.
			// otherwise we continue just draining the channel
			if !ok {
				return
			}

			// Setup cases. If the context for a channel has not
			// been cancelled, prepare a send of the argument we
			// read from argsChan. Otherwise set Chan to nil
			// (effectively disables that case).
			validCases := 0

			for i := range cases {
				cases[i].Send = arg
				if teeCtxs[i].Err() == nil {
					cases[i].Chan = vArgsChannels.Index(i)
					validCases++
				} else {
					cases[i].Chan = reflect.ValueOf(nil)
				}
			}

			if validCases == 0 { // all our ops failed.
				go drainChannel(vArgsChan)

				return
			}

			// Make a select for all cases and call it
			// "validCases" times. This puts the arg value in all
			// still available channels, potentially blocking if
			// one of those channels has no buffer left.
			for i := 0; i < validCases; i++ {
				chosen, _, _ := reflect.Select(cases)
				cases[chosen].Chan = reflect.ValueOf(nil) // ignore this case
			}
		}
	}()

	// Third, "multiplex" anything received from the argsChan into the
	// channels we created.
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Make a select with N cases (one per channel).
		cases := make([]reflect.SelectCase, n)
		for i := range cases {
			cases[i].Dir = reflect.SelectRecv
			cases[i].Chan = vRepliesChannels.Index(i)
		}

		validChannels := n
		for validChannels > 0 {
			chosen, v, ok := reflect.Select(cases)
			if !ok {
				cases[chosen].Chan = reflect.ValueOf(nil)
				validChannels--

				continue
			}

			vRepliesChan.Send(v)
		}

		// Close the response channels.
		vRepliesChan.Close()
	}()

	// Wait for everyone to finish.
	wg.Wait()

	return errs
}

func makeChanSliceOf(typ reflect.Type, capacity int, buffer int) reflect.Value {
	chanSlice := reflect.MakeSlice(reflect.SliceOf(reflect.ChanOf(reflect.BothDir, typ.Elem())), 0, capacity)
	for i := 0; i < capacity; i++ {
		chanSlice = reflect.Append(
			chanSlice,
			reflect.MakeChan(reflect.ChanOf(reflect.BothDir, typ.Elem()), buffer),
		)
	}

	return chanSlice
}

// returns true if all arguments are the same number.
func checkMatchingLengths(l ...int) bool {
	if len(l) <= 1 {
		return true
	}

	for i := 1; i < len(l); i++ {
		if l[i-1] != l[i] {
			return false
		}
	}

	return true
}

// makeCall decides if a call can be performed. If it's a local
// call it will use the configured server if set.
func (c *Client) makeCall(call *Call, ttl time.Duration) {
	c.logger.Trace("Make call", "service-ID", call.SvcID)

	// Handle local RPC calls
	if call.Dest == "" || c.connManager == nil || call.Dest == c.connManager.GetHostPeerID() {
		c.logger.Debug("Local call", "service-ID", call.SvcID)

		if c.server == nil {
			err := &clientError{"Cannot make local calls: server not set"}
			call.doneWithError(err)

			return
		}

		err := c.server.serverCall(call)
		call.doneWithError(err)

		return
	}

	// Handle remote RPC calls
	if c.connManager == nil {
		panic("no host set: cannot perform remote call")
	}

	if c.protocol == "" {
		panic("no protocol set: cannot perform remote call")
	}

	if _, err := c.send(call, ttl); err != nil {
		c.logger.Error("Error in send", "err", err)
	}
}

// send makes a REMOTE RPC call by initiating a libp2p stream to the
// destination and waiting for a response.
func (c *Client) send(call *Call, ttl time.Duration) (network.Stream, error) {
	var stream network.Stream

	item, err := c.streamMap.Get(call.Dest.String())

	if err != nil {
		ns, err := c.connManager.NewStream(call.ctx, call.Dest, c.protocol, call.tag)
		if err != nil {
			call.doneWithError(newClientError(err))

			return nil, err
		}

		stream = ns
	} else {
		s, ok := (item.Value()).(network.Stream)
		if !ok {
			c.logger.Error("network.Stream type assertion failed")
		}
		stream = s
	}

	go call.watchContextWithStream(stream)

	sWrap := wrapStream(stream)

	if err := sWrap.enc.Encode(call.SvcID); err != nil {
		call.doneWithError(newClientError(err))
		c.logger.Error(
			"Client error related to encode service ID",
			"err", err,
			"service-ID", call.SvcID,
		)

		_ = c.connManager.ResetStream(stream, call.tag)

		return nil, err
	}

	// In this case, we have a single argument in the channel.
	if err := sWrap.enc.Encode(call.Args); err != nil {
		call.doneWithError(newClientError(err))
		c.logger.Error(
			"Client error related to encoding argument",
			"err", err,
			"encode-args", call.Args,
		)

		if resetErr := c.connManager.ResetStream(stream, call.tag); resetErr != nil {
			call.logger.Trace("Failed to close stream", "err", resetErr)
		}

		return nil, err
	}

	sWrap.w.Flush()

	if err := sWrap.w.Flush(); err != nil {
		call.doneWithError(newClientError(err))
		c.logger.Error("Client error related to encoding flush", "err", err)

		_ = c.connManager.ResetStream(stream, call.tag)

		return nil, err
	}

	err = receiveResponse(c.logger, sWrap, call)

	if err != nil {
		c.logger.Error("Client received response error", "stream-ID", sWrap.stream.ID(), "err", err)

		_ = c.connManager.ResetStream(stream, call.tag)

		return nil, err
	}
	// choosing not to close the stream and cache it
	// if withTTL value is provided
	// s.Close()
	// TO-DO cache the stream in TTL-Map
	// TTL for 1000 milli seconds for testing,
	// this will be replaced by TTL specified by the user' making
	// the RPC call.
	if ttl != 0 {
		_, err = c.streamMap.Get(call.Dest.String())
		if err != nil {
			c.logger.Trace("Adding peer ID in the TTL map", "ID", call.Dest.String())

			setErr := c.streamMap.Set(call.Dest.String(), ttlmap.NewItem(stream, ttlmap.WithTTL(ttl)), nil)

			if setErr != nil {
				call.logger.Error("Failed to set error", "err", setErr)
			}
		}
	} else {
		if closeErr := c.connManager.CloseStream(stream, call.tag); closeErr != nil {
			call.logger.Trace("Failed to close stream", "err", closeErr)
		}
	}

	return stream, nil
}

// receiveResponse reads a response to an RPC call
func receiveResponse(logger hclog.Logger, s *streamWrap, call *Call) error {
	var resp Response

	if err := s.dec.Decode(&resp); err != nil {
		logger.Error("Client error while decoding response", "err", err)
		call.doneWithError(newClientError(err))

		return err
	}

	if e := resp.Error; e != "" {
		logger.Error("Client error in response", "response-error", resp.Error)

		err := responseError(resp.ErrType, e)
		// we still try to read the body if possible
		call.setError(err)
	}

	if err := s.dec.Decode(call.Reply); err != nil && !errors.Is(err, io.EOF) {
		logger.Error("Client error while decoding reply", "err", err)
		call.doneWithError(newClientError(err))

		return err
	}

	call.done()

	return nil
}

// makeSteram performs a streaming call, either local or remote.
func (c *Client) makeStream(call *Call) {
	c.logger.Trace("Streaming call service ID", "service-ID", call.SvcID)

	// Handle local RPC calls
	if call.Dest == "" || c.connManager == nil || call.Dest == c.connManager.GetHostPeerID() {
		c.logger.Debug("Handling local RPC call", "service-ID", call.SvcID)

		if c.server == nil {
			err := &clientError{"Cannot make local calls: server not set"}
			call.doneWithError(err)

			return
		}

		err := c.server.serverStream(call)
		if err != nil {
			go drainChannel(call.StreamArgs)
		}

		call.doneWithError(err)

		return
	}

	// Handle remote RPC calls
	if c.connManager == nil {
		panic("no host set: cannot perform remote call")
	}

	if c.protocol == "" {
		panic("no protocol set: cannot perform remote call")
	}

	c.stream(call)
}

// stream makes a REMOTE RPC streaming call by initiating a libp2p stream to
// the destination, writing argument channel objects to it and reading the
// replies to the replies channel.
func (c *Client) stream(call *Call) {
	s, err := c.connManager.NewStream(call.ctx, call.Dest, c.protocol, call.tag)
	if err != nil {
		call.doneWithError(newClientError(err))

		go drainChannel(call.StreamArgs)
		call.StreamReplies.Close()

		return
	}

	go call.watchContextWithStream(s)
	sWrap := wrapStream(s)

	// Send the service ID first. This may return an authorization error
	// for example.
	c.logger.Debug("Sending stream-RPC with", "service-ID", call.SvcID, "dest", call.Dest)

	if err := sWrap.enc.Encode(call.SvcID); err != nil {
		call.doneWithError(newClientError(err))

		_ = c.connManager.ResetStream(s, call.tag)

		go drainChannel(call.StreamArgs)
		call.StreamReplies.Close()

		return
	}

	// Flush that so that we become ready to read on the other side.
	if err := sWrap.w.Flush(); err != nil {
		call.doneWithError(newClientError(err))

		_ = c.connManager.ResetStream(s, call.tag)

		go drainChannel(call.StreamArgs)
		call.StreamReplies.Close()

		return
	}

	// Now we need to start writing arguments and reading.
	// Our context watcher will close the streams, we can therefore
	// not worry about contexts closures in our goroutines.
	var wg sync.WaitGroup

	wg.Add(2)

	// This goroutine sends things on the wire. It reads from the
	// arguments channel, encodes the object and flushes it.
	// Repeat until done or error.
	// Arguments channel is drained on error.
	go func() {
		// Close stream for writing when done
		defer wg.Done()
		defer func() {
			closeWriteErr := s.CloseWrite()
			if closeWriteErr != nil {
				c.logger.Error("Failed to close write stream", "err", closeWriteErr)
			}
		}()

		for {
			v, ok := call.StreamArgs.Recv()
			if !ok { // closed channel
				return
			}

			if err := sWrap.enc.Encode(v); err != nil {
				call.doneWithError(newClientError(err))
				// closing the args channel is responsibility
				// of the sender.

				_ = c.connManager.ResetStream(s, call.tag)

				go drainChannel(call.StreamArgs)

				return
			}

			// Flush it
			if err := sWrap.w.Flush(); err != nil {
				call.doneWithError(newClientError(err))

				_ = c.connManager.ResetStream(s, call.tag)

				go drainChannel(call.StreamArgs)

				return
			}
		}
	}()

	// This goroutine receives things from the wire.  First it reads a
	// Response (response can be considered reply "headers"). If it is an
	// error, then it aborts. Then it reads a reply object and sends it on
	// the reply channel.
	go func() {
		defer wg.Done()
		defer call.StreamReplies.Close()
		defer func() {
			closeReadErr := s.CloseRead()
			if closeReadErr != nil {
				c.logger.Error("Error closing read stream", "err", closeReadErr)
			}
		}()

		for {
			var resp Response

			err := sWrap.dec.Decode(&resp)
			if errors.Is(err, io.EOF) {
				return
			}

			if err != nil {
				call.setError(newClientError(err))

				_ = c.connManager.ResetStream(s, call.tag)

				return
			}

			if resp.Error != "" {
				call.setError(responseError(resp.ErrType, resp.Error))

				_ = c.connManager.ResetStream(s, call.tag)

				return
			}

			// Now decode the data
			reply := reflect.New(call.StreamReplies.Type().Elem()).Elem().Interface()
			err = sWrap.dec.Decode(&reply)

			if err != nil {
				call.setError(newClientError(err))

				_ = c.connManager.ResetStream(s, call.tag)

				return
			}
			// Put element
			call.StreamReplies.Send(reflect.ValueOf(reply))
		}
	}()

	// Wait for send/receive routines to finish, cleanup and then signal
	// finalization of the Call.
	wg.Wait()

	if closeErr := c.connManager.CloseStream(s, call.tag); closeErr != nil {
		call.logger.Trace("Failed to close stream", "err", closeErr)
	}

	call.done()
}
