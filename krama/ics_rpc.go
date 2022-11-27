package krama

import (
	"context"
	"errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/sarvalabs/moichain/types"
)

const (
	Slotsfull int64 = iota
	Hashmismatch
	Internalerror
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
	respChan := make(chan Response)

	interactions := new(types.Interactions)

	if err := interactions.FromBytes(req.IxData); err != nil {
		return errors.New("ixs decode error")
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
	// TODO: check for context
	if resp := <-respChan; resp.err != nil {
		response.Response = 0

		switch resp.err.Error() {
		case types.ErrSlotsFull.Error():
			response.StatusCode = Slotsfull
		case types.ErrHashMismatch.Error():
			response.StatusCode = Hashmismatch
		default:
			response.StatusCode = Internalerror
		}
	} else {
		response.Response = 1
		randomNodes, err := icsrpc.engine.getRandomNodes(ctx, 1, nil)
		if err != nil {
			return errors.New("unable to fetch random nodes")
		}
		response.RandomNodes = types.KIPPeerIDToString(randomNodes)
	}

	return nil
}
