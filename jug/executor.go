package jug

import (
	"math/big"

	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/types"
)

type state interface {
	Revert(snap *guna.StateObject) error
	GetDirtyObject(addr types.Address) (*guna.StateObject, error)
	CreateDirtyObject(addr types.Address, accType types.AccType) *guna.StateObject
	IsGenesis(addr types.Address) (bool, error)
}
type Executor struct {
	Ixs          types.Interactions
	objects      map[types.Address]*guna.StateObject
	snaps        map[types.Address]*guna.StateObject
	contextDelta types.ContextDelta
	totalGas     uint64
	gasLimit     uint64
	receipts     map[types.Hash]*types.Receipt
	stateManager state
	commitHashes map[types.Address]types.Hash
}

func NewExecutor(ix types.Interactions, gasLimit uint64, contextDelta types.ContextDelta, state state) *Executor {
	e := &Executor{
		Ixs:          ix,
		contextDelta: contextDelta,
		gasLimit:     gasLimit,
		stateManager: state,
		objects:      make(map[types.Address]*guna.StateObject),
		snaps:        make(map[types.Address]*guna.StateObject),
		commitHashes: make(map[types.Address]types.Hash),
		receipts:     make(map[types.Hash]*types.Receipt),
	}

	return e
}

func (e *Executor) getReceipt(ixHash types.Hash) *types.Receipt {
	if _, ok := e.receipts[ixHash]; !ok {
		e.receipts[ixHash] = &types.Receipt{
			IxHash:        ixHash,
			StateHashes:   make(map[types.Address]types.Hash),
			ContextHashes: make(map[types.Address]types.Hash),
		}
	}

	return e.receipts[ixHash]
}

func (e *Executor) Receipts() types.Receipts {
	return e.receipts
}

func (e *Executor) Execute() error {
	for _, ix := range e.Ixs {
		if err := e.fetchStateObjects(ix); err != nil {
			return err
		}

		switch ix.IxType() {
		case types.ValueTransfer:
			receipt := e.getReceipt(ix.Hash)

			for assetID, transferValue := range ix.Data.Input.TransferValue {
				gasConsumed, err := ValueTransfer(
					e.objects[ix.FromAddress()],
					e.objects[ix.ToAddress()],
					assetID,
					new(big.Int).SetUint64(transferValue),
				)
				if err != nil {
					return errors.Wrap(types.ErrExecutionFailed, err.Error())
				}

				e.totalGas += gasConsumed
				receipt.GasUsed += gasConsumed
			}

		case types.AssetCreation:
			receipt := e.getReceipt(ix.Hash)

			gasConsumed, id, err := CreateAsset(e.objects[ix.FromAddress()], ix.GetAssetCreationPayload())
			if err != nil {
				return errors.Wrap(types.ErrExecutionFailed, err.Error())
			}

			e.totalGas += gasConsumed
			receipt.GasUsed += gasConsumed
			receiptData := types.AssetCreationReceipt{AssetID: id}

			if err = receipt.SetExtraData(receiptData); err != nil {
				return errors.Wrap(types.ErrExecutionFailed, err.Error())
			}

		default:
			return errors.Wrap(types.ErrExecutionFailed, types.ErrInvalidInteractionType.Error())
		}

		if err := e.updateSargaState(ix); err != nil {
			return err
		}

		if err := e.UpdateContext(ix, e.contextDelta); err != nil {
			return errors.Wrap(types.ErrExecutionFailed, err.Error())
		}

		if err := e.CommitObjects(ix); err != nil {
			return errors.Wrap(types.ErrExecutionFailed, err.Error())
		}
	}

	return nil
}

func (e *Executor) updateSargaState(ix *types.Interaction) error {
	isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
	if err != nil {
		return err
	}

	if !isGenesis {
		return nil
	}

	genesisObject := e.getObject(guna.GenesisAddress)

	return genesisObject.AddAccountGenesisInfo(ix.ToAddress(), ix.Hash)
}

