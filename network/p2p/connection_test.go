package p2p

import (
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestIsInboundConnLimitReached(t *testing.T) {
	testcases := []struct {
		name                string
		inboundConnCount    int64
		maxInboundConnCount int64
		expected            bool
	}{
		{
			name:                "Inbound limit not reached",
			inboundConnCount:    5,
			maxInboundConnCount: 10,
			expected:            false,
		},
		{
			name:                "Inbound limit reached",
			inboundConnCount:    10,
			maxInboundConnCount: 10,
			expected:            true,
		},
		{
			name:                "Inbound limit exceeded",
			inboundConnCount:    11,
			maxInboundConnCount: 10,
			expected:            true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connManager := &ConnectionManager{
				inboundConnCount:    test.inboundConnCount,
				maxInboundConnCount: test.maxInboundConnCount,
			}

			require.Equal(t, test.expected, connManager.isInboundConnLimitReached())
		})
	}
}

func TestIsOutboundConnLimitReached(t *testing.T) {
	testcases := []struct {
		name                 string
		outboundConnCount    int64
		maxOutboundConnCount int64
		expected             bool
	}{
		{
			name:                 "Outbound limit not reached",
			outboundConnCount:    5,
			maxOutboundConnCount: 10,
			expected:             false,
		},
		{
			name:                 "Outbound limit reached",
			outboundConnCount:    10,
			maxOutboundConnCount: 10,
			expected:             true,
		},
		{
			name:                 "Outbound limit exceeded",
			outboundConnCount:    11,
			maxOutboundConnCount: 10,
			expected:             true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connInfo := &ConnectionManager{
				outboundConnCount:    test.outboundConnCount,
				maxOutboundConnCount: test.maxOutboundConnCount,
			}

			require.Equal(t, test.expected, connInfo.isOutboundConnLimitReached())
		})
	}
}

func TestIsOutboundConnLimitReachedForRange(t *testing.T) {
	testcases := []struct {
		name                 string
		rttRange             uint8
		outboundConnCount    [4]int64
		maxOutboundConnCount int64
		expected             bool
	}{
		{
			name:                 "Range limit not reached",
			rttRange:             0,
			outboundConnCount:    [4]int64{4, 2, 0, 1},
			maxOutboundConnCount: 20,
			expected:             false,
		},
		{
			name:                 "Range limit reached",
			rttRange:             1,
			outboundConnCount:    [4]int64{4, 5, 1, 1},
			maxOutboundConnCount: 20,
			expected:             true,
		},
		{
			name:                 "Range limit exceeded",
			rttRange:             2,
			outboundConnCount:    [4]int64{1, 3, 8, 1},
			maxOutboundConnCount: 20,
			expected:             true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connInfo := &ConnectionManager{
				outboundConnCountByRange: test.outboundConnCount,
				maxOutboundConnCount:     test.maxOutboundConnCount,
			}

			result := connInfo.isOutboundConnLimitReachedForRange(test.rttRange)

			require.Equal(t, test.expected, result)
		})
	}
}

func TestUpdateInboundConnCount(t *testing.T) {
	testcases := []struct {
		name              string
		delta             int64
		inboundConnCount  int64
		expectedConnCount int64
	}{
		{
			name:              "Positive delta",
			delta:             2,
			inboundConnCount:  3,
			expectedConnCount: 5,
		},
		{
			name:              "Negative delta",
			delta:             -2,
			inboundConnCount:  5,
			expectedConnCount: 3,
		},
		{
			name:              "Zero delta",
			delta:             0,
			inboundConnCount:  7,
			expectedConnCount: 7,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionManager{
				server:           createServer(t, 0, nil),
				inboundConnCount: test.inboundConnCount,
			}

			ci.updateInboundConnCount(test.delta)
			require.Equal(t, test.expectedConnCount, ci.inboundConnCount)
		})
	}
}

