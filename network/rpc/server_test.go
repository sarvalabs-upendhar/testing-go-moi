package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/go-moi/common"
)

var testLogger = hclog.NewNullLogger()

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type ctxTracker struct {
	ctxMu sync.Mutex
	ctx   context.Context
}

type senatusMock struct {
	AddrMap map[kramaid.KramaID][]multiaddr.Multiaddr
}

func (sm senatusMock) GetAddress(key kramaid.KramaID) (multiAddrs []multiaddr.Multiaddr, err error) {
	v, ok := sm.AddrMap[key]
	if !ok {
		return nil, common.ErrKeyDoNotExist
	}

	return v, nil
}

func (sm *senatusMock) SetAddress(key kramaid.KramaID, mAddrs []multiaddr.Multiaddr) error {
	if len(sm.AddrMap) == 0 {
		sm.AddrMap = make(map[kramaid.KramaID][]multiaddr.Multiaddr)
	}

	sm.AddrMap[key] = mAddrs

	return nil
}

func (ctxt *ctxTracker) setCtx(ctx context.Context) {
	if ctxt == nil {
		return
	}

	ctxt.ctxMu.Lock()
	defer ctxt.ctxMu.Unlock()
	ctxt.ctx = ctx
}

func (ctxt *ctxTracker) cancelled() bool {
	if ctxt == nil {
		return false
	}

	ctxt.ctxMu.Lock()
	defer ctxt.ctxMu.Unlock()

	return ctxt.ctx.Err() != nil
}

type Arith struct {
	ctxTracker *ctxTracker
}

func (t *Arith) Multiply(ctx context.Context, args *Args, reply *int) error {
	*reply = args.A * args.B

	return nil
}

// This uses non pointer args
func (t *Arith) Add(ctx context.Context, args Args, reply *int) error {
	*reply = args.A + args.B

	return nil
}

func (t *Arith) Divide(ctx context.Context, args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}

	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B

	return nil
}

func (t *Arith) GimmeError(ctx context.Context, args *Args, r *int) error {
	*r = 42

	return errors.New("an error")
}

func (t *Arith) Sleep(ctx context.Context, secs int, res *struct{}) error {
	t.ctxTracker.setCtx(ctx)

	tim := time.NewTimer(time.Duration(secs) * time.Second)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tim.C:
		return nil
	}
}

func (t *Arith) PrintHelloWorld(ctx context.Context, args struct{}, res *struct{}) error {
	t.ctxTracker.setCtx(ctx)

	fmt.Print("hello world!")

	return nil
}

func (t *Arith) DivideMyNumbers(ctx context.Context, args <-chan Args, quotients chan<- Quotient) error {
	t.ctxTracker.setCtx(ctx)

	for arg := range args {
		if arg.A == 1234 {
			time.Sleep(time.Second)
		}

		if arg.A == 666 {
			// it does not close(quotients) on purpose
			return errors.New("bad bad bad")
		}

		select {
		case <-ctx.Done():
			close(quotients)

			return ctx.Err()
		default:
		}

		if arg.B == 0 {
			close(quotients)

			return errors.New("divide by zero")
		}

		quo := Quotient{
			Quo: arg.A / arg.B,
			Rem: arg.A % arg.B,
		}
		quotients <- quo
	}

	close(quotients)

	return nil
}

func (t *Arith) DivideMyNumbersPointers(ctx context.Context, args <-chan *Args, quotients chan<- *Quotient) error {
	defer close(quotients)

	for arg := range args {
		if arg.B == 0 {
			return errors.New("divide by zero")
		}

		quo := Quotient{
			Quo: arg.A / arg.B,
			Rem: arg.A % arg.B,
		}
		quotients <- &quo
	}

	return nil
}

func createP2PHosts(t *testing.T) (cm1, cm2 *MockConnectionManager) {
	t.Helper()

	cm1 = NewMockConnectionManager("/ip4/127.0.0.1/tcp/19998")
	cm2 = NewMockConnectionManager("/ip4/127.0.0.1/tcp/19999")

	isH1InH2 := cm2.host.Peerstore().Addrs(cm1.host.ID())
	assert.Empty(t, isH1InH2)

	isH2InH1 := cm1.host.Peerstore().Addrs(cm2.host.ID())
	assert.Empty(t, isH2InH1)

	return
}

