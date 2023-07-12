package p2p

import (
	"context"
	"errors"
	"testing"
	"time"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/senatus"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
)

const (
	message = "hello world"
	topic   = "pub-sub"
)

var testLogger = hclog.NewNullLogger()

func TestServer_ConnectAndRegisterPeer(t *testing.T) {
	params := getParamsToCreateMultipleServers(
		t,
		4,
		nil,
		1,
		2,
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

	testcases := []struct {
		name        string
		server      *Server
		destination *Server
		expected    error
	}{
		{
			name:        "Connect server 0 to server 1",
			server:      servers[0],
			destination: servers[1],
			expected:    nil,
		},
		{
			name:        "Connect server 0 to server 2",
			server:      servers[0],
			destination: servers[2],
			expected:    nil,
		},
		{
			name:        "Server 0 tries to connect to server 3, but server 0 is already connected to max outbound peers",
			server:      servers[0],
			destination: servers[3],
			expected:    common.ErrOutboundConnLimit,
		},
		{
			name:        "Server 1 tries to connect to server 2, but server 2 is already connected to max inbound peers",
			server:      servers[1],
			destination: servers[2],
			expected:    errors.New("stream reset"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			info := getPeerInfo(t, testcase.destination)

			// add peer info to peer store
			addPeerInfo(t, testcase.server, info)

			err := testcase.server.ConnectAndRegisterPeer(*info)
			require.Equal(t, testcase.expected, err)
		})
	}
}

func TestConnectToBootStrapNodes(t *testing.T) {
	params := getParamsToCreateMultipleServers(
		t,
		1,
		nil,
		1,
		2,
		false,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
	}

	testCases := []struct {
		name                  string
		bootNodesMultiAddress []multiaddr.Multiaddr
		expectedError         string
	}{
		{
			name:                  "No BootNodes",
			bootNodesMultiAddress: nil,
			expectedError:         "no bootnodes",
		},
		{
			name:                  "Passing bootnodes which are not live",
			bootNodesMultiAddress: getRandomMultiAddrArray(t),
			expectedError:         "unable to connect to at least 1 bootstrap node(s)",
		},
		{
			name:                  "Ideal Case, Passing live bootnodes",
			bootNodesMultiAddress: getMultiAddresses(t, getBootstrapNodes(t, 2)),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := createServer(t, 0, paramsMap[0])

			// assign bootstrap nodes to server's config
			server.cfg.BootstrapPeers = testCase.bootNodesMultiAddress

			// Setup Host
			err := server.setupHost()
			require.NoError(t, err, testCase.expectedError)
			// Setup PubSub
			err = server.setupPubSub()
			require.NoError(t, err, testCase.expectedError)

			// Test
			err = server.connectToBootStrapNodes()

			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err, testCase.expectedError)
			}

			t.Cleanup(func() {
				closeTestServer(t, server)
			})
		})
	}
}

func TestNewServer_CheckBootNodeConnections(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 3)

	defaultConfig := getParamsToCreateMultipleServers(
		t,
		1,
		bootNodes,
		2,
		2,
		false,
	)

	server := createServer(t, 0, defaultConfig[0])
	server.StartServer()

	bootNodePeerIDs := getPeerIDs(t, bootNodes)
	// check if server connected to bootstrap peers
	for i := 0; i < 3; i++ {
		checkConnection(t, server, bootNodePeerIDs[i], true)
	}

	checkForHost(t, server)
	checkForSubscription(t, server)
	checkForKadDHT(t, server)
	closeTestServer(t, server)
}

func TestSetupHost(t *testing.T) {
	testcases := []struct {
		name          string
		isCtxNil      bool
		expectedError error
	}{
		{
			name:          "nil context",
			isCtxNil:      true,
			expectedError: errors.New("lifecycle context for node not configured"),
		},
		{
			name:          "valid context",
			isCtxNil:      false,
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			server := createServerWithoutHost(t)

			if test.isCtxNil {
				server.ctx = nil
			}

			err := server.setupHost()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
				checkForHost(t, server)
				checkForKadDHT(t, server)
				closeTestServer(t, server)
			}
		})
	}
}

