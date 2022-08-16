package jug

import (
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/guna"
	"math/big"
)

type state interface {
	Revert(snap *guna.StateObject) error
	GetDirtyObject(addr ktypes.Address) (*guna.StateObject, error)
	CreateDirtyObject(addr ktypes.Address, accType ktypes.AccType) *guna.StateObject
	CreateStateObject(addr ktypes.Address, accType ktypes.AccType) *guna.StateObject
	IsGenesis(addr ktypes.Address) (bool, error)
}
type Executor struct {
	Ixs          ktypes.Interactions
	objects      map[ktypes.Address]*guna.StateObject
	snaps        map[ktypes.Address]*guna.StateObject
	contextDelta ktypes.ContextDelta
	totalGas     uint64
	gasLimit     uint64
	receipts     map[ktypes.Hash]*ktypes.Receipt
	stateManager state
	commitHashes map[ktypes.Address]ktypes.Hash
}

func NewExecutor(ix ktypes.Interactions, gasLimit uint64, contextDelta ktypes.ContextDelta, state state) *Executor {
	e := &Executor{
		Ixs:          ix,
		contextDelta: contextDelta,
		gasLimit:     gasLimit,
		stateManager: state,
		objects:      make(map[ktypes.Address]*guna.StateObject),
		snaps:        make(map[ktypes.Address]*guna.StateObject),
		commitHashes: make(map[ktypes.Address]ktypes.Hash),
		receipts:     make(map[ktypes.Hash]*ktypes.Receipt),
	}

	return e
}
func (e *Executor) getReceipt(ixHash ktypes.Hash) *ktypes.Receipt {
	if _, ok := e.receipts[ixHash]; !ok {
		e.receipts[ixHash] = &ktypes.Receipt{
			IxHash:        ixHash,
			StateHashes:   make(map[ktypes.Address]ktypes.Hash),
			ContextHashes: make(map[ktypes.Address]ktypes.Hash),
		}
	}

	return e.receipts[ixHash]
}
func (e *Executor) Receipts() ktypes.Receipts {
	return e.receipts
}
func (e *Executor) Execute() error {
	for _, ix := range e.Ixs {
		if err := e.fetchStateObjects(ix); err != nil {
			return err
		}

		switch ix.IxType() {
		case ktypes.ValueTransfer:
			receipt := e.getReceipt(ix.Hash)

			for assetID, transferValue := range ix.Data.Input.TransferValue {
				gasConsumed, err := ValueTransfer(
					e.objects[ix.FromAddress()],
					e.objects[ix.ToAddress()],
					assetID,
					new(big.Int).SetUint64(transferValue),
				)
				if err != nil {
					return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
				}

				e.totalGas += gasConsumed
				receipt.GasUsed += gasConsumed
			}

		case ktypes.AssetCreation:
			receipt := e.getReceipt(ix.Hash)

			gasConsumed, id, err := CreateAsset(e.objects[ix.FromAddress()], ix.GetAssetCreationPayload())
			if err != nil {
				return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
			}

			e.totalGas += gasConsumed
			receipt.GasUsed += gasConsumed
			receiptData := ktypes.AssetCreationReceipt{AssetID: id}

			if err = receipt.SetExtraData(receiptData); err != nil {
				return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
			}

		default:
			return errors.Wrap(ktypes.ErrExecutionFailed, ktypes.ErrInvalidInteractionType.Error())
		}

		if err := e.updateSargaState(ix); err != nil {
			return err
		}

		if err := e.UpdateContext(ix, e.contextDelta); err != nil {
			return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
		}

		if err := e.CommitObjects(ix); err != nil {
			return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
		}
	}

	return nil
}

func (e *Executor) updateSargaState(ix *ktypes.Interaction) error {
	isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
	if err != nil {
		return err
	}

	if isGenesis {
		genesisObject := e.getObject(guna.GenesisAddress)
		genesisObject.AddAccountGenesisInfo(ix.ToAddress(), ix.Hash)
	}

	return nil
}
func (e *Executor) fetchStateObjects(ix *ktypes.Interaction) error {
	if senderAddr := ix.FromAddress(); senderAddr != ktypes.NilAddress && e.objects[senderAddr] == nil {
		senderObject, err := e.stateManager.GetDirtyObject(senderAddr)
		if err != nil {
			return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
		}

		e.objects[senderAddr] = senderObject
		e.snaps[senderAddr] = senderObject.Copy()
	}

	if receiverAddr := ix.ToAddress(); receiverAddr != ktypes.NilAddress {
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
			receiverObject = e.stateManager.CreateDirtyObject(receiverAddr, ktypes.AccTypeFromIxType(ix.IxType()))
		} else {
			if receiverObject, err = e.stateManager.GetDirtyObject(receiverAddr); err != nil {
				return errors.Wrap(ktypes.ErrExecutionFailed, err.Error())
			}
		}

		e.objects[receiverAddr] = receiverObject
		e.snaps[receiverAddr] = receiverObject.Copy()
	}

	return nil
}
func ValueTransfer(sender, receiver *guna.StateObject, assetID ktypes.AssetID, value *big.Int) (uint64, error) {
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
func CreateAsset(creator *guna.StateObject, assetDetails *ktypes.AssetDataInput) (uint64, string, error) {
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
func (e *Executor) getObject(addr ktypes.Address) *guna.StateObject {
	return e.objects[addr]
}
func (e *Executor) UpdateContext(ix *ktypes.Interaction, contextInfo ktypes.ContextDelta) error {
	receipt := e.getReceipt(ix.Hash)

	for addr, delta := range contextInfo {
		switch addr {
		case ix.FromAddress():
			hash, err := e.getObject(ix.FromAddress()).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			}

			receipt.ContextHashes[ix.FromAddress()] = hash
		case ix.ToAddress():
			var (
				hash ktypes.Hash
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
					return errors.Wrap(ktypes.ErrContextCreation, err.Error())
				}
			} else {
				// Update the context if it is existing account
				hash, err = e.getObject(ix.ToAddress()).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
				if err != nil {
					return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
				}
			}

			receipt.ContextHashes[ix.ToAddress()] = hash
		case guna.GenesisAddress:
			hash, err := e.getObject(guna.GenesisAddress).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			}

			receipt.ContextHashes[guna.GenesisAddress] = hash
		}
	}

	return nil
}

func (e *Executor) CommitObjects(ix *ktypes.Interaction) error {
	receipt := e.getReceipt(ix.Hash)

	if ix.FromAddress() != ktypes.NilAddress {
		senderObject := e.getObject(ix.FromAddress())

		senderHash, err := senderObject.Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[ix.FromAddress()] = senderHash
	}

	if ix.ToAddress() != ktypes.NilAddress {
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
		if ix.FromAddress() != ktypes.NilAddress {
			if err := e.stateManager.Revert(e.snaps[ix.FromAddress()]); err != nil {
				return err //This should not happen
			}
		}

		if ix.ToAddress() != ktypes.NilAddress {
			if err := e.stateManager.Revert(e.snaps[ix.ToAddress()]); err != nil {
				return err //This should not happen
			}
		}

		isGenesis, err := e.stateManager.IsGenesis(ix.ToAddress())
		if err != nil {
			return err
		}

		if isGenesis {
			if err := e.stateManager.Revert(e.snaps[guna.GenesisAddress]); err != nil {
				return err //This should not happen
			}
		}
	}

	return nil
}
