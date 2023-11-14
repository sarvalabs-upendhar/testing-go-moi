package api

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/diagnosis"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/storage"
)

// PublicDebugAPI is the collection of APIs exposed over the public debugging endpoint
type PublicDebugAPI struct {
	db      DB
	network Network
}

func NewPublicDebugAPI(db DB, network Network) *PublicDebugAPI {
	// Create the public Debug API wrapper and return it
	return &PublicDebugAPI{
		db:      db,
		network: network,
	}
}

// DBGet returns the raw value of the key that is stored in the database
func (p *PublicDebugAPI) DBGet(args *rpcargs.DebugArgs) (string, error) {
	encodedData := common.FromHex(args.Key)

	// Read the value of the encodedData from the database
	content, err := p.db.ReadEntry(encodedData)
	if err != nil {
		return "", err
	}

	decodedData := common.BytesToHex(content)

	return decodedData, nil
}

// getNodeMetaInfoByPeerID retrieves and returns node meta information from the database for the given peer id
func (p *PublicDebugAPI) getNodeMetaInfoByPeerID(peerID peer.ID) (*senatus.NodeMetaInfo, error) {
	value, err := p.db.ReadEntry(storage.NtqDBKey(peerID))
	if err != nil {
		return nil, err
	}

	info := new(senatus.NodeMetaInfo)
	if err = info.FromBytes(value); err != nil {
		return nil, err
	}

	return info, nil
}

// GetNodeMetaInfo retrieves and returns the metadata of nodes from the database based on the provided
// peer id or krama id. If neither peer id nor krama id is provided, returns the metadata of all nodes.
func (p *PublicDebugAPI) GetNodeMetaInfo(args *rpcargs.NodeMetaInfoArgs) (map[string]map[string]interface{}, error) {
	var (
		peerID peer.ID
		err    error
	)

	nodeMetaInfo := make(map[string]map[string]interface{})

	if args.PeerID != "" {
		if peerID, err = peer.Decode(args.PeerID); err != nil {
			return nil, err
		}
	}

	if args.KramaID != "" {
		if peerID, err = args.KramaID.DecodedPeerID(); err != nil {
			return nil, err
		}
	}

	if peerID != "" {
		info, err := p.getNodeMetaInfoByPeerID(peerID)
		if err != nil {
			return nil, err
		}

		nodeMetaInfo[peerID.String()] = map[string]interface{}{
			"addrs":        info.Addrs,
			"krama_id":     info.KramaID,
			"rtt":          hexutil.Uint64(info.RTT),
			"wallet_count": hexutil.Uint(info.WalletCount),
		}

		return nodeMetaInfo, nil
	}

	entriesChan, err := p.db.GetEntriesWithPrefix(context.Background(), []byte{storage.NTQ.Byte()})
	if err != nil {
		return nil, err
	}

	for entry := range entriesChan {
		peerID := peer.ID(bytes.TrimPrefix(entry.Key, []byte{storage.NTQ.Byte()}))

		info := new(senatus.NodeMetaInfo)
		if err = info.FromBytes(entry.Value); err != nil {
			continue
		}

		nodeMetaInfo[peerID.String()] = map[string]interface{}{
			"addrs":        info.Addrs,
			"krama_id":     info.KramaID,
			"rtt":          hexutil.Uint64(info.RTT),
			"wallet_count": hexutil.Uint(info.WalletCount),
		}
	}

	return nodeMetaInfo, nil
}

// GetAccounts returns a list of registered account addresses
func (p *PublicDebugAPI) GetAccounts() ([]common.Address, error) {
	return p.db.GetRegisteredAccounts()
}

// GetConnections returns a list of active connections and connection stats
func (p *PublicDebugAPI) GetConnections() rpcargs.ConnectionsResponse {
	connections := make([]rpcargs.Connection, 0, len(p.network.GetConns()))

	for _, conn := range p.network.GetConns() {
		connections = append(connections, rpcargs.Connection{
			PeerID:  conn.RemotePeer().String(),
			Streams: getStreams(conn),
		})
	}

	return rpcargs.ConnectionsResponse{
		Conns:              connections,
		InboundConnCount:   p.network.GetInboundConnCount(),
		OutboundConnCount:  p.network.GetOutboundConnCount(),
		ActivePubSubTopics: p.network.GetSubscribedTopics(),
	}
}

/*
RunDiagnosis runs the performance profiler and generate a report in a single zip.
Report includes:
- A list of running goroutines.
- A CPU profile.
- A heap profile.
- A mutex profile.
- A block profile.
- The version file.

Generated report will be stored at {outpath}/go-moi[timestamp].zip
These profiles can be monitored using pprof, reference documentation for more help
*/
func (p *PublicDebugAPI) RunDiagnosis(args *rpcargs.DiagnosisRequest) error {
	profileDuration, err := time.ParseDuration(args.ProfileTime)
	if err != nil {
		return errors.New("Invalid profile duration")
	}

	blockProfileDuration, err := time.ParseDuration(args.BlockProfileRate)
	if err != nil {
		return errors.New("Invalid block profile rate")
	}

	if len(args.Collectors) == 0 {
		args.Collectors = diagnosis.DefaultCollectors
	}

	if _, err := os.Stat(args.OutputPath); err != nil {
		return errors.New("Invalid output path")
	}

	return diagnosis.WriteProfiles(context.Background(),
		filepath.Join(args.OutputPath, "go-moi"+time.Now().Format(utils.TimeFormat)+".zip"), diagnosis.Options{
			Collectors:           args.Collectors,
			ProfileDuration:      profileDuration,
			MutexProfileFraction: args.MutexProfileFraction,
			BlockProfileRate:     blockProfileDuration,
		})
}

// helper functions

// getStreams retrieves stream information from a network connection
func getStreams(conn network.Conn) []rpcargs.Stream {
	streams := make([]rpcargs.Stream, 0, len(conn.GetStreams()))

	for _, stream := range conn.GetStreams() {
		streams = append(streams, rpcargs.Stream{
			Protocol:  stream.Protocol(),
			Direction: int(stream.Stat().Direction),
		})
	}

	return streams
}
