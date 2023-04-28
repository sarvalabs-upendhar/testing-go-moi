package api

import (
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
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
func (p *PublicIXAPI) SendInteraction(args *ptypes.SendIXArgs) (*types.Interaction, error) {
	err := validateArguments(args, p)
	if err != nil {
		return nil, err
	}

	nonce, err := p.ixpool.GetNonce(args.Sender)
	if err != nil {
		return nil, err
	}

	ixn, err := constructInteraction(args, nonce)
	if err != nil {
		return nil, err
	}

	// add the interactions to ix pool
	errs := p.ixpool.AddInteractions(types.Interactions{ixn})
	if len(errs) > 0 {
		return ixn, errs[0]
	}

	return ixn, nil
}

// helper function
func constructInteraction(args *ptypes.SendIXArgs, nonce uint64) (ix *types.Interaction, err error) {
	data := types.IxData{
		Input: types.IxInput{
			Type:           args.Type,
			Nonce:          nonce,
			Sender:         args.Sender,
			Receiver:       args.Receiver,
			TransferValues: make(map[types.AssetID]*big.Int, len(args.TransferValues)),
			FuelPrice:      args.FuelPrice.ToInt(),
			FuelLimit:      args.FuelLimit.ToInt(),
		},
	}

	switch args.Type {
	case types.IxValueTransfer:
		// Decode the transfer values
		for asset, value := range args.TransferValues {
			data.Input.TransferValues[asset] = value.ToInt()
		}

	case types.IxAssetCreate:
		data.Input.Payload, err = GetRawIXPayloadForAssetCreation(args.Payload)
		if err != nil {
			return nil, err
		}

	case types.IxLogicDeploy:
		data.Input.Payload, err = GetRawIXPayloadForLogicDeploy(args.Payload, nonce, data.Input.Sender)
		if err != nil {
			return nil, err
		}

	case types.IxLogicInvoke:
		data.Input.Payload, err = GetRawIXPayloadForLogicInvoke(args.Payload)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return types.NewInteraction(data, nil), nil
}

// ValidateArguments checks whether the SendIXArgs are valid or not
func validateArguments(args *ptypes.SendIXArgs, p *PublicIXAPI) error {
	if args.Sender.IsNil() {
		return types.ErrInvalidAddress
	}

	// Reject genesis account interaction
	if args.Sender == types.SargaAddress {
		return ErrGenesisAccount
	}

	if !args.Receiver.IsNil() {
		// Reject genesis account interaction
		if args.Receiver == types.SargaAddress {
			return ErrGenesisAccount
		}
	}

	// TODO: Add more checks to validate inputs

	return nil
}

// GetRawIXPayloadForAssetCreation returns the raw IXPayload for asset creation
func GetRawIXPayloadForAssetCreation(jsonPayload []byte) ([]byte, error) {
	payloadArgs := new(ptypes.RPCAssetCreation)
	if err := json.Unmarshal(jsonPayload, payloadArgs); err != nil {
		return nil, err
	}

	createPayload := &types.AssetCreatePayload{
		Type:   payloadArgs.Type,
		Symbol: payloadArgs.Symbol,
		Supply: payloadArgs.Supply.ToInt(),

		Dimension: payloadArgs.Dimension.ToInt(),
		Decimals:  payloadArgs.Decimals.ToInt(),

		IsFungible:     payloadArgs.IsFungible,
		IsMintable:     payloadArgs.IsMintable,
		IsTransferable: payloadArgs.IsTransferable,

		LogicID: types.LogicID(payloadArgs.LogicID),
		// LogicCode: payloadArgs.LogicCode,
	}

	assetPayload := &types.AssetPayload{
		Create: createPayload,
	}

	return polo.Polorize(assetPayload)
}

// GetRawIXPayloadForLogicDeploy returns the raw IXPayload for logic deployment
func GetRawIXPayloadForLogicDeploy(jsonPayload []byte, nonce uint64, sender types.Address) ([]byte, error) {
	payload := new(ptypes.RPCLogicPayload)
	if err := json.Unmarshal(jsonPayload, payload); err != nil {
		return nil, err
	}

	if len(payload.Manifest) == 0 {
		return nil, types.ErrEmptyManifest
	}

	return polo.Polorize(&types.LogicPayload{
		Callsite: payload.Callsite,
		Calldata: payload.Calldata.Bytes(),
		Manifest: payload.Manifest.Bytes(),
	})
}

// GetRawIXPayloadForLogicInvoke returns the raw IXPayload for logic invoke
func GetRawIXPayloadForLogicInvoke(jsonPayload []byte) ([]byte, error) {
	payload := new(ptypes.RPCLogicPayload)
	if err := json.Unmarshal(jsonPayload, payload); err != nil {
		return nil, err
	}

	return polo.Polorize(&types.LogicPayload{
		Callsite: payload.Callsite,
		Calldata: payload.Calldata.Bytes(),
		Logic:    types.FromHex(payload.LogicID),
	})
}
