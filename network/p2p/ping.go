package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"io"
	mrand "math/rand"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	pool "github.com/libp2p/go-buffer-pool"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/config"
)

const (
	PingSize         = 32
	PingResponseSize = 92
	pingTimeout      = time.Second * 60

	ServiceName = "moip2p.ping"
)

// PingMessage represents a response to a ping request.
type PingMessage struct {
	Data    []byte
	KramaID identifiers.KramaID
}

// PingService is a service for sending and receiving ping messages.
type PingService struct {
	ID     identifiers.KramaID
	Host   host.Host
	logger hclog.Logger
}

// NewPingService creates a new PingService instance.
func NewPingService(id identifiers.KramaID, h host.Host, logger hclog.Logger) *PingService {
	ps := &PingService{id, h, logger}

	h.SetStreamHandler(config.MOIPingStream, ps.streamHandler)

	return ps
}

// streamHandler handles incoming ping streams.
func (ps *PingService) streamHandler(s network.Stream) {
	if err := s.Scope().SetService(ServiceName); err != nil {
		ps.logger.Error("error attaching stream to ping service: %s", err)
		resetStream(s)

		return
	}

	if err := s.Scope().ReserveMemory(PingSize, network.ReservationPriorityAlways); err != nil {
		ps.logger.Error("error reserving memory for ping stream: %s", err)
		resetStream(s)

		return
	}
	defer s.Scope().ReleaseMemory(PingSize)

	buf := pool.Get(PingSize)
	defer pool.Put(buf)

	errCh := make(chan error, 1)
	defer close(errCh)

	timer := time.NewTimer(pingTimeout)
	defer timer.Stop()

	go func() {
		select {
		case <-timer.C:
			ps.logger.Debug("ping timeout")
		case <-errCh:
		}

		closeStream(s)
	}()

	for {
		_, err := io.ReadFull(s, buf)
		if err != nil {
			errCh <- err

			return
		}

		res := &PingMessage{
			Data:    buf,
			KramaID: ps.ID,
		}

		rawData, err := polo.Polorize(res)
		if err != nil {
			errCh <- err

			return
		}

		_, err = s.Write(rawData)
		if err != nil {
			errCh <- err

			return
		}

		timer.Reset(pingTimeout)
	}
}

// PingResponse represents the outcome of a ping attempt, including RTT, KramaID, and any errors.
type PingResponse struct {
	RTT     time.Duration
	KramaID identifiers.KramaID
	Error   error
}

// Ping initiates a ping to a remote peer and returns a channel of results.
func (ps *PingService) Ping(ctx context.Context, p peer.ID) <-chan PingResponse {
	return Ping(ctx, ps.Host, p)
}

// pingError creates a channel with a ping error result and closes it.
func pingError(err error) chan PingResponse {
	ch := make(chan PingResponse, 1)
	ch <- PingResponse{Error: err}
	close(ch)

	return ch
}

// Ping pings the remote peer until the context is canceled, returns the RTT, KramaID or error.
func Ping(ctx context.Context, h host.Host, p peer.ID) <-chan PingResponse {
	s, err := h.NewStream(network.WithAllowLimitedConn(ctx, "ping"), p, config.MOIPingStream)
	if err != nil {
		return pingError(err)
	}

	if err := s.Scope().SetService(ServiceName); err != nil {
		resetStream(s)

		return pingError(err)
	}

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		resetStream(s)

		return pingError(err)
	}

	ra := mrand.New(mrand.NewSource(int64(binary.BigEndian.Uint64(b))))

	ctx, cancel := context.WithCancel(ctx)

	out := make(chan PingResponse)

	go func() {
		defer close(out)
		defer cancel()

		for ctx.Err() == nil {
			res := ping(s, ra)

			// canceled, ignore everything.
			if ctx.Err() != nil {
				return
			}

			// No error, record the RTT.
			if res.Error == nil {
				h.Peerstore().RecordLatency(p, res.RTT)
			}

			select {
			case out <- res:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		// forces the ping to abort.
		<-ctx.Done()
		closeStream(s)
	}()

	return out
}

// ping performs the actual ping operation and returns the result.
func ping(s network.Stream, randReader io.Reader) PingResponse {
	if err := s.Scope().ReserveMemory(PingResponseSize+PingSize, network.ReservationPriorityAlways); err != nil {
		resetStream(s)

		return PingResponse{Error: err}
	}
	defer s.Scope().ReleaseMemory(PingResponseSize + PingSize)

	buf := pool.Get(PingSize)
	defer pool.Put(buf)

	if _, err := io.ReadFull(randReader, buf); err != nil {
		return PingResponse{Error: err}
	}

	before := time.Now()

	if _, err := s.Write(buf); err != nil {
		return PingResponse{Error: err}
	}

	rbuf := pool.Get(PingResponseSize)
	defer pool.Put(rbuf)

	if _, err := io.ReadFull(s, rbuf); err != nil {
		return PingResponse{Error: err}
	}

	var res PingMessage

	err := polo.Depolorize(&res, rbuf)
	if err != nil {
		return PingResponse{Error: errors.New("failed to depolorize ping packet")}
	}

	if !bytes.Equal(buf, res.Data) {
		return PingResponse{Error: errors.New("ping packet was incorrect")}
	}

	return PingResponse{RTT: time.Since(before), KramaID: res.KramaID}
}

// resetStream resets the given network stream.
func resetStream(s network.Stream) {
	err := s.Reset()
	if err != nil {
		return
	}
}

// closeStream closes the given network stream.
func closeStream(s network.Stream) {
	err := s.Close()
	if err != nil {
		return
	}
}
