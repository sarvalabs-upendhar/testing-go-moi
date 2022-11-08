package api

import (
	"errors"

	"gitlab.com/sarvalabs/moichain/utils"

	"gitlab.com/sarvalabs/moichain/guna"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/types"
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
func (p *PublicIXAPI) SendInteraction(args *SendIXArgs) (types.Interactions, error) {
	err := validateArguments(args, p)
	if err != nil {
		return nil, err
	}

	nonce, err := p.ixpool.GetNonce(types.HexToAddress(args.From))
	if err != nil {
		return nil, err
	}

	ixns, err := constructInteraction(args, nonce)
	if err != nil {
		return nil, err
	}

	err = validateInteraction(ixns[0], p)
	if err != nil {
		return nil, err
	}

	// Call the following method to add interactions to pool
	return ixns, p.ixpool.AddInteractions(ixns)[0]
}

// helper function
func constructInteraction(args *SendIXArgs, nonce uint64) (types.Interactions, error) {
	// Construct the interactions data
	ixns := make(types.Interactions, 1)
	ixns[0] = new(types.Interaction)

	ixns[0].Data = types.IxData{
		Input: types.InteractionInput{
			Type:     args.IxType,
			Nonce:    nonce,
			From:     types.HexToAddress(args.From),
			To:       types.HexToAddress(args.To),
			AnuPrice: args.AnuPrice,
		},
	}

	switch args.IxType {
	case 0:
		ixns[0].Data.Input.TransferValue = map[types.AssetID]uint64{types.AssetID(args.AssetID): uint64(args.Value)}

	case 1:
		ixns[0].Data.Input.Payload = types.InteractionInputPayload{
			AssetData: types.AssetDataInput{
				Dimension:   args.AssetCreation.Dimension,
				TotalSupply: args.AssetCreation.TotalSupply,
				Symbol:      args.AssetCreation.Symbol,
				IsFungible:  args.AssetCreation.IsFungible,
				IsMintable:  args.AssetCreation.IsMintable,
				Code:        args.AssetCreation.Code,
			},
		}
	default:
		return ixns, errors.New("invalid interaction type")
	}

	return ixns, nil
}

func validateArguments(args *SendIXArgs, p *PublicIXAPI) error {
	// Reject interaction if sender address is invalid
	senderAddress, err := utils.ValidateAddress(args.From)
	if err != nil {
		return types.ErrInvalidAddress
	}

	// Reject genesis account interaction
	if types.HexToAddress(senderAddress) == guna.GenesisAddress {
		return ErrGenesisAccount
	}

	if args.To != "" {
		receiverAddress, err := utils.ValidateAddress(args.To)
		if err != nil {
			return types.ErrInvalidAddress
		}

		// Reject genesis account interaction
		if types.HexToAddress(receiverAddress) == guna.GenesisAddress {
			return ErrGenesisAccount
		}
	}

	return nil
}

func validateInteraction(ix *types.Interaction, p *PublicIXAPI) error {
	// Check the interaction size to overcome DOS Attacks
	if uint64(ix.GetSize()) > txMaxSize {
		return ErrOversizedData
	}

	// Reject underpriced transactions
	if ix.IsUnderpriced(p.cfg.PriceLimit) {
		return types.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := p.sm.GetLatestNonce(ix.FromAddress()); n > ix.Nonce() {
		return ErrNonceTooLow
	}

	return nil
}
