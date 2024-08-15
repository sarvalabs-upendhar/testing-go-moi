package api

import (
	"encoding/hex"
	"errors"
	"math/big"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-polo"
)

var ErrGenesisAccount = errors.New("genesis account interactions forbidden")

// SendInteractions is a method of PublicIXAPI that stores the interaction
func (p *PublicCoreAPI) SendInteractions(sendIx *rpcargs.SendIX) (common.Hash, error) {
	sign, err := hex.DecodeString(sendIx.Signature)
	if err != nil {
		return common.NilHash, err
	}

	ixArgs, err := validateArgumentsWithSign(sendIx)
	if err != nil {
		return common.NilHash, err
	}

	ixn, err := constructInteraction(ixArgs, sign)
	if err != nil {
		return common.NilHash, err
	}

	// TODO Add validation to check for max ixn group size
	// add the interactions to ix pool
	errs := p.ixpool.AddLocalInteractions(common.Interactions{ixn})
	if len(errs) > 0 {
		return common.NilHash, errs[0]
	}

	return ixn.Hash(), nil
}

// helper function for moi.SendInteractions
func constructInteraction(args *common.SendIXArgs, sign []byte) (ix *common.Interaction, err error) {
	if args.FuelPrice == nil {
		return nil, common.ErrFuelPriceNotFound
	}

	if args.FuelLimit == 0 {
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

// helper function for moi.Call and moi.FuelEstimate
func constructIxn(sm backend.StateManager, args *common.SendIXArgs, sign []byte) (ix *common.Interaction, err error) {
	data := common.IxData{
		Input: common.IxInput{
			Type:            args.Type,
			Nonce:           args.Nonce,
			Sender:          args.Sender,
			Receiver:        args.Receiver,
			Payer:           args.Payer,
			TransferValues:  args.TransferValues,
			PerceivedValues: args.PerceivedValues,
			Payload:         args.Payload,
		},
	}

	if args.FuelPrice != nil {
		data.Input.FuelPrice = args.FuelPrice
	}

	if args.FuelLimit != 0 {
		data.Input.FuelLimit = args.FuelLimit
	}

	if data.Input.Type == common.IxLogicDeploy || data.Input.Type == common.IxAssetCreate {
		nonce, err := sm.GetNonce(args.Sender, common.NilHash)
		if err != nil {
			return nil, err
		}

		data.Input.Nonce = nonce
	}

	return common.NewInteraction(data, sign)
}

func createSendIXArgs(sendIx *rpcargs.IxArgs) (*common.SendIXArgs, error) {
	sendIXArgs := &common.SendIXArgs{
		Type:      sendIx.Type,
		Nonce:     0,
		Sender:    sendIx.Sender,
		Receiver:  sendIx.Receiver,
		Payer:     sendIx.Payer,
		FuelPrice: sendIx.FuelPrice.ToInt(),
		FuelLimit: uint64(sendIx.FuelLimit),
		Payload:   sendIx.Payload.Bytes(),
	}

	if len(sendIx.TransferValues) > 0 {
		sendIXArgs.TransferValues = make(map[identifiers.AssetID]*big.Int)
		for asset, amount := range sendIx.TransferValues {
			sendIXArgs.TransferValues[asset] = amount.ToInt()
		}
	}

	if len(sendIx.PerceivedValues) > 0 {
		sendIXArgs.PerceivedValues = make(map[identifiers.AssetID]*big.Int)
		for asset, amount := range sendIx.PerceivedValues {
			sendIXArgs.PerceivedValues[asset] = amount.ToInt()
		}
	}

	return sendIXArgs, nil
}
