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

// clusterRegistry represents a registry of cluster IDs.
type clusterRegistry struct {
	clusters map[common.ClusterID]struct{}
}

// add adds a cluster id to the clusterRegistry.
func (cr *clusterRegistry) add(clusterID common.ClusterID) {
	cr.clusters[clusterID] = struct{}{}
}

// has checks if a given cluster id exists.
func (cr *clusterRegistry) has(clusterID common.ClusterID) bool {
	_, ok := cr.clusters[clusterID]

	return ok
}

// len returns the number of cluster id's stored in the clusterRegistry.
func (cr *clusterRegistry) len() int {
	return len(cr.clusters)
}

// remove removes a cluster id from the clusterRegistry.
func (cr *clusterRegistry) remove(clusterID common.ClusterID) {
	delete(cr.clusters, clusterID)
}

type icsPeer struct {
	ctx          context.Context
	kramaID      id.KramaID
	isDirectPeer bool
	networkID    peer.ID
	stream       p2pnet.Stream
	rw           bufio.ReadWriter
	logger       hclog.Logger
	msgChan      chan []byte
	clusters     *clusterRegistry
}

func newICSPeer(
	ctx context.Context,
	kramaID id.KramaID,
	stream p2pnet.Stream,
	logger hclog.Logger,
	isDirectPeer bool,
) *icsPeer {
	return &icsPeer{
		ctx:          ctx,
		kramaID:      kramaID,
		networkID:    stream.Conn().RemotePeer(),
		stream:       stream,
		rw:           *bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream)),
		logger:       logger.Named("Transport-icsPeer"),
		msgChan:      make(chan []byte, 50),
		isDirectPeer: isDirectPeer,
		clusters: &clusterRegistry{
			clusters: make(map[common.ClusterID]struct{}),
		},
	}
}

func (p *icsPeer) decodePeerMessage() (*types.ICSMSG, error) {
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

	msg.ReceivedFrom = p.kramaID

	return msg, nil
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