func TestRegister(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)

	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")

	var arith Arith

	err := s.Register(arith)

	if err == nil {
		t.Error("expected an error")
	}

	err = s.Register(&arith)
	if err != nil {
		t.Error(err)
	}
	// Re-register
	err = s.Register(&arith)
	if err == nil {
		t.Error("expected an error")
	}
}

func testCall(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Fatal(err)
	}

	var q Quotient

	err = c.MoiCall(context.Background(), kramaid.KramaID(getKramaID(dest)), "Arith", "Divide", &Args{20, 6}, &q, 0)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}
}

func testRPCCallToSameDestinationMultipleSource(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Fatal(err)
	}

	var q Quotient

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",
		&Args{20, 6},
		&q,
		200*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",
		&Args{20, 6},
		&q,
		200*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",
		&Args{20, 6},
		&q,
		200*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",
		&Args{20, 6},
		&q,
		200*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}

	time.Sleep(500 * time.Millisecond)

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",
		&Args{20, 6},
		&q,
		200*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}
}

func testCallWithSenatus(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")

	sm := senatusMock{}

	err := sm.SetAddress(kramaid.KramaID(getKramaID(dest)), serverCM.host.Addrs())
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(testLogger, clientCM, "rpc", sm)

	var arith Arith

	err = s.Register(&arith)
	if err != nil {
		t.Fatal(err)
	}

	var q Quotient

	err = c.MoiCall(
		context.Background(),
		kramaid.KramaID(getKramaID(dest)),
		"Arith",
		"Divide",

		&Args{20, 6},
		&q,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}
}

// testing for invalid kramaID
func testValidKramaID(t *testing.T, dest peer.ID) {
	t.Helper()

	kID := kramaid.KramaID(getKramaID(dest))
	_, err := kID.PeerID()
	assert.NoError(t, err)
}

func testInValidKramaID(t *testing.T, dest peer.ID) {
	t.Helper()

	kID := kramaid.KramaID(getInValidKramaID(dest))
	_, err := kID.PeerID()
	assert.Error(t, err)
}

func getKramaID(dest peer.ID) string {
	moiIDAddr := "232fe27479B226B6d7AEe26f4Fcd70F55f3EC453"
	data := []byte(moiIDAddr)
	encoded := base58.Encode(data)
	kramaID := encoded + "." + dest.String()

	return kramaID
}

func getInValidKramaID(dest peer.ID) string {
	moiIDAddr := "232fe27479B226B6d7AEe26f4Fcd70F55f3EC453"
	kramaID := moiIDAddr + "." + string(dest)

	return kramaID
}

func TestCall(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		testCall(t, cm[0], cm[1], cm[0].host.ID())
	})
}

// TestRpcCallToSameDestinationMultipleSource
// This test tries to make multiple Rpc call (5 ) to same destination
// from multiple sources (3)
func TestRpcCallToSameDestinationMultipleSource(t *testing.T) {
	cm := createConnectionMangers(t, 4)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		// h2 --> h1
		testRPCCallToSameDestinationMultipleSource(t, cm[0], cm[1], cm[0].host.ID())
		// h3 --> h1
		testRPCCallToSameDestinationMultipleSource(t, cm[0], cm[2], cm[0].host.ID())
		// h4 ---> h1
		testRPCCallToSameDestinationMultipleSource(t, cm[0], cm[3], cm[0].host.ID())
	})
}

// TestStreamReusewithTTL tries to reuse stream to make RCP call
func TestStreamReusewithTTL(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		// h2 --> h1
		testStreamReusewithTTL(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func testStreamReusewithTTL(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Fatal("falied to Register")
	}

	ctx := context.Background()

	var q Quotient

	done := make(chan *Call, 1)
	// create a new call object and do authentication of passed value
	call1 := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)
	// create a libp2p stream to the destination peer id.

	t.Log("##### 1 ##### Calling send the first time")

	stream1, err := c.send(call1, time.Millisecond*300)
	assert.NoError(t, err)

	streamID1 := stream1.ID()

	t.Log("[testStreamReusewithTTL]", "stream-ID 1", streamID1)

	call2 := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)

	t.Log("##### 2 ##### Calling send the second time")

	stream2, err := c.send(call2, time.Millisecond*300)
	assert.NoError(t, err)

	streamID2 := stream2.ID()
	t.Log("[testStreamReusewithTTL]", "stream-ID 2", streamID2)

	assert.Equal(t, streamID1, streamID2)
	t.Cleanup(func() {
		_, err = c.streamMap.Delete(dest.String())
		if err != nil {
			t.Fatal("failed to Delete")
		}
	})
}

