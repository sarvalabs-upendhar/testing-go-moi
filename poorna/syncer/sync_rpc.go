package syncer

import (
	"context"
	"errors"
	"github.com/libp2p/go-libp2p-core/peer"
	gorpc "github.com/libp2p/go-libp2p-gorpc"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/poorna/api"
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
	req *ktypes.AccountsStatusMsg,
	resp *api.Response,
) error {
	return syncRPC.syncer.StatusUpdate(ctx.Value(gorpc.ContextKeyRequestSender).(peer.ID), req)
}

// GetTesseract is an RPC call to fetch the tesseract based on hash or height
func (syncRPC *SYNCRPCService) GetTesseract(
	ctx context.Context,
	req *ktypes.TesseractReq,
	resp *ktypes.TesseractResponse,
) error {
	tesseract, err := syncRPC.syncer.GetTesseract(req.Hash)
	if err != nil {
		return err
	}

	icsData, err := syncRPC.syncer.db.ReadEntry(tesseract.Body.ConsensusProof.ICSHash.Bytes())
	if err != nil {
		return errors.New("error fetching ICS Info")
	}

	rawData := tesseract.Bytes()
	resp.Data = rawData
	resp.Delta = map[ktypes.Hash][]byte{tesseract.Body.ConsensusProof.ICSHash: icsData}

	return nil
}
