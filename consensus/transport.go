package consensus

import (
	"context"
	"fmt"
	"log"

	id "github.com/sarvalabs/moichain/common/kramaid"
	networkmsg "github.com/sarvalabs/moichain/network/message"
	"github.com/sarvalabs/moichain/telemetry/tracing"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"

	ktypes "github.com/sarvalabs/moichain/consensus/types"
	"github.com/sarvalabs/moichain/network/rpc"
)

const (
	MinimumConnectionCount = 3
	kramaMoirpcStreamTTL   = 0
	TesseractTopic         = "MOI_PUBSUB_TESSERACT"
)

type network interface {
	Unsubscribe(topic string) error
	Broadcast(topic string, data []byte) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
	ConnectPeer(krama id.KramaID) error
	DisconnectPeer(kramaID id.KramaID) error
	StartNewRPCServer(protocol protocol.ID) *rpc.Client
	RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error
	GetKramaID() id.KramaID
}

type Transport struct {
	logger    hclog.Logger
	rpcClient *rpc.Client
	network   network
}

func NewKramaTransport(logger hclog.Logger, network network) *Transport {
	return &Transport{
		logger:  logger.Named("Krama-Transport"),
		network: network,
	}
}

func (t *Transport) RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error {
	t.rpcClient = t.network.StartNewRPCServer(serviceID)

	return t.network.RegisterNewRPCService(serviceID, serviceName, service)
}

func (t *Transport) InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error {
	var randomICSNodes []id.KramaID

	handler := func(msg *pubsub.Message) error {
		icsMsg := new(ktypes.ICSMSG)
		if err := icsMsg.FromBytes(msg.GetData()); err != nil {
			return err
		}

		if slot == nil {
			t.logger.Error("Slot not available for give cluster ID", "cluster-ID", icsMsg.ClusterID)

			return errors.New("invalid cluster id")
		}

		slot.ForwardInboundMsg(icsMsg)

		return nil
	}

	if err := t.network.Subscribe(ctx, string(slot.ClusterID()), handler); err != nil {
		return errors.Wrap(err, "failed to subscribe")
	}

	/*
	   TODO: This is causing delay in ICS creation, need to improve this
	   // Check whether the slot is a validator slot
	   	if slot.SlotType == ktypes.ValidatorSlot {
	   		randomICSNodes = t.connectRandomPeers(ctx, slot)
	   	}
	*/

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := t.network.Unsubscribe(string(slot.ClusterID())); err != nil {
					log.Println(err)
				}

				// Check whether the slot is a validator slot and the randomICSNodes is not empty
				if slot.SlotType == ktypes.ValidatorSlot && len(randomICSNodes) != 0 {
					t.disconnectRandomPeers(randomICSNodes)
				}

				t.logger.Info("Closing cluster communication channels", "cluster-ID", slot.ClusterID())

				return
			case msg, ok := <-slot.OutboundChan:
				if !ok {
					return
				}

				rawData, err := msg.Bytes()
				if err != nil {
					t.logger.Error("Failed to polorize cluster message", "cluster-ID", slot.ClusterID())
					panic(err)
				}

				if err := t.network.Broadcast(string(slot.ClusterID()), rawData); err != nil {
					t.logger.Error("Failed to broadcast cluster message", "cluster-ID", slot.ClusterID())
					panic(err)
				}
			}
		}
	}()

	return nil
}

func (t *Transport) Call(
	ctx context.Context,
	kramaID id.KramaID,
	svcName, svcMethod string,
	args, response interface{},
) error {
	if t.rpcClient == nil {
		return errors.New("rpc client not initiated")
	}

	_, span := tracing.Span(ctx, "KramaEngine", fmt.Sprintf("RPC call to %s", kramaID))
	defer func() {
		span.End()
	}()

	return t.rpcClient.MoiCall(ctx, kramaID, svcName, svcMethod, args, response, kramaMoirpcStreamTTL)
}

func (t *Transport) BroadcastTesseract(msg *networkmsg.TesseractMessage) error {
	rawData, err := msg.Bytes()
	if err != nil {
		return err
	}

	return t.network.Broadcast(TesseractTopic, rawData)
}

/*
func (t *Transport) connectRandomPeers(ctx context.Context, slot *ktypes.Slot) []id.KramaID {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "connectRandomPeers")
	defer span.End()

	var randomICSNodes []id.KramaID

	clusterInfo := slot.ClusterState()

	icsNodes := clusterInfo.NodeSet.GetNodes()
	visitedNodes := make(map[int]interface{})

	/* If the icsNodes slice is not empty, then connect to the random ics nodes. Break the loop
	either on successfully establishing a connection with three random ics nodes or on failure to
	connect three random ics nodes even after looping through the entire icsNodes slice. In case
	if the size of the icsNodes is less than three, then connect to the available nodes.
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
*/

func (t *Transport) disconnectRandomPeers(randomICSNodes []id.KramaID) {
	// Disconnect the random peers which got connected while subscribing to the network
	for _, node := range randomICSNodes {
		if err := t.network.DisconnectPeer(node); err != nil {
			log.Panicln(err)
		}
	}
}