func TestUpdateOutboundConnCount(t *testing.T) {
	testcases := []struct {
		name              string
		delta             int64
		outboundConnCount int64
		expectedConnCount int64
	}{
		{
			name:              "Positive delta",
			delta:             2,
			outboundConnCount: 10,
			expectedConnCount: 12,
		},
		{
			name:              "Negative delta",
			delta:             -1,
			outboundConnCount: 8,
			expectedConnCount: 7,
		},
		{
			name:              "Zero delta",
			delta:             0,
			outboundConnCount: 4,
			expectedConnCount: 4,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionManager{
				server:            createServer(t, 0, nil),
				outboundConnCount: test.outboundConnCount,
			}

			ci.updateOutboundConnCount(test.delta)
			require.Equal(t, test.expectedConnCount, ci.outboundConnCount)
		})
	}
}

func TestUpdateOutboundConnCountByRange(t *testing.T) {
	testcases := []struct {
		name              string
		delta             int64
		rttRange          uint8
		outboundConnCount [4]int64
		expectedConnCount int64
	}{
		{
			name:              "Positive delta",
			delta:             4,
			rttRange:          0,
			outboundConnCount: [4]int64{2, 4, 2, 2},
			expectedConnCount: 6,
		},
		{
			name:              "Negative delta",
			delta:             -1,
			rttRange:          1,
			outboundConnCount: [4]int64{1, 2, 1, 3},
			expectedConnCount: 1,
		},
		{
			name:              "Zero delta",
			delta:             0,
			rttRange:          2,
			outboundConnCount: [4]int64{1, 1, 1, 1},
			expectedConnCount: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionManager{
				server:                   createServer(t, 0, nil),
				outboundConnCountByRange: test.outboundConnCount,
			}

			ci.updateOutboundConnCountByRange(test.rttRange, test.delta)
			require.Equal(t, test.expectedConnCount, ci.getOutboundConnCountByRange(test.rttRange))
		})
	}
}

func TestUpdateConnCount(t *testing.T) {
	testcases := []struct {
		name              string
		direction         network.Direction
		inboundConnCount  int64
		outboundConnCount int64
		delta             int64
		rtt               int64
		expectedInbound   int64
		expectedOutbound  int64
	}{
		{
			name:              "Increment inbound",
			direction:         network.DirInbound,
			inboundConnCount:  5,
			outboundConnCount: 10,
			delta:             2,
			rtt:               90,
			expectedInbound:   7,
			expectedOutbound:  10,
		},
		{
			name:              "Decrement outbound",
			direction:         network.DirOutbound,
			inboundConnCount:  5,
			outboundConnCount: 10,
			delta:             -2,
			rtt:               150,
			expectedInbound:   5,
			expectedOutbound:  8,
		},
		{
			name:              "Increment outbound",
			direction:         network.DirOutbound,
			inboundConnCount:  0,
			outboundConnCount: 0,
			delta:             2,
			rtt:               500,
			expectedInbound:   0,
			expectedOutbound:  2,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionManager{
				server:                   createServer(t, 0, nil),
				inboundConnCount:         test.inboundConnCount,
				outboundConnCount:        test.outboundConnCount,
				outboundConnCountByRange: [4]int64{},
			}
			rttRange := ci.getConnRTTRange(test.rtt)

			ci.outboundConnCountByRange[rttRange] = test.outboundConnCount

			ci.updateConnCount(test.direction, test.rtt, test.delta)

			require.Equal(t, test.expectedInbound, ci.inboundConnCount)
			require.Equal(t, test.expectedOutbound, ci.outboundConnCount)
			require.Equal(t, test.expectedOutbound, ci.getOutboundConnCountByRange(rttRange))
		})
	}
}

func TestConnectPeerByKramaID(t *testing.T) {
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
					servers[i].AddPeerInfo(info)

					err := servers[i].ConnManager.connectPeerByKramaID(test.kramaID)

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
					servers[i].AddPeerInfo(info)

					servers[i].cfg.TrustedPeers = append(servers[i].cfg.TrustedPeers, config.NodeInfo{
						ID:      destination.id,
						Address: getMultiAddresses(t, []host.Host{destination.host})[0],
					})
				}

				// connect to trusted nodes
				servers[i].ConnManager.connectToTrustedNodes()

				for _, destination := range destinations {
					// get peer info of the node that we have connected to
					info := getPeerInfo(t, destination)

					checkConnection(t, servers[i], info.ID, true)
				}
			}
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
			err = server.ConnManager.connectToBootStrapNodes()

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