func TestConnectToTrustedNodes(t *testing.T) {
	params := getParamsToCreateMultipleServers(
		t,
		5,
		nil,
		2,
		0,
		true,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
		3: params[3],
		4: params[4],
	}
	servers := createMultipleServers(t, 5, paramsMap)

	for index, server := range servers {
		t.Log("Test connect peer", "index", index, "id", server.id)
	}

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	testcases := []struct {
		name                   string
		kramaID                id.KramaID
		establishedConnections map[int][]*Server
		newConnections         map[int][]*Server
	}{
		{
			name:                   "connected nodes",
			establishedConnections: map[int][]*Server{0: {servers[1]}},
			newConnections:         map[int][]*Server{0: {servers[1]}},
		},
		{
			name:                   "not connected nodes",
			establishedConnections: nil,
			newConnections:         map[int][]*Server{2: {servers[3], servers[4]}},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// connect servers
			if test.establishedConnections != nil {
				for i, destinations := range test.establishedConnections {
					connectTo(t, servers[i], destinations...)

					for _, destination := range destinations {
						err := servers[i].Peers.Register(getPeer(t, servers[i], destination))
						require.NoError(t, err)
					}
				}
			}

			for i, destinations := range test.newConnections {
				for _, destination := range destinations {
					// get peer info of node we connect to
					info := getPeerInfo(t, destination)

					// add peer info to peer store
					addPeerInfo(t, servers[i], info)

					servers[i].cfg.TrustedPeers = append(servers[i].cfg.TrustedPeers, config.NodeInfo{
						ID:      destination.id,
						Address: getMultiAddresses(t, []host.Host{destination.host})[0],
					})
				}

				// connect to trusted nodes
				servers[i].connectToTrustedNodes()

				for _, destination := range destinations {
					// get peer info of the node that we have connected to
					info := getPeerInfo(t, destination)

					checkConnection(t, servers[i], info.ID, true)
				}
			}
		})
	}
}

func TestSetupKadDht(t *testing.T) {
	server := createServerWithoutHost(t)
	t.Cleanup(func() {
		closeTestServer(t, server)
	})

	libp2pHost, err := libp2p.New(libp2p.ListenAddrs(tests.GetListenAddresses(t, 1)...))
	require.NoError(t, err)

	server.host = libp2pHost
	err = server.setupKadDht(server.host)
	require.NoError(t, err)

	checkForKadDHT(t, server)
}

func TestSetupPubSub(t *testing.T) {
	server := createServerWithoutHost(t)

	err := server.setupHost()
	require.NoError(t, err)

	checkForHost(t, server)
	t.Cleanup(func() {
		closeTestServer(t, server)
	})

	testcases := []struct {
		name          string
		host          host.Host
		expectedError error
	}{
		{
			name:          "nil host",
			host:          nil,
			expectedError: errors.New("libp2p host for node not configured"),
		},
		{
			name:          "valid host",
			host:          server.host,
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			server.host = test.host
			err := server.setupPubSub()

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
				checkForSubscription(t, server)
			}
		})
	}
}

func TestStreamHandler_Valid_HandshakeMsg(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
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

	registerStreamHandler(servers[1])

	newPeerSub := servers[1].mux.Subscribe(utils.NewPeerEvent{}) // subscribe to server-1 events
	peer := openStream(t, servers[0], servers[1])                // connect to peer-1 and get peer

	// send handshake message
	err := peer.SendHandshakeMessage(servers[0])
	require.NoError(t, err)

	// get the message from stream
	msg := readMessageFromBuffer(t, peer)

	// validate handshake message
	validateHandShakeMsg(t, servers[1], msg, nil)
	validateNewPeerEvent(t, newPeerSub, servers[0]) // server-1 sends the peer-id of server-0 to subscribers
}

func TestStreamHandler_Invalid_HandshakeMsgPayload(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
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

	registerStreamHandler(servers[1])

	peer := openStream(t, servers[0], servers[1]) // connect to peer-1 and get peer

	InvalidHandShakeMsg := getInvalidHandshakeMsg()

	// send invalid handshake message payload to server-1
	err := peer.Send(servers[0].GetKramaID(), networkmsg.HANDSHAKEMSG, InvalidHandShakeMsg)
	require.NoError(t, err)

	// get the message from stream
	msg := readMessageFromBuffer(t, peer)

	// validate handshake message
	validateHandShakeMsg(t, servers[1], msg, errors.New("wrong message type"))

	// make sure server-0 not registered in server-1
	checkForPeerRegistration(t, servers[1], servers[0], false)
}