func (e *Executor) fetchStateObjects(ix *types.Interaction) error {
	if senderAddr := ix.FromAddress(); senderAddr != types.NilAddress && e.objects[senderAddr] == nil {
		senderObject, err := e.stateManager.GetDirtyObject(senderAddr)
		if err != nil {
			return errors.Wrap(types.ErrExecutionFailed, err.Error())
		}

		e.objects[senderAddr] = senderObject
		e.snaps[senderAddr] = senderObject.Copy()
	}

	if receiverAddr := ix.ToAddress(); receiverAddr != types.NilAddress {
		var (
			receiverObject *guna.StateObject
			err            error
		)

		isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
		if err != nil {
			return err
		}

		if isGenesis {
			// Get Genesis Object
			genesisObject, err := e.stateManager.GetDirtyObject(guna.GenesisAddress)
			if err != nil {
				return err
			}

			e.objects[guna.GenesisAddress] = genesisObject
			e.snaps[guna.GenesisAddress] = genesisObject.Copy()
			// Create a dirty state object for new account
			receiverObject = e.stateManager.CreateDirtyObject(receiverAddr, types.AccTypeFromIxType(ix.IxType()))
		} else {
			if receiverObject, err = e.stateManager.GetDirtyObject(receiverAddr); err != nil {
				return errors.Wrap(types.ErrExecutionFailed, err.Error())
			}
		}

		e.objects[receiverAddr] = receiverObject
		e.snaps[receiverAddr] = receiverObject.Copy()
	}

	return nil
}

func ValueTransfer(sender, receiver *guna.StateObject, assetID types.AssetID, value *big.Int) (uint64, error) {
	bal, err := sender.BalanceOf(assetID)
	if err != nil {
		return 0, err
	}

	if value.Sign() <= 0 {
		return 0, errors.New("invalid transfer amount")
	}

	if bal.Cmp(value) == -1 {
		return 0, errors.New("low balance")
	}

	sender.SubBalance(assetID, value)
	receiver.AddBalance(assetID, value)

	return 1, nil
}

func CreateAsset(creator *guna.StateObject, assetDetails *types.AssetDataInput) (uint64, string, error) {
	assetID, err := creator.CreateAsset(
		uint8(assetDetails.Dimension),
		assetDetails.IsFungible,
		assetDetails.IsMintable,
		assetDetails.Symbol,
		int64(assetDetails.TotalSupply),
		assetDetails.Code,
	)
	if err != nil {
		return 0, "", err
	}

	return 1, string(assetID), nil
}

func (e *Executor) getObject(addr types.Address) *guna.StateObject {
	return e.objects[addr]
}

func (e *Executor) UpdateContext(ix *types.Interaction, contextInfo types.ContextDelta) error {
	receipt := e.getReceipt(ix.Hash)

	for addr, delta := range contextInfo {
		switch addr {
		case ix.FromAddress():
			hash, err := e.getObject(ix.FromAddress()).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(types.ErrUpdatingContext, err.Error())
			}

			receipt.ContextHashes[ix.FromAddress()] = hash
		case ix.ToAddress():
			var (
				hash types.Hash
				err  error
			)
			// Create context if it is new account
			isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
			if err != nil {
				return err
			}

			if isGenesis {
				if hash, err = e.getObject(ix.ToAddress()).CreateContext(
					delta.BehaviouralNodes,
					delta.RandomNodes,
				); err != nil {
					return errors.Wrap(types.ErrContextCreation, err.Error())
				}
			} else {
				// Update the context if it is existing account
				hash, err = e.getObject(ix.ToAddress()).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
				if err != nil {
					return errors.Wrap(types.ErrUpdatingContext, err.Error())
				}
			}

			receipt.ContextHashes[ix.ToAddress()] = hash
		case guna.GenesisAddress:
			hash, err := e.getObject(guna.GenesisAddress).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(types.ErrUpdatingContext, err.Error())
			}

			receipt.ContextHashes[guna.GenesisAddress] = hash
		}
	}

	return nil
}

func (e *Executor) CommitObjects(ix *types.Interaction) error {
	receipt := e.getReceipt(ix.Hash)

	if ix.FromAddress() != types.NilAddress {
		senderObject := e.getObject(ix.FromAddress())

		senderHash, err := senderObject.Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[ix.FromAddress()] = senderHash
	}

	if ix.ToAddress() != types.NilAddress {
		receiverObject := e.getObject(ix.ToAddress())

		receiverHash, err := receiverObject.Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[ix.ToAddress()] = receiverHash
	}

	isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
	if err != nil {
		return err
	}

	if isGenesis {
		genesisObject := e.getObject(guna.GenesisAddress)

		hash, err := genesisObject.Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[guna.GenesisAddress] = hash
	}

	return nil
}

func (e *Executor) Revert() error {
	for _, ix := range e.Ixs {
		if ix.FromAddress() != types.NilAddress {
			if err := e.stateManager.Revert(e.snaps[ix.FromAddress()]); err != nil {
				return err // This should not happen
			}
		}

		if ix.ToAddress() != types.NilAddress {
			if err := e.stateManager.Revert(e.snaps[ix.ToAddress()]); err != nil {
				return err // This should not happen
			}
		}

		isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
		if err != nil {
			return err
		}

		if isGenesis {
			if err := e.stateManager.Revert(e.snaps[guna.GenesisAddress]); err != nil {
				return err // This should not happen
			}
		}
	}

	return nil
}
