package krama

import (
	"context"
	"errors"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/polo/go-polo"
)

const (
	SLOTSFULL int64 = iota
	HASHMISMATCH
	INTERNALERROR
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
	req *ktypes.ICSRequest,
	response *ktypes.ICSResponse,
) error {
	respChan := make(chan Response)

	interactions := new(ktypes.Interactions)

	if err := polo.Depolorize(interactions, req.IxData); err != nil {
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
		switch resp.err {
		case ktypes.ErrSlotsFull:
			response.Response = 0
			response.StatusCode = SLOTSFULL
		case ktypes.ErrHashMismatch:
			response.Response = 0
			response.StatusCode = HASHMISMATCH
		default:
			response.Response = 0
			response.StatusCode = INTERNALERROR
		}
	} else {
		response.Response = 1
		randomNodes, err := icsrpc.engine.getRandomNodes(1, nil)
		if err != nil {
			return errors.New("unable to fetch random nodes")
		}
		response.RandomNodes = ktypes.KIPPeerIDToString(randomNodes)
	}

	return nil
}
