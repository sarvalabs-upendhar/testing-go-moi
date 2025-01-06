package api

import (
	"encoding/hex"
	"errors"

	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-polo"
)

const MaxAllowedOps = 3

var (
	ErrEmptyIxOps           = errors.New("ix operations cannot be empty")
	ErrTooManyIxOps         = errors.New("max 3 ops allowed")
	ErrAssetCreationLimit   = errors.New("maximum one asset creation allowed")
	ErrLogicDeploymentLimit = errors.New("maximum one logic deployment allowed")
)

// SendInteractions is a method of PublicIXAPI that stores the interaction
func (p *PublicCoreAPI) SendInteractions(sendIx *rpcargs.SendIX) (common.Hash, error) {
	signs, err := hex.DecodeString(sendIx.Signatures)
	if err != nil {
		return common.NilHash, err
	}

	ixData, err := validateArgumentsWithSign(sendIx)
	if err != nil {
		return common.NilHash, err
	}

	signatures := make(common.Signatures, 0)
	if err := signatures.FromBytes(signs); err != nil {
		return common.NilHash, err
	}

	ixn, err := common.NewInteraction(*ixData, signatures)
	if err != nil {
		return common.NilHash, err
	}

	// TODO Add validation to check for max ixn group size
	// add the interactions to ix pool
	ixs := common.NewInteractionsWithLeaderCheck(true, ixn)

	errs := p.ixpool.AddLocalInteractions(ixs)
	if len(errs) > 0 {
		return common.NilHash, errs[0]
	}

	return ixn.Hash(), nil
}

// validateArgumentsWithSign checks whether the IXArgs are valid or not
func validateArgumentsWithSign(args *rpcargs.SendIX) (*common.IxData, error) {
	bz, err := hex.DecodeString(args.IXArgs)
	if err != nil {
		return nil, err
	}

	ixData := new(common.IxData)

	err = polo.Depolorize(ixData, bz)
	if err != nil {
		return nil, err
	}

	if err = validateIxData(ixData, true); err != nil {
		return nil, err
	}

	return ixData, nil
}

func validateIxData(ixData *common.IxData, requiresFuel bool) error {
	if ixData.Sender.Address.IsNil() {
		return common.ErrInvalidAddress
	}

	// Reject genesis account interaction
	if ixData.Sender.Address == common.SargaAddress {
		return common.ErrGenesisAccount
	}

	if requiresFuel {
		if err := validateFuel(ixData); err != nil {
			return err
		}
	}

	if err := validateIxOps(ixData.IxOps); err != nil {
		return err
	}

	return nil
}

func validateFuel(ixData *common.IxData) error {
	if ixData.FuelPrice == nil {
		return common.ErrFuelPriceNotFound
	}

	if ixData.FuelLimit == 0 {
		return common.ErrFuelLimitNotFound
	}

	return nil
}

func validateIxOps(ixOps []common.IxOpRaw) error {
	if len(ixOps) == 0 {
		return ErrEmptyIxOps
	}

	if len(ixOps) > MaxAllowedOps {
		return ErrTooManyIxOps
	}

	assetCreationCount, logicDeployCount := 0, 0

	for _, op := range ixOps {
		if op.Type == common.IxAssetCreate {
			assetCreationCount++

			continue
		}

		if op.Type == common.IxLogicDeploy {
			logicDeployCount++
		}
	}

	if assetCreationCount > 1 {
		return ErrAssetCreationLimit
	}

	if logicDeployCount > 1 {
		return ErrLogicDeploymentLimit
	}

	return nil
}

// helper function for moi.Call and moi.FuelEstimate
func constructIxn(sm backend.StateManager, ixData *common.IxData) (ix *common.Interaction, err error) {
	for _, op := range ixData.IxOps {
		if op.Type == common.IxLogicDeploy || op.Type == common.IxAssetCreate {
			sequenceID, err := sm.GetSequenceID(ixData.Sender.Address, ixData.Sender.KeyID, common.NilHash)
			if err != nil {
				return nil, err
			}

			ixData.Sender.SequenceID = sequenceID

			break
		}
	}

	return common.NewInteraction(*ixData, nil)
}

func createIxData(ixArgs *rpcargs.IxArgs) *common.IxData {
	ixData := &common.IxData{
		Sender: ixArgs.Sender,
		Payer:  ixArgs.Payer,

		FuelPrice:  ixArgs.FuelPrice.ToInt(),
		FuelLimit:  uint64(ixArgs.FuelLimit),
		Perception: ixArgs.Perception.Bytes(),
	}

	if len(ixArgs.Funds) > 0 {
		ixData.Funds = make([]common.IxFund, len(ixArgs.Funds))
		for idx, asset := range ixArgs.Funds {
			ixData.Funds[idx] = common.IxFund{
				AssetID: asset.AssetID,
				Amount:  asset.Amount.ToInt(),
			}
		}
	}

	if len(ixArgs.IxOps) > 0 {
		ixData.IxOps = make([]common.IxOpRaw, len(ixArgs.IxOps))
		for idx, op := range ixArgs.IxOps {
			ixData.IxOps[idx] = common.IxOpRaw{
				Type:    op.Type,
				Payload: op.Payload.Bytes(),
			}
		}
	}

	if len(ixArgs.Participants) > 0 {
		ixData.Participants = make([]common.IxParticipant, len(ixArgs.Participants))
		for idx, participant := range ixArgs.Participants {
			ixData.Participants[idx] = common.IxParticipant{
				Address:  participant.Address,
				LockType: participant.LockType,
			}
		}
	}

	if ixArgs.Preferences != nil {
		ixData.Preferences = &common.IxPreferences{
			Compute: ixArgs.Preferences.Compute.Bytes(),
			Consensus: &common.IxConsensusPreference{
				MTQ:        ixArgs.Preferences.Consensus.MTQ.ToInt(),
				TrustNodes: ixArgs.Preferences.Consensus.TrustNodes,
			},
		}
	}

	return ixData
}