func TestStreamHandlerFunc_Invalid_MessagePayload(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
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

	registerStreamHandler(servers[1])

	peer := openStream(t, servers[0], servers[1]) // connect to peer-1 and get peer

	invalidMessagePayload := []byte{200}
	// send invalid message payload to server-1
	sendMessage(t, peer, invalidMessagePayload)

	// get the message from stream
	msg := readMessageFromBuffer(t, peer)

	// validate handshake message
	validateHandShakeMsg(t, servers[1], msg, errors.New("invalid message"))

	// make sure server-0 not registered in server-1
	checkForPeerRegistration(t, servers[1], servers[0], false)
}

func TestStreamHandler_Already_RegisteredPeer(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
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

	registerStreamHandler(servers[1])

	peer := openStream(t, servers[0], servers[1]) // open stream to peer-1 and get peer-1

	// register server-0's peer in server-1
	err := servers[1].Peers.Register(getPeer(t, servers[1], servers[0]))
	require.NoError(t, err)

	// send handshake message to server-1
	err = peer.SendHandshakeMessage(servers[0])
	require.NoError(t, err)

	// get the message from stream
	msg := readMessageFromBuffer(t, peer)

	validateHandShakeMsg(t, servers[1], msg, errAlreadyRegistered)
}

func TestDiscover_CheckEvents(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
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

	PeerEventSub := servers[0].mux.Subscribe(utils.NewPeerEvent{}) // subscribe to server-1 events
	PeerDiscoveredEventSub := servers[0].mux.Subscribe(utils.PeerDiscoveredEvent{})

	go servers[0].discover()
	time.Sleep(8 * time.Second)

	go servers[1].discover()
	time.Sleep(5 * time.Second)

	// check if server-0,1 are able to discover each other after 10 seconds
	checkForPeerRegistration(t, servers[0], servers[1], true)
	checkForPeerRegistration(t, servers[1], servers[0], true)

	validateNewPeerEvent(t, PeerEventSub, servers[1])
	validatePeerDiscoveredEvent(t, PeerDiscoveredEventSub, servers[1])
}

func TestServer_HandleDiscovery(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	params := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
		2,
		false,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
	}

	tests := []struct {
		name           string
		noDiscovery    bool
		refreshSenatus bool
		testFn         func()
	}{
		{
			name:           "discovery disabled, refresh senatus enabled",
			noDiscovery:    true,
			refreshSenatus: true,
			testFn: func() {
				paramsMap[0].ConfigCallback = func(c *config.NetworkConfig) {
					c.BootstrapPeers = getMultiAddresses(t, bootNodes)
					c.OutboundConnLimit = 2
					c.InboundConnLimit = 2
					c.NoDiscovery = true
					c.RefreshSenatus = true
				}
			},
		},
		{
			name:           "discovery enabled, refresh senatus disabled",
			noDiscovery:    false,
			refreshSenatus: false,
			testFn: func() {
				paramsMap[0].ConfigCallback = func(c *config.NetworkConfig) {
					c.BootstrapPeers = getMultiAddresses(t, bootNodes)
					c.OutboundConnLimit = 2
					c.InboundConnLimit = 2
					c.NoDiscovery = false
					c.RefreshSenatus = false
				}
			},
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

			initDiscoveryAndAdvertise(t, servers...)

			err := servers[0].handleDiscovery()
			require.NoError(t, err)

			peerIDs := getPeerIDs(t, []host.Host{servers[0].host, servers[1].host})

			require.Equal(t, !test.noDiscovery, servers[0].Peers.ContainsPeer(peerIDs[1]))

			if !test.refreshSenatus {
				_, err := servers[0].Senatus.GetAddressByPeerID(peerIDs[1])
				require.Error(t, err)

				return
			}

			addr, err := servers[0].Senatus.GetAddressByPeerID(peerIDs[1])
			require.NoError(t, err)

			require.Equal(t, servers[1].host.Addrs(), addr)
		})
	}
}

