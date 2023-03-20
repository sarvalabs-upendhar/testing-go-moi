package krama

import (
	"context"
	"errors"

	"github.com/sarvalabs/go-polo"

	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// ICSRPCService is a struct that represents an ICS RPC Service
type ICSRPCService struct {
	// Represents the ICS backend for the service
	engine *Engine
}

// NewICSRPCService is a constructor that generates and returns a new ICSRPCService
// object for a given ICS object
func NewICSRPCService(k *Engine) *ICSRPCService {
	return &ICSRPCService{k}
}

// ICSRequest is a method of ICSRPCService that sends an ICS join request
func (icsrpc *ICSRPCService) ICSRequest(
	ctx context.Context,
	req *ptypes.ICSRequest,
	response *ptypes.ICSResponse,
) error {
	var (
		respChan     = make(chan Response)
		interactions = new(types.Interactions)
	)

	if err := interactions.FromBytes(req.IxData); err != nil {
		if !errors.Is(err, polo.ErrNullPack) {
			return errors.New("ixs decode error")
		}
	}

	kramaRequest := Request{
		reqType:      1,
		msg:          req,
		responseChan: respChan,
		ixs:          *interactions,
	}

	icsrpc.engine.requests <- kramaRequest
	// Wait for response from krama engine
	response.ClusterID = req.ClusterID

	if resp := <-respChan; resp.err != nil {
		switch resp.err.Error() {
		case types.ErrSlotsFull.Error():
			response.StatusCode = ptypes.SlotsFull
		case types.ErrHashMismatch.Error():
			response.StatusCode = ptypes.InvalidHash
		default:
			response.StatusCode = ptypes.InternalError
		}

		return nil
	}

	response.StatusCode = ptypes.Success

	return nil
}