func TestCheckBootNodeConnections(t *testing.T) {
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

func TestGetPeers(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		2,
		2,
		true,
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
				setServerPeers(t, servers[1], servers[2].id, servers[3].id)
			},
			expectedList: []id.KramaID{servers[2].id, servers[3].id},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			fetchedList := test.server.ConnManager.getPeers()

			require.ElementsMatch(t, test.expectedList, fetchedList)
		})
	}
}

func TestConnectAndRegisterPeer_Connection_Failure(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 2)
	params := getParamsToCreateMultipleServers(
		t,
		4,
		bootNodes,
		1,
		8,
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
		source      *Server
		destination *Server
		rtt         int64
		testFn      func()
		expectedErr bool
	}{
		{
			name:        "source is connected to destination",
			source:      servers[0],
			destination: servers[1],
			rtt:         50,
			expectedErr: false,
		},
		{
			name:        "source couldn't connect with destination as the destination server is inactive",
			source:      servers[0],
			destination: servers[2],
			testFn: func() {
				err := servers[2].host.Close()
				require.NoError(t, err)
			},
			rtt:         150,
			expectedErr: true,
		},
		{
			name:        "source should fail to register if the peer already exists",
			source:      servers[0],
			destination: servers[3],
			testFn: func() {
				kPeer := openStream(t, servers[0], servers[3])
				err := servers[0].Peers.Register(kPeer)
				require.NoError(t, err)
			},
			rtt:         250,
			expectedErr: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			info := getPeerInfo(t, test.destination)

			err := test.source.ConnManager.ConnectAndRegisterPeer(*info, test.destination.id, test.rtt)

			if test.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// check whether the outbound connection is protected
			checkConnectionProtection(t, test.source, info.ID, true)
		})
	}
}

func TestConnectAndRegisterPeer_Connection_Limit(t *testing.T) {
	params := getParamsToCreateMultipleServers(
		t,
		4,
		nil,
		1,
		8,
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
		source      *Server
		destination *Server
		rtt         int64
		testFn      func()
		expectedErr error
	}{
		{
			name:        "source is connected to destination",
			source:      servers[0],
			destination: servers[1],
			rtt:         350,
			expectedErr: nil,
		},
		{
			name:        "source is already connected to max outbound peers in the given rtt range",
			source:      servers[0],
			destination: servers[2],
			rtt:         150,
			testFn: func() {
				servers[0].ConnManager.updateConnCount(network.DirOutbound, 150, 2)
			},
			expectedErr: common.ErrOutboundConnLimit,
		},
		{
			name:        "destination is already connected to max inbound peers",
			source:      servers[0],
			destination: servers[3],
			testFn: func() {
				servers[3].ConnManager.updateConnCount(network.DirInbound, 0, 1)
			},
			rtt:         50,
			expectedErr: errors.New("stream reset"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			info := getPeerInfo(t, test.destination)

			test.source.AddPeerInfo(info)

			err := test.source.ConnManager.ConnectAndRegisterPeer(*info, test.destination.id, test.rtt)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.True(t, test.source.Peers.ContainsPeer(test.destination.host.ID()))

			// check whether the oubound connection is protected
			checkConnectionProtection(t, test.source, info.ID, true)
		})
	}
}