func TestServer_GetPeerInfo(t *testing.T) {
	params := &CreateServerParams{
		EventMux: &utils.TypeMux{},
		ConfigCallback: func(c *config.NetworkConfig) {
			c.InboundConnLimit = 10
			c.OutboundConnLimit = 5
		},
		Logger: testLogger,
	}
	server := createServer(t, 0, params)

	tests := []struct {
		name        string
		peerInfo    *peer.AddrInfo
		testFn      func(peerInfo *peer.AddrInfo)
		expectedErr error
	}{
		{
			name: "Peer info present in Peer Store",
			peerInfo: &peer.AddrInfo{
				ID:    tests.GetTestPeerID(t),
				Addrs: tests.GetListenAddresses(t, 1),
			},
			testFn: func(peerInfo *peer.AddrInfo) {
				server.addToPeerStore(peerInfo)
			},
			expectedErr: nil,
		},
		{
			name: "Peer info not present in Peer Store, present in Senatus",
			peerInfo: &peer.AddrInfo{
				ID:    tests.GetTestPeerID(t),
				Addrs: tests.GetListenAddresses(t, 1),
			},
			testFn: func(peerInfo *peer.AddrInfo) {
				params.ServerCallback = func(s *Server) {
					m := NewMockReputationEngine()
					m.SetPeerInfo(
						peerInfo.ID,
						&senatus.NodeMetaInfo{
							Addrs: utils.MultiAddrToString(peerInfo.Addrs...),
							NTQ:   senatus.DefaultPeerNTQ,
						},
					)
					s.Senatus = m
				}

				server = createServer(t, 0, params)
			},
			expectedErr: nil,
		},
		{
			name: "Peer info not present in Peer Store, not present in Senatus",
			peerInfo: &peer.AddrInfo{
				ID:    tests.GetTestPeerID(t),
				Addrs: tests.GetListenAddresses(t, 1),
			},
			expectedErr: common.ErrKramaIDNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerInfo)
			}

			peerInfo, err := server.GetPeerInfo(test.peerInfo.ID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.peerInfo, peerInfo)
		})
	}
}

func TestConnectPeer(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		2,
		2,
		false,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
		3: params[3],
	}
	servers := createMultipleServers(t, 4, paramsMap)

	for index, server := range servers {
		t.Log("Test connect peer", "index", index, "id", server.id)
	}

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	testcases := []struct {
		name                   string
		kramaID                id.KramaID
		establishedConnections map[int][]*Server
		newConnections         map[int][]*Server
		expectedError          error
	}{
		{
			name:                   "invalid krama id",
			kramaID:                "Invalid_Krama_ID",
			establishedConnections: nil,
			newConnections:         nil,
			expectedError:          mudraCommon.ErrInvalidKramaID,
		},
		{
			name:                   "connected nodes",
			kramaID:                servers[1].GetKramaID(),
			establishedConnections: map[int][]*Server{0: {servers[1]}},
			newConnections:         map[int][]*Server{0: {servers[1]}},
			expectedError:          common.ErrConnectionExists,
		},
		{
			name:                   "not connected nodes",
			kramaID:                servers[3].GetKramaID(),
			establishedConnections: nil,
			newConnections:         map[int][]*Server{2: {servers[3]}},
			expectedError:          nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// connect servers
			if test.establishedConnections != nil {
				for i, destinations := range test.establishedConnections {
					connectTo(t, servers[i], destinations...)
				}
			}

			for i, destinations := range test.newConnections {
				for _, destination := range destinations {
					// get peer info of node we connect to
					info := getPeerInfo(t, destination)

					// add peer info to peer store
					addPeerInfo(t, servers[i], info)

					err := servers[i].ConnectPeer(test.kramaID)

					if test.expectedError != nil {
						require.EqualError(t, err, test.expectedError.Error())
					} else {
						require.NoError(t, err)
						checkConnection(t, servers[i], info.ID, true)
					}
				}
			}
		})
	}
}

func TestDisconnectPeer(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		2,
		2,
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

	testcases := []struct {
		name                   string
		kramaID                id.KramaID
		establishedConnections map[int][]*Server
		disConnections         map[int][]*Server
		expectedError          error
	}{
		{
			name:          "invalid krama id",
			kramaID:       "abcd",
			expectedError: mudraCommon.ErrInvalidKramaID,
		},
		{
			name:                   "connected nodes",
			kramaID:                servers[1].GetKramaID(),
			establishedConnections: map[int][]*Server{0: {servers[1]}},
			disConnections:         map[int][]*Server{0: {servers[1]}},
			expectedError:          nil,
		},
		{
			name:           "not connected nodes",
			kramaID:        servers[3].GetKramaID(),
			disConnections: map[int][]*Server{0: {servers[1]}},
			expectedError:  nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// connect servers
			if test.establishedConnections != nil {
				for i, destinations := range test.establishedConnections {
					connectTo(t, servers[i], destinations...)
				}
			}

			for i, destinations := range test.disConnections {
				for _, destination := range destinations {
					// get peer info of node we disconnect to
					info := getPeerInfo(t, destination)

					err := servers[i].DisconnectPeer(test.kramaID)

					if test.expectedError != nil {
						require.EqualError(t, err, test.expectedError.Error())
					} else {
						require.NoError(t, err)
						checkConnection(t, servers[i], info.ID, false)
					}
				}
			}
		})
	}
}

