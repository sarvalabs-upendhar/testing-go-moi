package krama

import (
	"context"
	"log"
	"math/rand"
	"time"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/hashicorp/go-hclog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/moirpc"
	"github.com/sarvalabs/moichain/types"
)

const (
	MinimumConnectionCount = 3
	TesseractTopic         = "MOI_PUBSUB_TESSERACT"
)

const kramaMoirpcStreamTTL = time.Duration(2) * time.Minute

type network interface {
	Unsubscribe(topic string) error
	Broadcast(topic string, data []byte) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
	ConnectPeer(kramaID id.KramaID) error
	DisconnectPeer(kramaID id.KramaID) error
	InitNewRPCServer(protocol protocol.ID) *moirpc.Client
	RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error
	GetKramaID() id.KramaID
}

type Transport struct {
	logger    hclog.Logger
	rpcClient *moirpc.Client
	network   network
}

func NewKramaTransport(logger hclog.Logger, network network) *Transport {
	return &Transport{
		logger:  logger.Named("Krama-Transport"),
		network: network,
	}
}

func (t *Transport) RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error {
	t.rpcClient = t.network.InitNewRPCServer(serviceID)

	return t.network.RegisterNewRPCService(serviceID, serviceName, service)
}

func (t *Transport) InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error {
	var randomICSNodes []id.KramaID

	handler := func(msg *pubsub.Message) error {
		icsMsg := new(ktypes.ICSMSG)
		if err := polo.Depolorize(icsMsg, msg.GetData()); err != nil {
			return err
		}

		if slot == nil {
			t.logger.Error("slot not available for give cluster id", icsMsg.ClusterID)

			return errors.New("invalid cluster id")
		}

		slot.ForwardInboundMsg(icsMsg)

		return nil
	}

	if err := t.network.Subscribe(ctx, string(slot.ClusterID()), handler); err != nil {
		return errors.Wrap(err, "failed to subscribe")
	}

	// Check whether the slot is a validator slot
	if slot.SlotType == ktypes.ValidatorSlot {
		randomICSNodes = t.connectRandomPeers(slot)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := t.network.Unsubscribe(string(slot.ClusterID())); err != nil {
					log.Panicln(err)
				}

				// Check whether the slot is a validator slot and the randomICSNodes is not empty
				if slot.SlotType == ktypes.ValidatorSlot && len(randomICSNodes) != 0 {
					t.disconnectRandomPeers(randomICSNodes)
				}

				t.logger.Info("Closing cluster communication channels", "cluster-id", slot.ClusterID())

				return
			case msg, ok := <-slot.OutboundChan:
				if !ok {
					return
				}

				rawData, err := polo.Polorize(msg)
				if err != nil {
					t.logger.Error("Failed to polorize cluster message", "cluster-id", slot.ClusterID())
					panic(err)
				}

				if err := t.network.Broadcast(string(slot.ClusterID()), rawData); err != nil {
					t.logger.Error("Failed to broadcast cluster message", "cluster-id", slot.ClusterID())
					panic(err)
				}
			}
		}
	}()

	return nil
}

func (t *Transport) Call(kramaID id.KramaID, svcName, svcMethod string, args, response interface{}) error {
	if t.rpcClient == nil {
		return errors.New("rpc client not initiated")
	}

	return t.rpcClient.MoiCall(kramaID, svcName, svcMethod, args, response, kramaMoirpcStreamTTL)
}

func (t *Transport) BroadcastTesseract(msg *ptypes.TesseractMessage) error {
	rawData, err := polo.Polorize(msg)
	if err != nil {
		return err
	}

	return t.network.Broadcast(TesseractTopic, rawData)
}

func (t *Transport) connectRandomPeers(slot *ktypes.Slot) []id.KramaID {
	var randomICSNodes []id.KramaID

	clusterInfo := slot.CLusterInfo()

	icsNodes := clusterInfo.ICS.GetNodes()
	visitedNodes := make(map[int]interface{})

	/* If the icsNodes slice is not empty, then connect to the random ics nodes. Break the loop
	either on successfully establishing a connection with three random ics nodes or on failure to
	connect three random ics nodes even after looping through the entire icsNodes slice. In case
	if the size of the icsNodes is less than three, then connect to the available nodes. */
	counter := 0
	for len(visitedNodes) < len(icsNodes) && counter < MinimumConnectionCount {
		source := rand.NewSource(time.Now().UnixNano())
		reg := rand.New(source)
		index := reg.Intn(len(icsNodes))
		node := icsNodes[index]

		if _, ok := visitedNodes[index]; ok {
			continue
		}

		visitedNodes[index] = nil

		if err := t.network.ConnectPeer(node); err != nil {
			// If the node is already connected increment the counter
			if errors.Is(err, types.ErrConnectionExists) {
				counter++
			}

			continue
		}

		// As the connection is successful, increment the counter and append the node to randomICSNodes slice.
		counter++

		randomICSNodes = append(randomICSNodes, node)
	}

	return randomICSNodes
}

func (t *Transport) disconnectRandomPeers(randomICSNodes []id.KramaID) {
	// Disconnect the random peers which got connected while subscribing to the network
	for _, node := range randomICSNodes {
		if err := t.network.DisconnectPeer(node); err != nil {
			log.Panicln(err)
		}
	}
}
