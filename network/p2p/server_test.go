package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

const (
	message = "hello world"
	topic   = "pub-sub"
)

var testLogger = hclog.NewNullLogger()

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
				servers[test.index],
				*handShakeMsg,
				response,
			)

			if test.shouldAddPeer {
				peer := openStream(t, servers[test.index], servers[test.index+1])
				addPeer(t, servers[test.index], peer)
			}

			// send message from first server to second server
			err = servers[test.index].SendMessage(servers[test.index+1].host.ID(), networkmsg.HANDSHAKEMSG, handShakeMsg)
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
	testcases := []struct {
		name             string
		defaultValidator bool
		containsErrorMsg string
	}{
		{
			// Join method throws error
			name:             "With nil validator",
			defaultValidator: false,
			containsErrorMsg: "topic already exists",
		},
		{
			// RegisterTopicValidator method throws error
			name:             "With default validator",
			defaultValidator: true,
			containsErrorMsg: "duplicate validator for topic",
		},
	}

	for _, testcase := range testcases {
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

		registerEmptySubscriptionHandler(t, servers[0], topic, testcase.defaultValidator, false)
		err := servers[0].Subscribe(
			servers[0].ctx,
			topic,
			nil,
			testcase.defaultValidator,
			func(msg *pubsub.Message) error { // subscribing again on same topic
				return nil
			})
		require.ErrorContains(t, err, testcase.containsErrorMsg)
	}
}

func TestSubscribe_CheckMsgOnTopic(t *testing.T) {
	t.Parallel()

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
	registerEmptySubscriptionHandler(t, servers[0], topic, true, true) // shouldn't receive self-published message
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

	registerEmptySubscriptionHandler(t, server, topic, true, false)
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
	t.Parallel()

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
	registerEmptySubscriptionHandler(t, servers[0], topic, true, true)
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

	testcases := []struct {
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

	for _, test := range testcases {
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
	t.Parallel()

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
	registerEmptySubscriptionHandler(t, servers[0], config.SenatusTopic, true, true)
	subscribeHelloMsg(t, servers[1], config.SenatusTopic, servers[0], response)
	time.Sleep(1 * time.Second) // give time for discovery and subscription

	servers[0].SendHelloMessage()

	// wait till handler completes
	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	err := waitForResponse(ctx, response)
	require.NoError(t, err)
}
