package common

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-polo"
)

const (
	TransferEndpoint = "Transfer"
	MintEndpoint     = "Mint"
	LockupEndpoint   = "Lockup"
	BurnEndpoint     = "Burn"
	ApproveEndpoint  = "Approve"
	ReleaseEndpoint  = "Release"
	RevokeEndpoint   = "Revoke"
)

type TransferParams struct {
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
	// TokenID     TokenID                `polo:"token_id"`
	Amount *big.Int `polo:"amount"`
}

type BurnParams struct {
	Amount *big.Int `polo:"amount"`
}

type MintParams struct {
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
	Amount      *big.Int               `polo:"amount"`
}

type ApproveParams struct {
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
	Amount      *big.Int               `polo:"amount"`
	ExpiresAt   uint64                 `polo:"expires_at"`
}

type LockupParams struct {
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
	Amount      *big.Int               `polo:"amount"`
}

type ReleaseParams struct {
	Benefactor  identifiers.Identifier `polo:"benefactor"`
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
	Amount      *big.Int               `polo:"amount"`
}

type RevokeParams struct {
	Beneficiary identifiers.Identifier `polo:"beneficiary"`
}

func NewTransferParams(to identifiers.Identifier, amount *big.Int) *TransferParams {
	return &TransferParams{
		Beneficiary: to,
		Amount:      amount,
	}
}

func NewIxForTransfer(
	benefactor identifiers.Identifier,
	payouts ...PayoutDetails,
) (*Interaction, error) {
	ops := make([]IxOpRaw, 0, len(payouts))
	addedPs := make(map[identifiers.Identifier]struct{})

	ps := make([]IxParticipant, 0, len(payouts))

	ps = append(ps, IxParticipant{ID: benefactor})

	addedPs[benefactor] = struct{}{}

	for _, payout := range payouts {
		payload := &TransferParams{
			Beneficiary: payout.Beneficiary,
			Amount:      payout.Amount,
		}

		callData, err := polo.PolorizeDocument(payload, polo.DocStructs())
		if err != nil {
			return nil, err
		}

		ops = append(ops, IxOpRaw{
			Type: IxAssetAction,
			Payload: func() []byte {
				ap := &AssetActionPayload{
					AssetID:  payout.AssetID,
					Callsite: TransferEndpoint,
					Calldata: callData.Bytes(),
				}
				encoded, _ := ap.Bytes()

				return encoded
			}(),
		})

		if _, exists := addedPs[payout.Beneficiary]; !exists {
			ps = append(ps, IxParticipant{ID: payout.Beneficiary})
		}

		if _, exists := addedPs[payout.AssetID.AsIdentifier()]; !exists {
			ps = append(ps, IxParticipant{ID: payout.AssetID.AsIdentifier(), LockType: NoLock})
		}
	}

	return NewInteraction(IxData{
		Sender:       Sender{ID: benefactor},
		Participants: ps,
		IxOps:        ops,
		FuelPrice:    big.NewInt(0),
	}, nil)
}

func NewIxForMint(
	benefactor identifiers.Identifier,
	payouts ...PayoutDetails,
) (*Interaction, error) {
	ops := make([]IxOpRaw, 0, len(payouts))
	ps := make([]IxParticipant, 0, len(payouts))
	addedPs := make(map[identifiers.Identifier]struct{})

	ps = append(ps, IxParticipant{ID: benefactor})

	addedPs[benefactor] = struct{}{}

	for _, payout := range payouts {
		payload := &MintParams{
			Beneficiary: payout.Beneficiary,
			Amount:      payout.Amount,
		}

		callData, err := polo.PolorizeDocument(payload, polo.DocStructs())
		if err != nil {
			return nil, err
		}

		ops = append(ops, IxOpRaw{
			Type: IxAssetAction,
			Payload: func() []byte {
				ap := &AssetActionPayload{
					AssetID:  payout.AssetID,
					Callsite: "Mint",
					Calldata: callData.Bytes(),
				}
				encoded, _ := ap.Bytes()

				return encoded
			}(),
		})

		if _, exists := addedPs[payout.Beneficiary]; !exists {
			ps = append(ps, IxParticipant{ID: payout.Beneficiary})
		}

		if _, exists := addedPs[payout.AssetID.AsIdentifier()]; !exists {
			ps = append(ps, IxParticipant{ID: payout.AssetID.AsIdentifier(), LockType: MutateLock})
		}
	}

	return NewInteraction(IxData{
		Sender:       Sender{ID: benefactor},
		Participants: ps,
		IxOps:        ops,
		FuelPrice:    big.NewInt(0),
	}, nil)
}

