package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/moichain/guna"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/types"
)

const (
	txMaxSize = 128 * 1024 // 128Kb
)

var (
	ErrNonceTooLow    = errors.New("nonce too low")
	ErrOversizedData  = errors.New("over sized data")
	ErrGenesisAccount = errors.New("genesis account interactions forbidden")
)

// PublicIXAPI is a struct that represents a wrapper for the public interaction APIs.
type PublicIXAPI struct {
	// Represents the API backend
	ixpool IxPool
	sm     StateManager
	cfg    *common.IxPoolConfig
}

func NewPublicIXAPI(ixpool IxPool, sm StateManager, cfg *common.IxPoolConfig) *PublicIXAPI {
	// Create the public interaction API wrapper and return it
	return &PublicIXAPI{ixpool, sm, cfg}
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

	err = validateInteraction(ixn, p)
	if err != nil {
		return nil, err
	}

	// Call the following method to add interactions to pool
	return ixn, p.ixpool.AddInteractions(types.Interactions{ixn})[0]
}

// helper function
func constructInteraction(args *SendIXArgs, nonce uint64) (*types.Interaction, error) {
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
		payloadArgs := new(AssetCreationArgs)
		if err := json.Unmarshal(args.Payload, payloadArgs); err != nil {
			return nil, err
		}

		supplyData, err := hex.DecodeString(payloadArgs.Supply)
		if err != nil {
			return nil, err
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

		assetPayload := types.AssetPayload{
			Create: createPayload,
		}

		payloadData, err := polo.Polorize(assetPayload)
		if err != nil {
			return nil, err
		}

		data.Input.Payload = payloadData

	default:
		return nil, errors.New("invalid interaction type")
	}

	return types.NewInteraction(data, nil), nil
}

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

func validateInteraction(ix *types.Interaction, p *PublicIXAPI) error {
	// Check the interaction size to overcome DOS Attacks
	ixSize, err := ix.Size()
	if err != nil {
		return err
	}

	if ixSize > txMaxSize {
		return ErrOversizedData
	}

	// Reject underpriced transactions
	if ix.IsUnderpriced(p.cfg.PriceLimit) {
		return types.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := p.sm.GetLatestNonce(ix.Sender()); n > ix.Nonce() {
		return ErrNonceTooLow
	}

	return nil
}
