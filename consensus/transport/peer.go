package transport

import (
	"bufio"
	"context"
	"fmt"

	"github.com/hashicorp/go-hclog"
	p2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-msgio"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

type icsInboundPeer struct {
	networkID peer.ID
	stream    p2pnet.Stream
	KramaID   id.KramaID
}

func newICSInboundPeer(stream p2pnet.Stream) *icsInboundPeer {
	return &icsInboundPeer{
		networkID: stream.Conn().RemotePeer(),
		stream:    stream,
	}
}

func (p *icsInboundPeer) decodePeerMessage() (*types.ICSMSG, error) {
	// Use msgio.NewReader to create a new message reader
	reader := msgio.NewReader(p.stream)

	// Read the message from the stream
	buffer, err := reader.ReadMsg()
	if err != nil {
		return nil, err
	}

	// Create a new ICSMSG instance
	msg := new(types.ICSMSG)

	// Parse the message from the buffer
	err = msg.FromBytes(buffer)
	if err != nil {
		return nil, err
	}

	msg.ReceivedFrom = p.KramaID

	return msg, nil
}

type icsOutboundPeer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	kramaID   id.KramaID
	stream    p2pnet.Stream
	rw        bufio.ReadWriter
	logger    hclog.Logger
	msgChan   chan []byte
}

func newICSOutboundPeer(
	ctx context.Context,
	kramaID id.KramaID,
	stream p2pnet.Stream,
	logger hclog.Logger,
) *icsOutboundPeer {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &icsOutboundPeer{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		kramaID:   kramaID,
		stream:    stream,
		rw:        *bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream)),
		logger:    logger.Named("Transport-icsOutboundPeer"),
		msgChan:   make(chan []byte, 1000),
	}
}

func shipMessage(rw *bufio.ReadWriter, data []byte) error {
	writer := msgio.NewWriter(rw.Writer)
	if err := writer.WriteMsg(data); err != nil {
		return err
	}

	return rw.Writer.Flush()
}

func GenerateWireMessage(
	sender id.KramaID,
	clusterID common.ClusterID,
	msgType networkmsg.MsgType,
	payload networkmsg.Payload,
) ([]byte, error) {
	rawPayload, err := payload.Bytes()
	if err != nil {
		return nil, fmt.Errorf("polorize message payload %w", err)
	}

	icsMsg := types.NewICSMsg(sender, clusterID, msgType, rawPayload)

	rawData, err := icsMsg.Bytes()
	if err != nil {
		return nil, err
	}

	return rawData, nil
}

// send bundles the given payload into a network message and ship it to the peer
func (p *icsOutboundPeer) send(
	sender id.KramaID,
	clusterID common.ClusterID,
	msgType networkmsg.MsgType,
	payload types.ICSPayload,
) error {
	rawMsg, err := GenerateWireMessage(sender, clusterID, msgType, payload)
	if err != nil {
		return err
	}

	p.msgChan <- rawMsg

	return nil
}

func (p *icsOutboundPeer) handleMessage() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case msg := <-p.msgChan:
			if err := shipMessage(&p.rw, msg); err != nil {
				p.logger.Trace("Failed to ship message", "err", err)
			}
		}
	}
}

func (p *icsOutboundPeer) close() {
	p.ctxCancel()
}