func NewIxForLockup(benefactor, beneficiary identifiers.Identifier, amount *big.Int) (*Interaction, error) {
	ps := make([]IxParticipant, 0)
	ps = append(ps, IxParticipant{ID: benefactor})
	ps = append(ps, IxParticipant{ID: beneficiary})
	ps = append(ps, IxParticipant{ID: KMOITokenAssetID.AsIdentifier(), LockType: NoLock})
	ops := make([]IxOpRaw, 0)

	ops = append(ops, IxOpRaw{
		Type: IxAssetAction,
		Payload: func() []byte {
			payload := &LockupParams{
				Beneficiary: beneficiary,
				Amount:      amount,
			}
			callData, _ := polo.PolorizeDocument(payload, polo.DocStructs())

			ap := &AssetActionPayload{
				AssetID:  KMOITokenAssetID,
				Callsite: LockupEndpoint,
				Calldata: callData.Bytes(),
			}

			encoded, _ := ap.Bytes()

			return encoded
		}(),
	})

	return NewInteraction(IxData{
		Sender:       Sender{ID: benefactor},
		Participants: ps,
		IxOps:        ops,
		FuelPrice:    big.NewInt(0),
	}, nil)
}

func GetAssetActionPayload(assetID identifiers.AssetID, callsite string, params any) (*AssetActionPayload, error) {
	ap := &AssetActionPayload{
		AssetID:  assetID,
		Callsite: callsite,
	}

	callData, err := polo.PolorizeDocument(params, polo.DocStructs())
	if err != nil {
		return nil, err
	}

	ap.Calldata = callData.Bytes()

	return ap, nil
}

func GetParamsFromActionPayload(payload *AssetActionPayload) (any, []identifiers.Identifier, error) {
	ps := make([]identifiers.Identifier, 0)

	switch payload.Callsite {
	case LockupEndpoint:
		lockup := &LockupParams{}

		if payload.Calldata != nil {
			if err := polo.Depolorize(lockup, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, nil, err
			}

			ps = append(ps, lockup.Beneficiary)
		}

		return lockup, ps, nil
	case TransferEndpoint:
		transfer := new(TransferParams)

		if payload.Calldata != nil {
			if err := polo.Depolorize(transfer, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, nil, err
			}

			ps = append(ps, transfer.Beneficiary)
		}

		return transfer, ps, nil
	case RevokeEndpoint:
		revoke := &RevokeParams{}
		if payload.Calldata != nil {
			if err := polo.Depolorize(revoke, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, ps, err
			}

			ps = append(ps, revoke.Beneficiary)
		}

		return revoke, ps, nil
	case MintEndpoint:
		mint := &MintParams{}

		if payload.Calldata != nil {
			if err := polo.Depolorize(mint, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, nil, err
			}

			ps = append(ps, mint.Beneficiary)
		}

		return mint, ps, nil
	case BurnEndpoint:
		burn := &BurnParams{}
		if payload.Calldata != nil {
			if err := polo.Depolorize(burn, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, nil, err
			}
		}

		return burn, ps, nil
	case ReleaseEndpoint:
		release := &ReleaseParams{}

		if payload.Calldata != nil {
			if err := polo.Depolorize(release, payload.Calldata, polo.DocStructs()); err != nil {
				return nil, nil, err
			}

			ps = append(ps, release.Beneficiary)
			ps = append(ps, release.Benefactor)
		}

		return release, ps, nil
	default:
		return nil, nil, errors.New("invalid callsite")
	}
}
