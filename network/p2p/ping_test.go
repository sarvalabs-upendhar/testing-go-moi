package p2p

import (
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-polo"

	"github.com/stretchr/testify/require"
)

func TestPing(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	// Create a new host and ping service for testing
	params := getParamsToCreateMultipleServers(
		t,
		3,
		bootNodes,
		1,
		2,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
	}

	servers := createMultipleServers(t, 3, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	tests := []struct {
		name        string
		source      *Server
		destination *Server
		expectedErr bool
	}{
		{
			name:        "ping should return the rtt",
			source:      servers[0],
			destination: servers[1],
		},
		{
			name:        "ping should return an error",
			source:      servers[1],
			destination: servers[1],
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ps := NewPingService(test.source.id, test.source.host, test.source.logger)

			// Create a context with a timeout for the test
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Perform the ping and receive results
			result := <-ps.Ping(ctx, test.destination.host.ID())

			if test.expectedErr {
				require.Error(t, result.Error)

				return
			}

			require.NoError(t, result.Error)
			require.Equal(t, test.destination.id, result.KramaID)
		})
	}
}

func TestStreamHandler_Valid_Response(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		1,
		2,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig[0],
		1: defaultConfig[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	stream := openPingStream(t, servers[0], servers[1])
	// Construct a ping message
	pingMsg := constructPingMessage(t)

	// Write the ping message to the stream
	writePingMessage(t, stream, pingMsg)

	time.Sleep(200 * time.Millisecond)

	// Read the response from the stream
	response := readPingMessage(t, stream)

	require.Equal(t, pingMsg, response.Data)
	require.Equal(t, servers[1].GetKramaID(), response.KramaID)
}

// helper functions

// open a stream between source and destination server for pinging
func openPingStream(t *testing.T, source *Server, destination *Server) network.Stream {
	t.Helper()

	connectTo(t, source, destination)

	stream, err := source.ConnManager.NewStream(source.ctx, destination.host.ID(), config.MOIPingStream, MOIStreamTag)

	require.NoError(t, err)

	return stream
}

// construct and return the ping message
func constructPingMessage(t *testing.T) []byte {
	t.Helper()

	buf := make([]byte, PingSize)

	_, err := io.ReadFull(rand.Reader, buf)
	require.NoError(t, err)

	return buf
}

// read the ping response from the network stream
func readPingMessage(t *testing.T, stream network.Stream) PingMessage {
	t.Helper()

	buf := make([]byte, PingResponseSize)

	_, err := io.ReadFull(stream, buf)
	require.NoError(t, err)

	var res PingMessage

	err = polo.Depolorize(&res, buf)
	require.NoError(t, err)

	return res
}

// write the ping message to the network stream
func writePingMessage(t *testing.T, stream network.Stream, message []byte) {
	t.Helper()

	_, err := stream.Write(message)
	require.NoError(t, err)
}
