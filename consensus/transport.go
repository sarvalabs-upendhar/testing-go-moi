package consensus

import (
	"context"
	"fmt"
	"log"

	"github.com/sarvalabs/go-moi/common/config"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/telemetry/tracing"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"

	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/rpc"
)

const (
	MinimumConnectionCount = 3
	kramaMoirpcStreamTTL   = 0
)

type network interface {
	Unsubscribe(topic string) error
	Broadcast(topic string, data []byte) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
	StartNewRPCServer(protocol protocol.ID, tag string) *rpc.Client
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
	t.rpcClient = t.network.StartNewRPCServer(serviceID, serviceName)

	return t.network.RegisterNewRPCService(serviceID, serviceName, service)
}

func (t *Transport) InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error {
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

	go func() {
		defer func() {
			if err := t.network.Unsubscribe(string(slot.ClusterID())); err != nil {
				log.Println(err)
			}

			t.logger.Info("Closing cluster communication channels", "cluster-ID", slot.ClusterID())
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-slot.OutboundChan:
				if !ok {
					return
				}

				rawData, err := msg.Bytes()
				if err != nil {
					t.logger.Error("Failed to polorize cluster message", "cluster-ID", slot.ClusterID())

					return
				}

				if err := t.network.Broadcast(string(slot.ClusterID()), rawData); err != nil {
					t.logger.Error("Failed to broadcast cluster message", "cluster-ID", slot.ClusterID())

					return
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

	return t.network.Broadcast(config.TesseractTopic, rawData)
}