func TestGetPeers(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	params := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
		2,
		true,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
	}
	servers := createMultipleServers(t, 2, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	peersList := tests.GetTestKramaIDs(t, 2)

	testcases := []struct {
		name         string
		server       *Server
		expectedList []id.KramaID
		testFn       func()
	}{
		{
			name:         "Should return an empty list if no Krama ID's in peersList",
			server:       servers[0],
			expectedList: make([]id.KramaID, 0),
		},
		{
			name:   "Returns a slice of Krama ID's connected to a client",
			server: servers[1],
			testFn: func() {
				setServerPeers(t, servers[1], peersList)
			},
			expectedList: peersList,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			fetchedList := test.server.GetPeers()
			require.ElementsMatch(t, test.expectedList, fetchedList)
		})
	}
}

func TestSendMessage_CheckMsgHandler(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		2,
		2,
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

	testcases := []struct {
		name          string
		index         int
		shouldAddPeer bool
	}{
		{
			name:          "peer id in peers list",
			index:         0,
			shouldAddPeer: true,
		},
		{
			name:  "peer id not in peers list",
			index: 2,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connectTo(t, servers[test.index], servers[test.index+1])

			// used to wait until handler executes
			response := make(chan int)

			servers[test.index].GetAddrs()

			handShakeMsg, err := servers[test.index].constructHandshakeMSG()
			require.NoError(t, err)
			// handles sent message
			registerMessageHandler(
				t, servers[test.index+1],
				config.MOIProtocolStream,
				servers[test.index].id,
				handShakeMsg,
				response,
			)

			if test.shouldAddPeer {
				peer := openStream(t, servers[test.index], servers[test.index+1])
				addPeer(t, servers[test.index], peer)
			}

			// send message from first server to second server
			err = servers[test.index].SendMessage(servers[test.index+1].host.ID(), networkmsg.HANDSHAKEMSG, &handShakeMsg)
			require.NoError(t, err)

			// wait till handler completes
			ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancelFn()

			err = waitForResponse(ctx, response)
			require.NoError(t, err)
		})
	}
}

func TestSubscribe_Twice_OnSameTopic(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		1,
		bootNodes,
		2,
		2,
		false,
	)
	paramsMap := map[int]*CreateServerParams{
		0: params[0],
	}
	servers := createMultipleServers(t, 1, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	registerEmptySubscriptionHandler(t, servers[0], topic, false)
	err := servers[0].Subscribe(servers[0].ctx, topic, func(msg *pubsub.Message) error { // subscribing again on same topic
		return nil
	})
	require.ErrorContains(t, err, "topic already exists")
}

func TestSubscribe_CheckMsgOnTopic(t *testing.T) {
	// used to wait until handler executes message published on topic
	response := make(chan int)

	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
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

	startDiscovery(t, servers...)
	registerEmptySubscriptionHandler(t, servers[0], topic, true) // shouldn't receive self-published message
	subscribeMessage(t, servers[1], topic, response)

	// make sure handlers stored
	checkForTopicSet(t, servers[0], topic)
	checkForTopicSet(t, servers[1], topic)

	time.Sleep(time.Second) // wait for subscription to happen

	rawData, err := polo.Polorize(message)
	require.NoError(t, err)
	err = servers[0].pubSubTopics.getTopicSet(topic).topicHandle.Publish(servers[0].ctx, rawData)
	require.NoError(t, err)

	// wait till handler completes
	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	err = waitForResponse(ctx, response)
	require.NoError(t, err)
}

