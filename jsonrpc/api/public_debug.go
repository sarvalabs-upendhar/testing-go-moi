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
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/diagnosis"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/storage"
)

// PublicDebugAPI is the collection of APIs exposed over the public debugging endpoint
type PublicDebugAPI struct {
	db      backend.DB
	network backend.Network
	syncer  backend.Syncer
}

func NewPublicDebugAPI(db backend.DB, network backend.Network, syncer backend.Syncer) *PublicDebugAPI {
	// Create the public Debug API wrapper and return it
	return &PublicDebugAPI{
		db:      db,
		network: network,
		syncer:  syncer,
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

	return common.BytesToHex(content), nil
}

// getNodeMetaInfoByPeerID retrieves and returns node meta information from the database for the given peer id
func (p *PublicDebugAPI) getNodeMetaInfoByPeerID(peerID peer.ID) (*senatus.NodeMetaInfo, error) {
	value, err := p.db.ReadEntry(storage.SenatusDBKey(peerID))
	if err != nil {
		return nil, err
	}

	info := new(senatus.NodeMetaInfo)
	if err = info.FromBytes(value); err != nil {
		return nil, err
	}

	return info, nil
}

// NodeMetaInfo retrieves and returns the metadata of nodes from the database based on the provided
// peer id or krama id. If neither peer id nor krama id is provided, returns the metadata of all nodes.
func (p *PublicDebugAPI) NodeMetaInfo(args *rpcargs.NodeMetaInfoArgs) (
	map[string]rpcargs.NodeMetaInfoResponse,
	error,
) {
	var (
		peerID peer.ID
		err    error
	)

	nodeMetaInfo := make(map[string]rpcargs.NodeMetaInfoResponse)

	if args.PeerID != "" && args.KramaID != "" {
		return nil, common.ErrInvalidIDCombination
	}

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

		nodeMetaInfo[peerID.String()] = rpcargs.NodeMetaInfoResponse{
			Addrs:       info.Addrs,
			KramaID:     info.KramaID,
			RTT:         hexutil.Uint64(info.RTT),
			WalletCount: hexutil.Uint(info.WalletCount),
		}

		return nodeMetaInfo, nil
	}

	entriesChan, err := p.db.GetEntriesWithPrefix(context.Background(), storage.SenatusPrefix())
	if err != nil {
		return nil, err
	}

	for entry := range entriesChan {
		peerID := peer.ID(bytes.TrimPrefix(entry.Key, storage.SenatusPrefix()))

		info := new(senatus.NodeMetaInfo)
		if err = info.FromBytes(entry.Value); err != nil {
			continue
		}

		nodeMetaInfo[peerID.String()] = rpcargs.NodeMetaInfoResponse{
			Addrs:       info.Addrs,
			KramaID:     info.KramaID,
			RTT:         hexutil.Uint64(info.RTT),
			WalletCount: hexutil.Uint(info.WalletCount),
		}
	}

	return nodeMetaInfo, nil
}

// Accounts returns a list of registered account addresses
func (p *PublicDebugAPI) Accounts() ([]identifiers.Address, error) {
	return p.db.GetRegisteredAccounts()
}

// Connections returns a list of active connections and connection stats
func (p *PublicDebugAPI) Connections() (*rpcargs.ConnectionsResponse, error) {
	connections := make([]rpcargs.Connection, 0, len(p.network.GetConns()))

	for _, conn := range p.network.GetConns() {
		connections = append(connections, rpcargs.Connection{
			PeerID:  conn.RemotePeer().String(),
			Streams: getStreams(conn),
		})
	}

	return &rpcargs.ConnectionsResponse{
		Conns:              connections,
		InboundConnCount:   p.network.GetInboundConnCount(),
		OutboundConnCount:  p.network.GetOutboundConnCount(),
		ActivePubSubTopics: p.network.GetSubscribedTopics(),
	}, nil
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
func (p *PublicDebugAPI) RunDiagnosis(args *rpcargs.DiagnosisRequest) (*rpcargs.DiagnosisResponse, error) {
	profileDuration, err := time.ParseDuration(args.ProfileTime)
	if err != nil {
		return nil, errors.New("Invalid profile duration")
	}

	blockProfileDuration, err := time.ParseDuration(args.BlockProfileRate)
	if err != nil {
		return nil, errors.New("Invalid block profile rate")
	}

	if len(args.Collectors) == 0 {
		args.Collectors = diagnosis.DefaultCollectors
	}

	if _, err := os.Stat(args.OutputPath); err != nil {
		return nil, errors.New("Invalid output path")
	}

	return nil, diagnosis.WriteProfiles(context.Background(),
		filepath.Join(args.OutputPath, "go-moi"+time.Now().Format(utils.TimeFormat)+".zip"), diagnosis.Options{
			Collectors:           args.Collectors,
			ProfileDuration:      profileDuration,
			MutexProfileFraction: args.MutexProfileFraction,
			BlockProfileRate:     blockProfileDuration,
		},
	)
}

// SyncJob returns the sync job meta info for given address
func (p *PublicDebugAPI) SyncJob(args *rpcargs.SyncJobRequest) (*rpcargs.SyncJobInfo, error) {
	return p.syncer.GetSyncJobInfo(args.Address)
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