// TestNewStreamAfterTTLTimeout wait for enough time
// so that first stream cache time out and second RPC
// call creates a new stream to make the RPC call
func TestNewStreamAfterTTLTimeout(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		testNewStreamAfterTTLTimeout(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func testNewStreamAfterTTLTimeout(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()

	var q Quotient

	done := make(chan *Call, 1)
	// create a new call object and do authentication of passed value
	call1 := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)
	// create a libp2p stream to the destination peer id.

	t.Log("##### 1 ##### Calling send the first time")

	stream1, err := c.send(call1, time.Millisecond*300)
	assert.NoError(t, err)

	streamID1 := stream1.ID()
	t.Log("[testNewStreamAfterTTLTimeout]", "stream-ID 1", streamID1)

	// sleeping for enough time so that stream1 TTL time out
	time.Sleep(time.Second)

	call2 := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)

	t.Log("##### 2 ##### Calling send the second time")

	stream2, err := c.send(call2, time.Millisecond*300)
	assert.NoError(t, err)

	streamID2 := stream2.ID()
	t.Log("[testNewStreamAfterTTLTimeout]", "stream-ID 2", streamID2)

	assert.NotEqual(t, streamID1, streamID2)
	t.Cleanup(func() {
		_, err = c.streamMap.Delete(dest.String())
		if err != nil {
			t.Fatal("failed to Delete")
		}
	})
}

// TestStreamwithZeroTTL
// This test should create two streams when making two RPC calls
func TestStreamwithTTL(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("remote1", func(t *testing.T) {
		testStreamwithZeroTTL(t, cm[0], cm[1], cm[0].host.ID())
	})

	t.Run("remote2", func(t *testing.T) {
		testStreamwithNonZeroTTL(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func testStreamwithZeroTTL(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()

	var q Quotient

	done := make(chan *Call, 1)
	// create a new call object and do authentication of passed value
	call1 := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)
	// create a libp2p stream to the destination peer id.

	stream1, err := c.send(call1, 0)
	assert.NoError(t, err)

	streamID1 := stream1.ID()
	t.Log("[testStreamwithZeroTTL]", " stream-ID 1", streamID1)
	<-done

	_, err = c.streamMap.Get(dest.String())
	assert.Error(t, err)

	t.Cleanup(func() {
		_, err = c.streamMap.Delete(dest.String())
		assert.Error(t, err)
	})
}

func testStreamwithNonZeroTTL(t *testing.T, serverCM, clientCM ConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()

	var q Quotient

	done := make(chan *Call, 1)
	// create a new call object and do authentication of passed value
	call := newCall(testLogger, ctx, dest, "Arith", "Divide", &Args{20, 6}, &q, done)
	// create a libp2p stream to the destination peer id.

	stream, err := c.send(call, 10*time.Millisecond)
	assert.NoError(t, err)

	streamID := stream.ID()
	t.Log("[testStreamwithNonZeroTTL]", "stream-ID 1", streamID)

	<-done

	_, err = c.streamMap.Get(dest.String())
	assert.NoError(t, err)

	t.Cleanup(func() {
		_, err = c.streamMap.Delete(dest.String())
		if err != nil {
			t.Error("failed to delete")
		}
	})
}

func TestValidKramaID(t *testing.T) {
	cm := createConnectionMangers(t, 1)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		testValidKramaID(t, cm[0].host.ID())
	})
}

func TestInvalidKramaID(t *testing.T) {
	cm := createConnectionMangers(t, 1)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		testInValidKramaID(t, cm[0].host.ID())
	})
}

func TestNonExistingPeerIDInPeerStore(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("remote", func(t *testing.T) {
		testCallWithSenatus(t, cm[0], cm[1], cm[0].host.ID())
		isH1InH2 := cm[1].host.Peerstore().Addrs(cm[1].host.ID())
		assert.NotEmpty(t, isH1InH2)
	})
}

func createDht(nodehost host.Host, protocolID string) (*dht.IpfsDHT, error) {
	// Create DHT server mode option
	dhtmode := dht.Mode(dht.ModeServer)

	// Trace log
	testLogger.Trace("Generated DHT configuration")

	// Start a Kademlia DHT on the host in server mode
	kaddht, err := dht.New(context.Background(), nodehost, dhtmode, dht.ProtocolPrefix(protocol.ID(protocolID)))
	// Handle any potential error
	if err != nil {
		testLogger.Error("Failed to create the Kademlia DHT", "err", err)
		log.Fatal(err)

		return nil, err
	}

	return kaddht, nil
}