func TestUnSubscribe_CheckTopic(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		1,
		bootNodes,
		2,
		2,
		false,
	)
	server := createServer(t, 0, params[0])

	t.Cleanup(func() {
		closeTestServer(t, server)
	})

	registerEmptySubscriptionHandler(t, server, topic, false)
	unsubscribeServers(t, server, topic)
	// make sure topic removed
	checkForTopic(t, server, topic, false)
}

func TestBroadcast_UnSubscribedTopic(t *testing.T) {
	s := createServer(t, 0, nil)

	err := s.Broadcast("topic_2", []byte("0x00"))
	require.ErrorContains(t, err, "topic not found")
}

func TestBroadcast_CheckMsgOnTopic(t *testing.T) {
	response := make(chan int)

	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
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

	startDiscovery(t, servers...)
	registerEmptySubscriptionHandler(t, servers[0], topic, true)
	subscribeMessage(t, servers[1], topic, response)
	time.Sleep(1 * time.Second) // wait for discovery and subscription

	rawData, err := polo.Polorize(message)
	require.NoError(t, err)

	err = servers[0].Broadcast(topic, rawData)
	require.NoError(t, err)

	// wait till handler completes
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()

	err = waitForResponse(ctx, response)
	require.NoError(t, err)
}

func TestJoinPubSubTopic(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)
	params := getParamsToCreateMultipleServers(
		t,
		1,
		bootNodes,
		2,
		2,
		false,
	)
	server := createServer(t, 0, params[0])

	t.Cleanup(func() {
		closeTestServer(t, server)
	})

	testTopicSet := &TopicSet{
		topicHandle: new(pubsub.Topic),
		subHandle:   nil,
	}

	server.pubSubTopics.addTopicSet("topic_1", testTopicSet)

	tests := []struct {
		name             string
		topic            string
		existingTopicSet *TopicSet
	}{
		{
			name:             "Should return available topicSet for existing topic",
			topic:            "topic_1",
			existingTopicSet: testTopicSet,
		},
		{
			name:             "Should return new topicSet for non-existing topic",
			topic:            "topic_2",
			existingTopicSet: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			existingTopicSet, err := server.JoinPubSubTopic(test.topic)
			require.NoError(t, err)

			if test.existingTopicSet != nil {
				require.Equal(t, testTopicSet, existingTopicSet)

				return
			}

			// make sure only topicHandle created
			require.NotNil(t, server.pubSubTopics.getTopicSet(test.topic).topicHandle)
			require.Nil(t, server.pubSubTopics.getTopicSet(test.topic).subHandle)
		})
	}
}

func TestSendHelloMessage_CheckMsgOnTopic(t *testing.T) {
	// used to wait until handler executes message published on topic
	response := make(chan int)

	bootNodes := getBootstrapNodes(t, 2)
	defaultConfig := getParamsToCreateMultipleServers(
		t,
		2,
		bootNodes,
		2,
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

	startDiscovery(t, servers...)
	registerEmptySubscriptionHandler(t, servers[0], SenatusTopic, true)
	subscribeHelloMsg(t, servers[1], SenatusTopic, servers[0], response)
	time.Sleep(5 * time.Second) // give time for discovery and subscription

	servers[0].SendHelloMessage()

	// wait till handler completes
	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	err := waitForResponse(ctx, response)
	require.NoError(t, err)
}

func TestGetRandomNode_CheckInRoutingTable(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	addresses := getMultiAddresses(t, bootNodes)
	paramsMap := map[int]*CreateServerParams{}

	for i := 0; i < 9; i++ {
		paramsMap[i] = &CreateServerParams{
			EventMux: &utils.TypeMux{},
			ConfigCallback: func(c *config.NetworkConfig) {
				c.BootstrapPeers = addresses
			},
			ServerCallback: func(s *Server) {
				m := NewMockReputationEngine()
				m.SetNTQ(s.GetKramaID(), getRandomNumber(t, 27))
				s.Senatus = m
			},
		}
	}

	servers := createMultipleServers(t, 9, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	startDiscovery(t, servers...)
	time.Sleep(5 * time.Second)

	// check if random peer exists in routing table
	for i := 0; i < 9; i++ { // indicates server
		for j := 0; j < 10; j++ { // indicates no. of times random node fetched
			pID := servers[i].GetRandomNode()
			routingTable := servers[0].kadDHT.RoutingTable()

			require.NotEqual(t, "", routingTable.Find(pID))
		}
	}
}
