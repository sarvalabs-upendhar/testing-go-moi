package consensus

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

// ICSRPCService is a struct that represents an ICS RPC Service
type ICSRPCService struct {
	// Represents the ICS backend for the service
	engine KramaEngine
}

// NewICSRPCService is a constructor that generates and returns a new ICSRPCService
// object for a given ICS object
func NewICSRPCService(k KramaEngine) *ICSRPCService {
	return &ICSRPCService{k}
}

// ICSRequest is a method of ICSRPCService that sends an ICS join request
func (icsrpc *ICSRPCService) ICSRequest(
	ctx context.Context,
	icsReq *networkmsg.ICSRequest,
	response *networkmsg.ICSResponse,
) error {
	var (
		canonicalICSReq = new(networkmsg.CanonicalICSRequest)
		respChan        = make(chan Response)
		interactions    = new(common.Interactions)
	)

	response.StatusCode = networkmsg.InternalError

	if err := canonicalICSReq.FromBytes(icsReq.ReqData); err != nil {
		icsrpc.engine.Logger().Error("Failed to depolorize canonical ICS request", "err", err)

		return err
	}

	if err := interactions.FromBytes(canonicalICSReq.IxData); err != nil {
		icsrpc.engine.Logger().Error("Failed to depolorize interactions", "err", err)

		return err
	}

	if err := crypto.VerifySignatureUsingKramaID(
		id.KramaID(canonicalICSReq.Operator),
		icsReq.ReqData,
		icsReq.Signature,
	); err != nil {
		icsrpc.engine.Logger().Error("Failed to verify ICS request signature", "err", err)

		return errors.Wrap(err, "failed to verify ICS request signature")
	}

	for _, ix := range *interactions {
		rawPayload, err := ix.PayloadForSignature()
		if err != nil {
			return err
		}

		isVerified, err := crypto.Verify(rawPayload, ix.Signature(), ix.Sender().Bytes())
		if !isVerified || err != nil {
			return common.ErrInvalidIXSignature
		}
	}

	kramaRequest := Request{
		slotType:     ktypes.ValidatorSlot,
		operator:     id.KramaID(canonicalICSReq.Operator),
		msg:          canonicalICSReq,
		responseChan: respChan,
		ixs:          *interactions,
		reqTime:      time.Unix(0, canonicalICSReq.Timestamp),
	}

	icsrpc.engine.Requests() <- kramaRequest
	// Wait for response from krama engine
	response.ClusterID = canonicalICSReq.ClusterID

	if resp := <-respChan; resp.err != nil {
		switch resp.err.Error() {
		case common.ErrSlotsFull.Error():
			response.StatusCode = networkmsg.SlotsFull
		case common.ErrHashMismatch.Error():
			response.StatusCode = networkmsg.InvalidHash
		default:
			response.StatusCode = networkmsg.InternalError
		}

		return nil
	}

	response.StatusCode = networkmsg.Success

	return nil
}
