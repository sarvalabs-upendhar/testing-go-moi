package p2p

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common/utils"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
)

func msgIDFunction(pmsg *pubsub_pb.Message) string {
	h := common.FastSum256(pmsg.Data)

	return base64.URLEncoding.EncodeToString(h[:])
}

// creates a custom gossipSub parameter set.
func pubsubGossipParam() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = 6
	gParams.D = 8
	gParams.Dhi = 12
	gParams.GossipFactor = 0.15
	gParams.HeartbeatInterval = 600 * time.Millisecond

	return gParams
}

// TopicSet is a wrapper for Topic & Subscription
type TopicSet struct {
	topicHandle *pubsub.Topic        // PubSub topic handler
	subHandle   *pubsub.Subscription // PubSub subscription handler
}

// pubSubTopics is a struct that represents a set of pub-sub topics sucscribed by the node
type pubSubTopics struct {
	psTopics     map[string]*TopicSet // map of PubSub topic names to respective TopicSet
	topicSetLock sync.RWMutex         // lock for the psTopics map
}

func (pst *pubSubTopics) addTopicSet(topicName string, topicSet *TopicSet) {
	pst.psTopics[topicName] = topicSet
}

func (pst *pubSubTopics) getTopicSet(topicName string) *TopicSet {
	return pst.psTopics[topicName]
}

func (pst *pubSubTopics) deleteTopicSet(topicName string) {
	delete(pst.psTopics, topicName)
}

func (s *Server) setPubSubRouter() error {
	var err error

	s.psRouter, err = pubsub.NewGossipSub(
		s.ctx,
		s.host,
		[]pubsub.Option{
			pubsub.WithGossipSubParams(pubsubGossipParam()),
			pubsub.WithMessageIdFn(msgIDFunction),
		}...,
	)
	if err != nil {
		return err
	}

	return nil
}

// Sets up the PubSub router for the node.
// Expects the node to already be configured with a libp2p host.
// Returns any error that occurs during the setup.
//
// Creates a new FloodSub router and map of topic sets for the node.
func (s *Server) setupPubSub() (err error) {
	if s.host == nil {
		return errors.New("libp2p host for node not configured")
	}

	// initialize an empty pubSubTopics
	s.pubSubTopics = &pubSubTopics{
		psTopics:     make(map[string]*TopicSet),
		topicSetLock: sync.RWMutex{},
	}

	// create a PubSub router for the Server with the pubsub options
	err = s.setPubSubRouter()
	if err != nil {
		return fmt.Errorf("failed to setup gossip sub %w", err)
	}

	return nil
}

// Broadcast is a method of Server that broadcasts a given polo message to a
// given PubSub topic. Expects the node to be subscribed to that topic.
func (s *Server) Broadcast(topicName string, data []byte) error {
	s.pubSubTopics.topicSetLock.RLock()
	defer s.pubSubTopics.topicSetLock.RUnlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)
	if topicSet == nil {
		return errors.New("topic not found")
	}

	// Attempt to publish the message to the pubsub topic
	if err := topicSet.topicHandle.Publish(s.ctx, data); err != nil {
		s.logger.Error("Failed to publish message", "topic", topicName, "err", err)
		// Return the error
		return err
	}

	return nil
}

// JoinPubSubTopic joins the pubsub topic and returns the TopicSet with topic handler only
// Note that this doesn't subscribe to the given topic, Call Subscribe for creating a subscription handler
func (s *Server) JoinPubSubTopic(topicName string) (*TopicSet, error) {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)
	if topicSet == nil {
		topic, err := s.psRouter.Join(topicName)
		if err != nil {
			return nil, err
		}

		s.pubSubTopics.addTopicSet(topicName, &TopicSet{topic, nil})
	}

	return s.pubSubTopics.getTopicSet(topicName), nil
}

func (s *Server) wrapAndReportValidation(topic string, v utils.WrappedVal) (string, pubsub.ValidatorEx) {
	return topic, func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (res pubsub.ValidationResult) {
		result, err := v(ctx, pid, msg)
		if err != nil {
			s.logger.Error("Error validating pubsub message", "err", err)
		}

		if result != pubsub.ValidationAccept {
			s.logger.Trace("Validation failed", "topic", topic, "validation-result", result)
		}

		return result
	}
}

// Subscribe subscribes the node to a given PubSub topic.
// Accepts the topic name to subscribe and handler function to handle messages from that subscription.
//
// Creates topic and subscription handles for the topic, wraps it in a TopicSet
// and adds it to the node's pubsub topicset. Creates a handler pipeline with the
// given handler function and starts a subscription loop that invokes the pipeline.
func (s *Server) Subscribe(
	ctx context.Context,
	topicName string,
	validator utils.WrappedVal,
	defaultValidator bool,
	handler func(msg *pubsub.Message) error,
) error {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	// custom validation
	if validator != nil {
		err := s.psRouter.RegisterTopicValidator(s.wrapAndReportValidation(topicName, validator))
		if err != nil {
			return err
		}
	}

	// default validation
	if defaultValidator {
		err := s.psRouter.RegisterTopicValidator(
			s.wrapAndReportValidation(
				topicName,
				func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
					return s.basicSeqnoValidator(ctx, pid, msg), nil
				},
			),
		)
		if err != nil {
			return err
		}
	}

	// Join pubsub topic and get a topic handle
	topicHandle, err := s.psRouter.Join(topicName)
	if err != nil {
		// Return the error
		return err
	}

	// Subscribe to the topic and get a subscription handle
	subcHandle, err := topicHandle.Subscribe(pubsub.WithBufferSize(60))
	if err != nil {
		// Return the error
		return err
	}

	s.pubSubTopics.addTopicSet(topicName, &TopicSet{topicHandle, subcHandle})

	go s.routeSubscriptionMessages(ctx, topicName, handler, subcHandle)

	return nil
}

// routeSubscriptionMessages listens to the messages over the subscription
// and calls the respective handler with message
func (s *Server) routeSubscriptionMessages(
	ctx context.Context,
	topicName string,
	handler func(msg *pubsub.Message) error,
	subcHandle *pubsub.Subscription,
) {
	pipeline := func(msg *pubsub.Message) {
		// Call the given subscription handler
		// an error because it is being invoked as a goroutine
		if err := handler(msg); err != nil {
			if !errors.Is(err, common.ErrAlreadyKnown) {
				s.logger.Error("Error handling pubsub message", "err", err)
			}

			return
		}
	}

	for {
		// Retrieve the next message from the subscription
		msg, err := subcHandle.Next(ctx)
		if err != nil {
			s.logger.Debug("Topic subscription closed", "topic", topicName)

			return
		}

		// Skip handling self published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		go pipeline(msg)
	}
}

func (s *Server) GetSubscribedTopics() map[string]int {
	topics := make(map[string]int)

	for _, topic := range s.psRouter.GetTopics() {
		topics[topic] = len(s.psRouter.ListPeers(topic))
	}

	return topics
}

// Unsubscribe is a method of Server that unsubscribes the node from a given PubSub topic
func (s *Server) Unsubscribe(topicName string) error {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)

	// Check if topic exists
	if topicSet == nil {
		return nil
	}

	// Cancel the subscription to the topic
	topicSet.subHandle.Cancel()

	// Attempt to close the topic handler for the topic
	if err := topicSet.topicHandle.Close(); err != nil {
		return err
	}

	s.pubSubTopics.deleteTopicSet(topicName)

	return nil
}
