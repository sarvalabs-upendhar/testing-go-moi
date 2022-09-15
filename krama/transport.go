package krama

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/krama/types"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"log"
)

type network interface {
	Unsubscribe(topic string) error
	Broadcast(topic string, data []byte) error
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
	InitNewRPCServer(protocol protocol.ID) *rpc.Client
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
	t.rpcClient = t.network.InitNewRPCServer(serviceID)

	return t.network.RegisterNewRPCService(serviceID, serviceName, service)
}

func (t *Transport) InitClusterCommunication(ctx context.Context, slot *types.Slot) error {
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

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := t.network.Unsubscribe(string(slot.ClusterID())); err != nil {
					log.Panicln(err)
				}

				t.logger.Info("Closing cluster communication channels", "cluster-id", slot.ClusterID())

				return
			case msg, ok := <-slot.OutboundChan:
				if !ok {
					return
				}

				if err := t.network.Broadcast(string(slot.ClusterID()), polo.Polorize(msg)); err != nil {
					t.logger.Error("Failed to broadcast cluster message", "cluster-id", slot.ClusterID())
					panic(err)
				}
			}
		}
	}()

	return nil
}

func (t *Transport) Call(peerId peer.ID, svcName, svcMethod string, args, response interface{}) error {
	if t.rpcClient == nil {
		return errors.New("rpc client not initiated")
	}

	return t.rpcClient.Call(peerId, svcName, svcMethod, args, response)
}
