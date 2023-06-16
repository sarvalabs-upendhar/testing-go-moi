package krama

import (
	"context"
	"time"

	"github.com/sarvalabs/moichain/mudra"

	"github.com/pkg/errors"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
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
	icsReq *ptypes.ICSRequest,
	response *ptypes.ICSResponse,
) error {
	var (
		canonicalICSReq = new(ptypes.CanonicalICSRequest)
		respChan        = make(chan Response)
		interactions    = new(types.Interactions)
	)

	response.StatusCode = ptypes.InternalError

	if err := canonicalICSReq.FromBytes(icsReq.ReqData); err != nil {
		icsrpc.engine.logger.Error("failed to depolorize canonical ics request ", err)

		return err
	}

	if err := interactions.FromBytes(canonicalICSReq.IxData); err != nil {
		icsrpc.engine.logger.Error("failed to depolorize interactions ", err)

		return err
	}

	if err := mudra.VerifySignatureUsingKramaID(
		id.KramaID(canonicalICSReq.Operator),
		icsReq.ReqData,
		icsReq.Signature,
	); err != nil {
		icsrpc.engine.logger.Error("failed to verify ics request signature ", err)

		return errors.Wrap(err, "failed to verify ics request signature")
	}

	kramaRequest := Request{
		slotType:     ktypes.ValidatorSlot,
		operator:     id.KramaID(canonicalICSReq.Operator),
		msg:          canonicalICSReq,
		responseChan: respChan,
		ixs:          *interactions,
		reqTime:      time.Unix(0, canonicalICSReq.Timestamp),
	}

	icsrpc.engine.requests <- kramaRequest
	// Wait for response from krama engine
	response.ClusterID = canonicalICSReq.ClusterID

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
