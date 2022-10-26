package api

import (
	"errors"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
)

const (
	txMaxSize = 128 * 1024 //128Kb
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
func (p *PublicIXAPI) SendInteraction(args *SendIXArgs) (ktypes.Interactions, error) {
	err := validateArguments(args, p)
	if err != nil {
		return nil, err
	}

	nonce, err := p.ixpool.GetNonce(ktypes.HexToAddress(args.From))
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
func constructInteraction(args *SendIXArgs, nonce uint64) (ktypes.Interactions, error) {
	// Construct the interactions data
	ixns := make(ktypes.Interactions, 1)
	ixns[0] = new(ktypes.Interaction)

	ixns[0].Data = ktypes.IxData{
		Input: ktypes.InteractionInput{
			Type:     args.IxType,
			Nonce:    nonce,
			From:     ktypes.HexToAddress(args.From),
			To:       ktypes.HexToAddress(args.To),
			AnuPrice: args.AnuPrice,
		},
	}

	switch args.IxType {
	case 0:
		ixns[0].Data.Input.TransferValue = map[ktypes.AssetID]uint64{ktypes.AssetID(args.AssetID): uint64(args.Value)}

	case 1:
		ixns[0].Data.Input.Payload = ktypes.InteractionInputPayload{
			AssetData: ktypes.AssetDataInput{
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
	senderAddress, err := kutils.ValidateAddress(args.From)
	if err != nil {
		return ktypes.ErrInvalidAddress
	}

	// Reject genesis account interaction
	if isGenesisAccount, _ := p.sm.IsGenesis(ktypes.HexToAddress(senderAddress)); isGenesisAccount {
		return ErrGenesisAccount
	}

	if args.To != "" {
		receiverAddress, err := kutils.ValidateAddress(args.To)
		if err != nil {
			return ktypes.ErrInvalidAddress
		}

		// Reject genesis account interaction
		if isGenesisAccount, _ := p.sm.IsGenesis(ktypes.HexToAddress(receiverAddress)); isGenesisAccount {
			return ErrGenesisAccount
		}
	}

	return nil
}

func validateInteraction(ix *ktypes.Interaction, p *PublicIXAPI) error {
	// Check the interaction size to overcome DOS Attacks
	if uint64(ix.GetSize()) > txMaxSize {
		return ErrOversizedData
	}

	// Reject underpriced transactions
	if ix.IsUnderpriced(p.cfg.PriceLimit) {
		return ktypes.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := p.sm.GetLatestNonce(ix.FromAddress()); n > ix.Nonce() {
		return ErrNonceTooLow
	}

	return nil
}
