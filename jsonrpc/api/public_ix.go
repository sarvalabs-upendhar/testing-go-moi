package api

import (
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

var ErrGenesisAccount = errors.New("genesis account interactions forbidden")

// PublicIXAPI is a struct that represents a wrapper for the public interaction APIs.
type PublicIXAPI struct {
	// Represents the API backend
	ixpool IxPool
	sm     StateManager
}

func NewPublicIXAPI(ixpool IxPool, sm StateManager) *PublicIXAPI {
	// Create the public interaction API wrapper and return it
	return &PublicIXAPI{ixpool, sm}
}

// SendInteraction is a method of PublicIXAPI that stores the interaction
func (p *PublicIXAPI) SendInteraction(sendIx *rpcargs.SendIX) (*common.Interaction, error) {
	sign, err := hex.DecodeString(sendIx.Signature)
	if err != nil {
		return nil, err
	}

	ixArgs, err := validateArgumentsWithSign(sendIx)
	if err != nil {
		return nil, err
	}

	ixn, err := constructInteraction(ixArgs, sign)
	if err != nil {
		return nil, err
	}

	// add the interactions to ix pool
	errs := p.ixpool.AddInteractions(common.Interactions{ixn})
	if len(errs) > 0 {
		return nil, errs[0]
	}

	return ixn, nil
}

// helper function
func constructInteraction(args *common.SendIXArgs, sign []byte) (ix *common.Interaction, err error) {
	if args.FuelPrice == nil {
		return nil, common.ErrFuelPriceNotFound
	}

	if args.FuelLimit == nil {
		return nil, common.ErrFuelLimitNotFound
	}

	data := common.IxData{
		Input: common.IxInput{
			Type:            args.Type,
			Nonce:           args.Nonce,
			Sender:          args.Sender,
			Receiver:        args.Receiver,
			Payer:           args.Payer,
			TransferValues:  args.TransferValues,
			PerceivedValues: args.PerceivedValues,
			FuelPrice:       args.FuelPrice,
			FuelLimit:       args.FuelLimit,
			Payload:         args.Payload,
		},
	}

	return common.NewInteraction(data, sign)
}

// validateArgumentsWithSign checks whether the IXArgs are valid or not
func validateArgumentsWithSign(args *rpcargs.SendIX) (*common.SendIXArgs, error) {
	bz, err := hex.DecodeString(args.IXArgs)
	if err != nil {
		return nil, err
	}

	ixArgs := new(common.SendIXArgs)

	err = polo.Depolorize(ixArgs, bz)
	if err != nil {
		return nil, err
	}

	if ixArgs.Sender.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	if ixArgs.Sender == ixArgs.Receiver {
		return nil, common.ErrInvalidIxParticipants
	}

	// Reject genesis account interaction
	if ixArgs.Sender == common.SargaAddress {
		return nil, ErrGenesisAccount
	}

	if !ixArgs.Receiver.IsNil() {
		// Reject genesis account interaction
		if ixArgs.Receiver == common.SargaAddress {
			return nil, ErrGenesisAccount
		}
	}

	// TODO: Add more checks to validate inputs

	return ixArgs, nil
}