func TestNonExistingPeerIDWithDht(t *testing.T) {
	protocolID := "moi_protocol_discovery_id_network_test"

	// initializing bootstrap p2p host
	bootstrapnode, _ := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/19991"),
	)

	bootstrapnodeMultiAddr := bootstrapnode.Addrs()
	// bootstrap node AddressInfo
	bootstrapnodeAddrInfo := new(peer.AddrInfo)
	bootstrapnodeAddrInfo.ID = bootstrapnode.ID()
	bootstrapnodeAddrInfo.Addrs = bootstrapnodeMultiAddr

	// dht for bootstrap node
	_, err := createDht(bootstrapnode, protocolID)
	assert.NoError(t, err)

	cm1, cm2 := createP2PHosts(t)
	defer stopConnectionManagers(t, []*MockConnectionManager{cm1, cm2})

	// dht for node 1
	_, err = createDht(cm1.host, protocolID)
	assert.NoError(t, err)

	// dht for node 2
	_, err = createDht(cm2.host, protocolID)
	assert.NoError(t, err)

	t.Run("remote", func(t *testing.T) {
		// connecting node 1 with bootstrap node
		err = cm1.host.Connect(context.Background(), *bootstrapnodeAddrInfo)
		assert.NoError(t, err)

		// connecting node 2 with bootstrap node
		err = cm2.host.Connect(context.Background(), *bootstrapnodeAddrInfo)
		assert.NoError(t, err)

		time.Sleep(time.Second * 2)
		// testing if rpc call go through
		testCall(t, cm1, cm2, cm2.host.ID())

		isH1InH2 := cm2.host.Peerstore().Addrs(cm1.host.ID())
		assert.NotEmpty(t, isH1InH2)

		isH2InH1 := cm1.host.Peerstore().Addrs(cm2.host.ID())
		assert.NotEmpty(t, isH2InH1)
	})
}

func TestErrorResponse(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	t.Run("remote", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
		if err == nil || err.Error() != "an error" {
			t.Error("expected different error")
		}
		if r != 42 {
			t.Error("response should be set even on error")
		}
	})

	t.Run("local", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
		if err == nil || err.Error() != "an error" {
			t.Error("expected different error")
		}
		if r != 42 {
			t.Error("response should be set even on error")
		}
	})
}

func TestNonRPCError(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	t.Run("local non rpc error", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
		if err != nil {
			if IsRPCError(err) {
				t.Log(err)
				t.Error("expected non rpc error")
			}
		}
	})

	t.Run("local rpc error", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "ThisIsNotAMethod", &Args{1, 2}, &r)
		if err != nil {
			if !IsRPCError(err) {
				t.Log(err)
				t.Error("expected rpc error")
			}
		}
	})

	t.Run("remote non rpc error", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
		if err != nil {
			if IsRPCError(err) {
				t.Log(err)
				t.Error("expected non rpc error")
			}
		}
	})

	t.Run("remote rpc error", func(t *testing.T) {
		var r int
		c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)
		err := c.Call(cm[0].host.ID(), "Arith", "ThisIsNotAMethod", &Args{1, 2}, &r)
		if err != nil {
			if !IsRPCError(err) {
				t.Log(err)
				t.Error("expected rpc error")
			}
		}
	})
}

func testCallContext(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith
	arith.ctxTracker = &ctxTracker{}

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second/2)
	defer cancel()

	err = c.CallContext(ctx, dest, "Arith", "Sleep", 5, &struct{}{}, 0)

	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "context") {
		t.Error("expected a context error:", err)
	}

	time.Sleep(200 * time.Millisecond)

	if !arith.ctxTracker.cancelled() {
		t.Error("expected ctx cancellation in the function")
	}
}