func TestGetRandomNode_CheckInRoutingTable(t *testing.T) {
	t.Parallel()

	bootNodes := getBootstrapNodes(t, 2)
	addresses := getMultiAddresses(t, bootNodes)
	paramsMap := map[int]*CreateServerParams{}

	for i := 0; i < 5; i++ {
		paramsMap[i] = &CreateServerParams{
			EventMux: &utils.TypeMux{},
			ConfigCallback: func(c *config.NetworkConfig) {
				c.BootstrapPeers = addresses
				c.DiscoveryInterval = 200 * time.Millisecond
			},
			ServerCallback: func(s *Server) {
				m := NewMockReputationEngine()
				m.SetNTQ(s.GetKramaID(), tests.GetRandomNumber(t, 27))
				s.Senatus = m
			},
		}
	}

	servers := createMultipleServers(t, 5, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	startDiscovery(t, servers...)
	time.Sleep(500 * time.Millisecond)

	// check if random peer exists in routing table
	for i := 0; i < 5; i++ { // indicates server
		for j := 0; j < 3; j++ { // indicates no. of times random node fetched
			pID := servers[i].ConnManager.GetRandomPeer()
			routingTable := servers[i].kadDHT.RoutingTable()

			require.NotEqual(t, "", routingTable.Find(pID).String())
		}
	}
}

func TestGetPeerInfo(t *testing.T) {
	params := &CreateServerParams{
		EventMux: &utils.TypeMux{},
		ConfigCallback: func(c *config.NetworkConfig) {
			c.InboundConnLimit = 10
			c.OutboundConnLimit = 5
		},
		Logger: testLogger,
	}
	server := createServer(t, 0, params)

	testcases := []struct {
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
				server.ConnManager.AddToPeerStore(peerInfo)
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

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerInfo)
			}

			peerInfo, err := server.ConnManager.GetPeerInfo(test.peerInfo.ID)

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

func TestRetrieveRTTAndRefreshSenatus(t *testing.T) {
	bootNodes := getBootstrapNodes(t, 1)

	params := getParamsToCreateMultipleServers(
		t,
		5,
		bootNodes,
		1,
		2,
		false,
	)

	paramsMap := map[int]*CreateServerParams{
		0: params[0],
		1: params[1],
		2: params[2],
		3: params[3],
		4: params[4],
	}

	servers := createMultipleServers(t, 5, paramsMap)

	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	testCases := []struct {
		name          string
		source        *Server
		destination   *Server
		testFn        func(source *Server, dest *Server)
		expectedRTT   int64
		expectedError error
	}{
		{
			name:        "Peer info present in senatus",
			source:      servers[0],
			destination: servers[1],
			testFn: func(source *Server, dest *Server) {
				nodeInfo := &senatus.NodeMetaInfo{
					KramaID: dest.id,
					Addrs:   utils.MultiAddrToString(dest.host.Addrs()...),
					RTT:     250,
				}

				err := source.Senatus.AddNewPeerWithPeerID(dest.host.ID(), nodeInfo)
				require.NoError(t, err)
			},
			expectedRTT:   250,
			expectedError: nil,
		},
		{
			name:        "Incorrect peer info present in senatus",
			source:      servers[1],
			destination: servers[2],
			testFn: func(source *Server, dest *Server) {
				nodeInfo := &senatus.NodeMetaInfo{
					KramaID: dest.id,
					Addrs:   utils.MultiAddrToString(servers[0].host.Addrs()...),
					RTT:     900,
				}

				err := source.Senatus.AddNewPeerWithPeerID(dest.host.ID(), nodeInfo)
				require.NoError(t, err)
			},
			expectedRTT:   800,
			expectedError: nil,
		},
		{
			name:          "Peer info not present in senatus",
			source:        servers[2],
			destination:   servers[3],
			expectedRTT:   800,
			expectedError: nil,
		},
		{
			name:        "Inactive peer info not available in senatus",
			source:      servers[3],
			destination: servers[4],
			testFn: func(source *Server, dest *Server) {
				err := dest.host.Close()
				require.NoError(t, err)
			},
			expectedError: errors.New("routing: not found"),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.source, test.destination)
			}

			kramaID, rtt, err := test.source.ConnManager.retrieveRTTAndRefreshSenatus(peer.AddrInfo{
				ID:    test.destination.host.ID(),
				Addrs: test.destination.host.Addrs(),
			})

			if test.expectedError != nil {
				require.Error(t, test.expectedError)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.destination.id, kramaID)
			require.True(t, rtt <= test.expectedRTT)
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

	// check whether the inbound connection is protected
	checkConnectionProtection(t, servers[1], servers[0].host.ID(), true)
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

func TestStreamHandler_Connection_Reset(t *testing.T) {
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

	outboundPeer := openStream(t, servers[0], servers[1]) // connect to peer-1 and get peer

	// send handshake message
	err := outboundPeer.SendHandshakeMessage(servers[0])
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// check whether the inbound connection is protected
	checkConnectionProtection(t, servers[1], servers[0].host.ID(), true)

	inboundPeer := servers[1].Peers.Peer(servers[0].host.ID())

	// reset the connection
	err = servers[1].ConnManager.ResetStream(inboundPeer.stream, MOIStreamTag)
	require.NoError(t, err)

	// check whether the inbound connection is unprotected
	checkConnectionProtection(t, servers[1], servers[0].host.ID(), false)
}
