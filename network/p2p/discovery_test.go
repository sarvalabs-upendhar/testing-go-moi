package p2p

import (
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/stretchr/testify/require"
)

func TestAdvertise(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		1,
		2,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	go servers[0].ds.advertise()

	time.Sleep(100 * time.Millisecond)

	peerInfo := findPeer(t, servers[1])

	require.Equal(t, servers[0].host.ID(), peerInfo.ID)
}

func TestDiscover(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		1,
		2,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
	}

	servers := createMultipleServers(t, 2, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	servers[1].cfg.DiscoveryInterval = 100 * time.Millisecond

	go servers[0].ds.advertise()

	go servers[1].ds.discover()

	peerInfo := <-servers[1].ds.peerChan

	require.Equal(t, servers[0].host.ID(), peerInfo.ID)
}

func TestCheckEvents(t *testing.T) {
	t.Parallel()

	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
		4,
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

	PeerEventSub := servers[0].mux.Subscribe(utils.NewPeerEvent{}) // subscribe to server-1 events

	initDiscoveryAndAdvertise(t, servers[0])
	time.Sleep(200 * time.Millisecond)
	initDiscoveryAndAdvertise(t, servers[1])
	time.Sleep(500 * time.Millisecond)

	// check if server-0,1 are able to discover each other
	checkForPeerRegistration(t, servers[0], servers[1], true)
	checkForPeerRegistration(t, servers[1], servers[0], true)

	validateNewPeerEvent(t, PeerEventSub, servers[1])
}

func TestHandlePeers(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		5,
		bootNodes,
		1,
		4,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
		3: params[3],
		4: params[4],
	}

	servers := createMultipleServers(t, 6, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	tests := []struct {
		name              string
		source            *Server
		destination       *Server
		testFn            func(source *Server, destination *Server)
		expectedPeerCount int
	}{
		{
			name:              "new valid peer",
			source:            servers[0],
			destination:       servers[1],
			expectedPeerCount: 1,
		},
		{
			name:              "new peer with same peer id",
			source:            servers[2],
			destination:       servers[2],
			expectedPeerCount: 0,
		},
		{
			name:        "peer in cooldown cache",
			source:      servers[2],
			destination: servers[3],
			testFn: func(source *Server, destination *Server) {
				source.ConnManager.coolDownCache.Add(destination.host.ID())
			},
			expectedPeerCount: 0,
		},
		{
			name:        "peer already registered",
			source:      servers[3],
			destination: servers[4],
			testFn: func(source *Server, destination *Server) {
				source.Peers.addPeer(&Peer{
					kramaID:   destination.id,
					networkID: destination.host.ID(),
				})
			},
			expectedPeerCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.source, test.destination)
			}

			go test.source.ds.handleDiscoveredPeers()

			time.Sleep(50 * time.Millisecond)

			test.source.ds.peerChan <- peer.AddrInfo{
				ID:    test.destination.host.ID(),
				Addrs: test.destination.host.Addrs(),
			}

			time.Sleep(100 * time.Millisecond)

			require.Equal(t, test.expectedPeerCount, len(test.source.ConnManager.getPeers()))
		})
	}
}

func TestHandlePeers_CheckConfig(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		1,
		4,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
	}

	tests := []struct {
		name                 string
		testFn               func()
		expectedPeerCount    int
		shouldExistInSenatus bool
	}{
		{
			name: "discovery disabled, refresh senatus enabled",
			testFn: func() {
				paramsMap[0].ConfigCallback = func(c *config.NetworkConfig) {
					c.BootstrapPeers = getMultiAddresses(t, bootNodes)
					c.OutboundConnLimit = 4
					c.InboundConnLimit = 1
					c.NoDiscovery = true
					c.RefreshSenatus = true
				}
			},
			expectedPeerCount:    0,
			shouldExistInSenatus: true,
		},
		{
			name: "discovery enabled, refresh senatus disabled",
			testFn: func() {
				paramsMap[0].ConfigCallback = func(c *config.NetworkConfig) {
					c.BootstrapPeers = getMultiAddresses(t, bootNodes)
					c.OutboundConnLimit = 4
					c.InboundConnLimit = 1
					c.NoDiscovery = false
					c.RefreshSenatus = false
				}
			},
			expectedPeerCount:    1,
			shouldExistInSenatus: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			servers := createMultipleServers(t, 2, paramsMap)

			t.Cleanup(func() {
				closeTestServers(t, servers)
			})

			go servers[0].ds.handleDiscoveredPeers()

			time.Sleep(100 * time.Millisecond)

			servers[0].ds.peerChan <- peer.AddrInfo{
				ID:    servers[1].host.ID(),
				Addrs: servers[1].host.Addrs(),
			}

			time.Sleep(100 * time.Millisecond)

			require.Equal(t, test.expectedPeerCount, len(servers[0].ConnManager.getPeers()))
			assertNodeMetaInfoInSenatus(t, servers[0], servers[1].host.ID(), test.shouldExistInSenatus)
		})
	}
}

func TestHandlePeerDiscoveryRequest(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		1,
		4,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
		3: params[3],
	}

	servers := createMultipleServers(t, 4, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	tests := []struct {
		name                 string
		source               *Server
		destination          *Server
		testFn               func(server *Server)
		shouldExistInSenatus bool
	}{
		{
			name:                 "request to discover an active peer",
			source:               servers[0],
			destination:          servers[1],
			shouldExistInSenatus: true,
		},
		{
			name:        "request to discover an inactive peer",
			source:      servers[2],
			destination: servers[3],
			testFn: func(server *Server) {
				err := server.host.Close()
				require.NoError(t, err)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.destination)
			}

			go test.source.ds.handlePeerDiscoveryRequest()

			time.Sleep(50 * time.Millisecond)

			postDiscoverPeerEvent(t, test.source, test.destination.host.ID())

			time.Sleep(100 * time.Millisecond)

			assertNodeMetaInfoInSenatus(t, test.source, test.destination.host.ID(), test.shouldExistInSenatus)
		})
	}
}