func TestCallContext(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("local", func(t *testing.T) {
		testCallContext(t, cm[0], cm[1], cm[1].host.ID())
	})

	// t.Run("remote", func(t *testing.T) {
	// 	testCallContext(t, h1, h2, h1.ID())
	// 	testLogger.Info("***DONE REMOTE")
	// })

	t.Run("async", func(t *testing.T) {
		s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
		c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)

		var arith Arith
		arith.ctxTracker = &ctxTracker{}
		err := s.Register(&arith)
		if err != nil {
			t.Error("failed to Register")
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second/2)
		defer cancel()

		done := make(chan *Call, 1)
		err = c.GoContext(ctx, cm[0].host.ID(), "Arith", "Sleep", 5, &struct{}{}, done)
		if err != nil {
			t.Fatal(err)
		}

		call := <-done
		if call.Error == nil || !strings.Contains(call.Error.Error(), "context") {
			t.Error("expected a context error:", err)
		}
	})
}

func TestMultiCall(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	replies := make([]int, 2)
	ctxs := make([]context.Context, 2)
	repliesInt := make([]interface{}, 2)

	for i := range repliesInt {
		repliesInt[i] = &replies[i]
		ctxs[i] = context.Background()
	}

	errs := c.MultiCall(
		ctxs,
		[]peer.ID{cm[0].host.ID(), cm[1].host.ID()},
		"Arith",
		"Multiply",
		&Args{2, 3},
		repliesInt,
	)

	if len(errs) != 2 {
		t.Fatal("expected two errs")
	}

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}

	for _, reply := range replies {
		if reply != 6 {
			t.Error("expected 2*3=6")
		}
	}
}

func TestMultiGo(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	replies := make([]int, 2)
	ctxs := make([]context.Context, 2)
	repliesInt := make([]interface{}, 2)
	dones := make([]chan *Call, 2)

	for i := range repliesInt {
		repliesInt[i] = &replies[i]
		ctxs[i] = context.Background()
		dones[i] = make(chan *Call, 1)
	}

	err = c.MultiGo(
		ctxs,
		[]peer.ID{cm[0].host.ID(), cm[1].host.ID()},
		"Arith",
		"Multiply",
		&Args{2, 3},
		repliesInt,
		dones,
	)
	if err != nil {
		t.Error(err)
	}

	<-dones[0]
	<-dones[1]

	for _, reply := range replies {
		if reply != 6 {
			t.Error("expected 2*3=6")
		}
	}
}

