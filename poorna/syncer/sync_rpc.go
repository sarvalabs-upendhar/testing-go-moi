package syncer

import (
	"context"
	"errors"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/moichain/poorna/moirpc"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
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
	req *ptypes.AccountsStatusMsg,
	resp *ptypes.Response,
) error {
	peerID, ok := ctx.Value(moirpc.ContextKeyRequestSender).(peer.ID)
	if !ok {
		syncRPC.syncer.logger.Error("type assertion failed")

		return types.ErrInterfaceConversion
	}

	return syncRPC.syncer.StatusUpdate(peerID, req)
}

// GetTesseract is an RPC call to fetch the tesseract based on hash or height
func (syncRPC *SYNCRPCService) GetTesseract(
	ctx context.Context,
	req *ptypes.TesseractReq,
	resp *TesseractResponse,
) error {
	tesseract, err := syncRPC.syncer.GetTesseract(req.Hash, req.WithInteractions)
	if err != nil {
		return err
	}

	icsData, err := syncRPC.syncer.db.ReadEntry(tesseract.Body.ConsensusProof.ICSHash.Bytes())
	if err != nil {
		return errors.New("error fetching ICS Info")
	}

	rawData, err := tesseract.Bytes()
	if err != nil {
		return err
	}

	resp.Data = rawData
	resp.Delta = map[types.Hash][]byte{tesseract.Body.ConsensusProof.ICSHash: icsData}

	return nil
}
