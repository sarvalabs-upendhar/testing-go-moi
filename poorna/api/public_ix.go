package api

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/moichain/guna"

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
func (p *PublicIXAPI) SendInteraction(args *SendIXArgs) (*types.Interaction, error) {
	err := validateArguments(args, p)
	if err != nil {
		return nil, err
	}

	nonce, err := p.ixpool.GetNonce(types.HexToAddress(args.Sender))
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
func constructInteraction(args *SendIXArgs, nonce uint64) (ix *types.Interaction, err error) {
	data := types.IxData{
		Input: types.IxInput{
			Type:           args.Type,
			Nonce:          nonce,
			Sender:         types.HexToAddress(args.Sender),
			Receiver:       types.HexToAddress(args.Receiver),
			TransferValues: make(map[types.AssetID]*big.Int, len(args.TransferValues)),
			FuelPrice:      new(big.Int).SetBytes(types.FromHex(args.FuelPrice)),
			FuelLimit:      new(big.Int).SetBytes(types.FromHex(args.FuelLimit)),
		},
	}

	switch args.Type {
	case types.IxValueTransfer:
		// Decode the transfer values
		for asset, value := range args.TransferValues {
			valueData, err := hex.DecodeString(value)
			if err != nil {
				return nil, err
			}

			data.Input.TransferValues[asset] = new(big.Int).SetBytes(valueData)
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

	case types.IxLogicExecute:
		data.Input.Payload, err = GetRawIXPayloadForLogicExecute(args.Payload)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return types.NewInteraction(data, nil), nil
}

// ValidateArguments checks whether the SendIXArgs are valid or not
func validateArguments(args *SendIXArgs, p *PublicIXAPI) error {
	// Reject interaction if sender address is invalid
	senderAddress, err := utils.ValidateAddress(args.Sender)
	if err != nil {
		return types.ErrInvalidAddress
	}

	// Reject genesis account interaction
	if senderAddress == guna.SargaAddress {
		return ErrGenesisAccount
	}

	if args.Receiver != "" {
		receiverAddress, err := utils.ValidateAddress(args.Receiver)
		if err != nil {
			return types.ErrInvalidAddress
		}

		// Reject genesis account interaction
		if receiverAddress == guna.SargaAddress {
			return ErrGenesisAccount
		}
	}

	// TODO: Add more checks to validate inputs

	return nil
}

// GetRawIXPayloadForAssetCreation returns the raw IXPayload for asset creation
func GetRawIXPayloadForAssetCreation(jsonPayload []byte) ([]byte, error) {
	payloadArgs := new(AssetCreationArgs)
	if err := json.Unmarshal(jsonPayload, payloadArgs); err != nil {
		return nil, err
	}

	supplyData, err := hex.DecodeString(payloadArgs.Supply)
	if err != nil {
		return nil, errors.New("failed to decode supply")
	}

	createPayload := &types.AssetCreatePayload{
		Type:   payloadArgs.Type,
		Symbol: payloadArgs.Symbol,
		Supply: new(big.Int).SetBytes(supplyData),

		Dimension: payloadArgs.Dimension,
		Decimals:  payloadArgs.Decimals,

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
	payload := new(LogicDeployArgs)
	if err := json.Unmarshal(jsonPayload, payload); err != nil {
		return nil, err
	}

	if len(payload.Manifest) == 0 {
		return nil, types.ErrEmptyManifest
	}

	// FIXME: It is not appropriate to generate logicID here
	logicID, err := types.NewLogicIDv0(
		payload.Type,
		payload.IsStateFul,
		payload.IsInteractive,
		0,
		utils.NewAccountAddress(nonce, sender),
	)
	if err != nil {
		return nil, err
	}

	deployPayload := &types.LogicPayload{
		Logic:    logicID,
		Calldata: types.FromHex(payload.CallData),
		Deploy: &types.LogicDeployPayload{
			Type:          payload.Type,
			IsStateful:    payload.IsStateFul,
			IsInteractive: payload.IsInteractive,
			Manifest:      types.FromHex(payload.Manifest),
		},
	}

	return polo.Polorize(deployPayload)
}

// GetRawIXPayloadForLogicExecute returns the raw IXPayload for logic execution
func GetRawIXPayloadForLogicExecute(jsonPayload []byte) ([]byte, error) {
	payload := new(LogicExecuteArgs)
	if err := json.Unmarshal(jsonPayload, payload); err != nil {
		return nil, err
	}

	logicExecPayload := &types.LogicPayload{
		Logic:    types.FromHex(payload.LogicID),
		Callsite: payload.CallSite,
		Calldata: types.FromHex(payload.CallData),
	}

	return polo.Polorize(logicExecPayload)
}
