package syncer

import (
	"context"
	"errors"

	gorpc "github.com/libp2p/go-libp2p-gorpc"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/sarvalabs/moichain/poorna/api"
	"gitlab.com/sarvalabs/moichain/types"
)

// SYNCRPCService is a struct that represents an SYNC RPC Service
type SYNCRPCService struct {
	syncer *Syncer
}

func NewSyncRPCService(syncer *Syncer) *SYNCRPCService {
	return &SYNCRPCService{syncer}
}

// StatusUpdate is an RPC call to update the status of the remote peer
func (syncRPC *SYNCRPCService) StatusUpdate(
	ctx context.Context,
	req *types.AccountsStatusMsg,
	resp *api.Response,
) error {
	if peerID, ok := ctx.Value(gorpc.ContextKeyRequestSender).(peer.ID); ok {
		return syncRPC.syncer.StatusUpdate(peerID, req)
	}

	return errors.New("failed to retrieve the peer ID of current request sender")
}

// GetTesseract is an RPC call to fetch the tesseract based on hash or height
func (syncRPC *SYNCRPCService) GetTesseract(
	ctx context.Context,
	req *types.TesseractReq,
	resp *types.TesseractResponse,
) error {
	tesseract, err := syncRPC.syncer.GetTesseract(req.Hash, req.WithInteractions)
	if err != nil {
		return err
	}

	icsData, err := syncRPC.syncer.db.ReadEntry(tesseract.Body.ConsensusProof.ICSHash.Bytes())
	if err != nil {
		return errors.New("error fetching ICS Info")
	}

	rawData := tesseract.Bytes()
	resp.Data = rawData
	resp.Delta = map[types.Hash][]byte{tesseract.Body.ConsensusProof.ICSHash: icsData}

	return nil
}