func testDecodeContext(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith
	arith.ctxTracker = &ctxTracker{}

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()

	var res int

	err = c.CallContext(ctx, dest, "Arith", "Add", Args{1, 1}, &res, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDecodeContext(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("local", func(t *testing.T) {
		testDecodeContext(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testDecodeContext(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func TestAuthorization(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	cm = append(cm, NewMockConnectionManager("/ip4/127.0.0.1/tcp/19997"))

	cm[2].host.Peerstore().AddAddrs(cm[0].host.ID(), cm[0].host.Addrs(), peerstore.PermanentAddrTTL)

	authorizationFunc := AuthorizeWithMap(
		map[peer.ID]map[string]bool{
			cm[1].host.ID(): {
				"Arith.Multiply": true,
			},
		},
	)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc", WithAuthorizeFunc(authorizationFunc))
	c := NewClientWithServer(testLogger, cm[1], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	dest := cm[0].host.ID()

	var r int

	err = c.Call(dest, "Arith", "Multiply", &Args{2, 3}, &r)
	if err != nil {
		t.Fatal(err)
	}

	if r != 6 {
		t.Error("result is:", r)
	}

	var q Quotient
	err = c.Call(dest, "Arith", "Divide", &Args{20, 6}, &q)

	if err == nil {
		t.Fatal("expected error instead")
	}

	if !IsAuthorizationError(err) {
		t.Error("expected authorization error, but found", responseErrorType(err))
	}

	c1 := NewClientWithServer(testLogger, cm[2], "rpc", nil, s)
	err = c1.Call(dest, "Arith", "Multiply", &Args{2, 3}, &r)

	if err == nil {
		t.Fatal("expected error instead")
	}

	if !IsAuthorizationError(err) {
		t.Error("expected authorization error, but found", responseErrorType(err))
	}

	// Authorization should not impact while accessing methods locally.
	// All methods should be allowed locally.
	t.Run("local", func(t *testing.T) {
		testCall(t, cm[0], cm[1], "")
	})
}

func testRequestSenderPeerIDContext(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith
	arith.ctxTracker = &ctxTracker{}

	err := s.Register(&arith)
	if err != nil {
		t.Fatal(err)
	}

	err = c.Call(dest, "Arith", "PrintHelloWorld", struct{}{}, &struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	p, err := GetRequestSender(arith.ctxTracker.ctx)
	if err != nil {
		t.Fatal(err)
	}

	if dest == "" || dest == clientCM.GetHostPeerID() {
		if p != serverCM.host.ID() {
			t.Errorf("invalid peer id of request sender on local call: have: %s, want: %s", p, serverCM.host.ID())
		}
	} else {
		if p != clientCM.GetHostPeerID() {
			t.Errorf("invalid peer id of request sender on remote call: have: %s, want: %s", p, clientCM.host.ID())
		}
	}
}

func TestRequestSenderPeerIDContext(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("local", func(t *testing.T) {
		testRequestSenderPeerIDContext(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testRequestSenderPeerIDContext(t, cm[0], cm[1], cm[1].host.ID())
	})
}

func testStream(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	numbers <- Args{2, 3}
	numbers <- Args{6, 2}
	numbers <- Args{9, 5}
	close(numbers)

	err = c.Stream(context.Background(), dest, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	if err != nil {
		t.Fatal(err)
	}

	if len(quotients) != 3 {
		t.Fatal("expected 3 quotients waiting in channel")
	}

	q := <-quotients

	if q.Quo != 0 || q.Rem != 2 {
		t.Error("wrong result")
	}

	q = <-quotients

	if q.Quo != 3 || q.Rem != 0 {
		t.Error("wrong result")
	}

	q = <-quotients
	if q.Quo != 1 || q.Rem != 4 {
		t.Error("wrong result")
	}

	_, ok := <-quotients

	if ok {
		t.Error("channel should have been closed")
	}

	// Now test with pointer arguments
	numbersP := make(chan *Args, 10)
	quotientsP := make(chan *Quotient, 10)

	numbersP <- &Args{2, 3}
	close(numbersP)

	err = c.Stream(context.Background(), dest, "Arith", "DivideMyNumbersPointers", numbersP, quotientsP, "rpc-conn-tag")
	if err != nil {
		t.Fatal(err)
	}

	qP := <-quotientsP

	if qP.Quo != 0 || qP.Rem != 2 {
		t.Error("wrong result")
	}

	_, ok = <-quotientsP

	if ok {
		t.Error("channel should be closed")
	}
}

func TestStream(t *testing.T) {
	cm := createConnectionMangers(t, 2)

	t.Run("local", func(t *testing.T) {
		testStream(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testStream(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func testStreamError(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()
	t.Parallel()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	numbers <- Args{2, 3}
	numbers <- Args{6, 0}
	close(numbers)

	err = c.Stream(context.Background(), dest, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	if err == nil {
		t.Error("expected an error")
	}

	if err.Error() != "divide by zero" {
		t.Error("wrong error message")
	}

	// sometimes the error comes in before the first response is posted on
	// the channel, sometimes it doesn't. In any case, channel should be
	// closed soon.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wait := make(chan struct{})

	go func() {
		defer func() { wait <- struct{}{} }()

		for {
			select {
			case <-ctx.Done():
				t.Error("should have drained the channel")

				return
			case _, ok := <-quotients:
				if !ok {
					return
				}
			}
		}
	}()
	<-wait
}

func TestStreamError(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)

	t.Cleanup(func() {
		stopConnectionManagers(t, cm)
	})

	t.Run("local", func(t *testing.T) {
		testStreamError(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testStreamError(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func testStreamCancel(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()
	t.Parallel()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	if err := s.Register(&arith); err != nil {
		t.Error("failed to Register")
	}

	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)

		defer cancel()

		numbers <- Args{2, 3}
		numbers <- Args{1234, 5}
		close(numbers)

		err := c.Stream(ctx, dest, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
		if err == nil {
			t.Error("expected an error")
		}

		if err.Error() != ctx.Err().Error() {
			t.Error("wrong error message")
		}
	}()

	// channel should be closed.
	ctx2, cancel := context.WithTimeout(context.Background(), 4*time.Second)

	defer cancel()

	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx2.Done():
				t.Error("should have drained the channel")

				return
			case _, ok := <-quotients:
				if !ok {
					return
				}
			}
		}
	}()
	wg.Wait()
}

func TestStreamCancel(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)

	t.Cleanup(func() {
		stopConnectionManagers(t, cm)
	})

	t.Run("local", func(t *testing.T) {
		testStreamCancel(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testStreamCancel(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func TestMultiStream(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
	s2 := NewServer(testLogger, cm[1], "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	err = s2.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()
	dests := make([]peer.ID, 2)
	dests[0] = cm[0].host.ID()
	dests[1] = cm[1].host.ID()

	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	numbers <- Args{10, 3}
	numbers <- Args{10, 3}
	close(numbers)

	errs := c.MultiStream(ctx, dests, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(quotients) != 4 {
		t.Error("not all responses arrived")
	}

	for r := range quotients {
		if r.Quo != 3 || r.Rem != 1 {
			t.Error("wrong result")
		}
	}
}

func TestMultiStreamErrors(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
	s2 := NewServer(testLogger, cm[1], "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	err = s2.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx := context.Background()
	dests := make([]peer.ID, 2)
	dests[0] = cm[0].host.ID()
	dests[1] = cm[1].host.ID()

	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	numbers <- Args{10, 0}
	numbers <- Args{10, 0}
	close(numbers)

	errs := c.MultiStream(ctx, dests, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	for _, err := range errs {
		if err == nil {
			t.Fatal("expected errors")
		}
	}
}

func TestMultiStreamCancel(t *testing.T) {
	t.Parallel()

	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	s := NewServer(testLogger, cm[0], "rpc-conn-tag", "rpc")
	s2 := NewServer(testLogger, cm[1], "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, cm[0], "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	err = s2.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	dests := make([]peer.ID, 2)
	dests[0] = cm[0].host.ID()
	dests[1] = cm[1].host.ID()
	numbers := make(chan Args, 10)
	quotients := make(chan Quotient, 10)

	numbers <- Args{10, 2}
	numbers <- Args{1234, 3}
	close(numbers)

	errs := c.MultiStream(ctx, dests, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	for _, err := range errs {
		if err == nil {
			t.Fatal("expected errors")

			continue
		}

		if err.Error() != ctx.Err().Error() {
			t.Error("expected context error")
		}
	}
}

// the client cancels the request but does not close the sending channel. Things
// should return.
func testStreamClientMisbehave(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	arith.ctxTracker = &ctxTracker{}

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	numbers := make(chan Args)
	quotients := make(chan Quotient, 10)

	go func() {
		for {
			numbers <- Args{1234, 5} // slow operation

			time.Sleep(100 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = c.Stream(ctx, dest, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	if err == nil {
		t.Error("expected an error")
	}

	if err.Error() != ctx.Err().Error() {
		t.Error("wrong error message")
	}

	time.Sleep(1000 * time.Millisecond)

	if !arith.ctxTracker.cancelled() {
		t.Error("expected ctx cancellation in the function")
	}
}

func TestStreamClientMisbehave(t *testing.T) {
	cm := createConnectionMangers(t, 2)

	t.Cleanup(func() {
		stopConnectionManagers(t, cm)
	})

	t.Run("local", func(t *testing.T) {
		testStreamClientMisbehave(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testStreamClientMisbehave(t, cm[0], cm[1], cm[0].host.ID())
	})
}

// the server errors but does not cancel the reply channel.
func testStreamServerMisbehave(t *testing.T, serverCM, clientCM *MockConnectionManager, dest peer.ID) {
	t.Helper()

	s := NewServer(testLogger, serverCM, "rpc-conn-tag", "rpc")
	c := NewClientWithServer(testLogger, clientCM, "rpc", nil, s)

	var arith Arith

	err := s.Register(&arith)
	if err != nil {
		t.Error("failed to Register")
	}

	numbers := make(chan Args, 2)
	quotients := make(chan Quotient, 10)

	go func() {
		numbers <- Args{2, 3}
		numbers <- Args{666, 1} // causes error without closing channel

		for {
			numbers <- Args{1234, 5} // slow operation

			time.Sleep(100 * time.Millisecond)
		}
	}()

	err = c.Stream(context.Background(), dest, "Arith", "DivideMyNumbers", numbers, quotients, "rpc-conn-tag")
	if err == nil {
		t.Error("expected an error")
	}

	if err.Error() != "bad bad bad" {
		t.Error("wrong error message")
	}
}

func TestStreamServerMisbehave(t *testing.T) {
	cm := createConnectionMangers(t, 2)
	defer stopConnectionManagers(t, cm)

	t.Run("local", func(t *testing.T) {
		testStreamServerMisbehave(t, cm[0], cm[1], cm[1].host.ID())
	})

	t.Run("remote", func(t *testing.T) {
		testStreamServerMisbehave(t, cm[0], cm[1], cm[0].host.ID())
	})
}

func CreateKramaID() (string, error) {
	return "", nil
}
