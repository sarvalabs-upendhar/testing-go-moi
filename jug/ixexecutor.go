package jug

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/guna"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// IxExecutor represents an executor instance that handles the
// execution of a group of Interactions within a single cluster.
type IxExecutor struct {
	Interactions types.Interactions
	contextDelta types.ContextDelta

	state state
	exec  *ExecutionManager
	tank  *ctypes.FuelTank

	objects   map[types.Address]*guna.StateObject
	snapshots map[types.Address]*guna.StateObject

	receipts     map[types.Hash]*types.Receipt
	commitHashes map[types.Address]types.Hash
}

// Execute executes all the given Interactions with their context delta.
// Returns an error if the execution of any interaction fails.
func (executor *IxExecutor) Execute(ixs types.Interactions, delta types.ContextDelta) error {
	// Update the interaction and the context delta into the executor
	executor.Interactions = ixs
	executor.contextDelta = delta

	for _, ix := range executor.Interactions {
		// Retrieve the state objects for interaction participants
		if err := executor.fetchStateObjects(ix); err != nil {
			return errors.Wrap(err, "execution failed")
		}

		// Retrieve the receipt for the interaction
		receipt := executor.getReceipt(ix.Hash())

		switch ix.Type() {
		// Value Transfer Interaction
		case types.IxValueTransfer:
			// For each asset, apply the value transfer routine for the given transfer amount.
			for assetID, transferValue := range ix.TransferValues() {
				// Perform value transfer and record fuel consumed
				fuelConsumed, err := executor.ValueTransfer(
					executor.getStateObject(ix.Sender()),
					executor.getStateObject(ix.Receiver()),
					assetID, transferValue,
				)
				if err != nil {
					return errors.Wrap(err, "execution failed (IxValueTransfer)")
				}

				// Update fuel consumption
				if !executor.tank.Exhaust(fuelConsumed) {
					return errors.Wrap(types.ErrInsufficientFuel, "execution failed (IxValueTransfer)")
				}

				receipt.FuelUsed += fuelConsumed
			}

		// Asset Create Interaction
		case types.IxAssetCreate:
			payload, err := ix.GetAssetPayload()
			if err != nil {
				return err
			}

			if payload.Create == nil {
				return errors.New("execution failed (IxAssetCreate): asset create payload is empty")
			}

			// Perform asset creation and record fuel consumed
			fuelConsumed, assetID, err := executor.CreateAsset(
				executor.getStateObject(ix.Sender()),
				*payload.Create,
			)
			if err != nil {
				return errors.Wrap(err, "execution failed (IxAssetCreate)")
			}

			// Update fuel consumption
			if !executor.tank.Exhaust(fuelConsumed) {
				return errors.Wrap(types.ErrInsufficientFuel, "execution failed (IxAssetCreate)")
			}

			receipt.FuelUsed += fuelConsumed

			// Create data for asset creation receipt and set it into the receipt
			receiptData := types.AssetCreationReceipt{AssetID: assetID}
			if err = receipt.SetExtraData(receiptData); err != nil {
				return errors.Wrap(err, "execution failed (IxAssetCreate)")
			}

		// Deploy Logic Interaction
		case types.IxLogicDeploy:
			payload, err := ix.GetLogicPayload()
			if err != nil {
				return err
			}

			if payload.Deploy == nil {
				return errors.New("execution failed (IxLogicDeploy): logic deploy payload is empty")
			}

			// Perform logic deploy and record fuel consumed
			fuelConsumed, logicID, err := executor.LogicDeploy(
				executor.getStateObject(ix.Receiver()),
				payload,
			)
			if err != nil {
				return errors.Wrap(err, "execution failed (IxLogicDeploy)")
			}

			// Update fuel consumption
			if !executor.tank.Exhaust(fuelConsumed) {
				return errors.Wrap(types.ErrInsufficientFuel, "execution failed (IxLogicDeploy)")
			}

			receipt.FuelUsed += fuelConsumed

			// Create data for logic deploy receipt and set it into the receipt
			receiptData := types.LogicDeployReceipt{LogicID: logicID.Hex()}
			if err = receipt.SetExtraData(receiptData); err != nil {
				return errors.Wrap(err, "execution failed (IxLogicDeploy)")
			}

		// Execute Logic Interaction
		case types.IxLogicExecute:
			payload, err := ix.GetLogicPayload()
			if err != nil {
				return err
			}

			// Perform logic deploy and record fuel consumed
			fuelConsumed, returnData, err := executor.LogicExecute(
				executor.getStateObject(ix.Receiver()),
				payload,
			)
			if err != nil {
				return errors.Wrap(err, "execution failed (IxLogicExecute)")
			}

			// Update fuel consumption
			if !executor.tank.Exhaust(fuelConsumed) {
				return errors.Wrap(types.ErrInsufficientFuel, "execution failed (IxLogicExecute)")
			}

			receipt.FuelUsed += fuelConsumed

			// Create data for logic execute receipt and set it into the receipt
			receiptData := types.LogicExecuteReceipt{ReturnData: returnData}
			if err = receipt.SetExtraData(receiptData); err != nil {
				return errors.Wrap(err, "execution failed (IxLogicExecute)")
			}

		default:
			return errors.Wrap(types.ErrInvalidInteractionType, "execution failed")
		}

		// Update Sarga state if the interaction receiver if it is an unregistered (new) account
		if err := executor.updateSargaState(ix); err != nil {
			return err
		}

		// Update the context for all interaction participants and update
		// the interaction receipt with their updated context hashes
		if err := executor.UpdateContext(ix, executor.contextDelta); err != nil {
			return errors.Wrap(err, "execution failed")
		}

		// Commit the state objects of all interaction participants
		if err := executor.CommitStateObjects(ix); err != nil {
			return errors.Wrap(err, "execution failed")
		}
	}

	return nil
}

// UpdateContext updates the context of the participant accounts updates the context hashes for
// the interaction receipt with the updated context hashes for all the interaction participants.
// If the interaction receiver is a new account, the context and hash for the sarga account is also updated.
func (executor *IxExecutor) UpdateContext(ix *types.Interaction, contextDelta types.ContextDelta) error {
	// Retrieve the receipt for the interaction
	receipt := executor.getReceipt(ix.Hash())

	for addr, delta := range contextDelta {
		// For the address of each delta group in the context delta, determine action
		// based on whether it is the sender, receiver or the sarga address
		switch addr {
		// Receiver Address
		case ix.Receiver():
			var hash types.Hash

			// Check if the account is registered
			accountRegistered, err := executor.state.IsAccountRegistered(addr)
			if err != nil {
				return err
			}

			// Create a context for the account state object if it is a new account
			if !accountRegistered {
				if hash, err = executor.getStateObject(addr).CreateContext(
					delta.BehaviouralNodes,
					delta.RandomNodes,
				); err != nil {
					return errors.Wrap(types.ErrContextCreation, err.Error())
				}

				// Update the execution receipt for the interaction with the new context hash for the receiver
				receipt.ContextHashes[addr] = hash

				continue
			}

			// If the account already exists, fall
			// through and update an existing context
			fallthrough

		// Sender Address / Sarga Address
		case ix.Sender(), guna.SargaAddress:
			// Retrieve the state object for address and update its context
			hash, err := executor.getStateObject(addr).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(types.ErrUpdatingContext, err.Error())
			}

			// Update the execution receipt for the interaction with the new context hash for the sender
			receipt.ContextHashes[addr] = hash
		}
	}

	return nil
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the StateObject of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects(ix *types.Interaction) error {
	// Retrieve the receipt for the interaction
	receipt := executor.getReceipt(ix.Hash())

	// Commit the sender state object (if it exists)
	if !ix.Sender().IsNil() {
		senderHash, err := executor.getStateObject(ix.Sender()).Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[ix.Sender()] = senderHash
	}

	// Commit the receiver state object (if it exists)
	if !ix.Receiver().IsNil() {
		receiverHash, err := executor.getStateObject(ix.Receiver()).Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[ix.Receiver()] = receiverHash
	}

	// Check if the receiver account is registered
	accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	// If the receiver account is new (unregistered),
	// commit the state object of the sarga account
	if !accountRegistered {
		genesisHash, err := executor.getStateObject(guna.SargaAddress).Commit()
		if err != nil {
			return err
		}

		receipt.StateHashes[guna.SargaAddress] = genesisHash
	}

	return nil
}

// Receipts returns all the execution receipts in the executor as
// types.Receipt objects in a map index by their interaction hash.
func (executor *IxExecutor) Receipts() types.Receipts {
	return executor.receipts
}

// Revert reverts all state objects modified by the IxExecutor to their original state.
func (executor *IxExecutor) Revert() error {
	for _, ix := range executor.Interactions {
		// Revert sender state object to snapshot state
		if !ix.Sender().IsNil() {
			if err := executor.state.Revert(executor.snapshots[ix.Sender()]); err != nil {
				return err // This should not happen
			}
		}

		// Revert receiver state object to snapshot state
		if !ix.Receiver().IsNil() {
			if err := executor.state.Revert(executor.snapshots[ix.Receiver()]); err != nil {
				return err // This should not happen
			}
		}

		// Check if the receiver account has been registered
		accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
		if err != nil {
			return err
		}

		// If the account is new (unregistered), revert sarga state object to its snapshot
		if !accountRegistered {
			if err := executor.state.Revert(executor.snapshots[guna.SargaAddress]); err != nil {
				return err // This should not happen
			}
		}
	}

	return nil
}

// getReceipt returns the types.Receipt object for a given interaction hash.
// If there is no receipt for the hash, a new Receipt is generated, stored and returned.
func (executor *IxExecutor) getReceipt(ixHash types.Hash) *types.Receipt {
	// Return receipt from executor's receipts if it exists
	if receipt, ok := executor.receipts[ixHash]; ok {
		// Return the existing receipt
		return receipt
	}

	// Create new receipt for interaction hash if it does not exist
	receipt := &types.Receipt{
		IxHash:        ixHash,
		StateHashes:   make(map[types.Address]types.Hash),
		ContextHashes: make(map[types.Address]types.Hash),
	}

	// Set the created receipt into the executor
	executor.receipts[ixHash] = receipt
	// Return the newly created receipt
	return receipt
}

// updateSargaState will update the Sarga StateObject with the account genesis
// information for the receiver of an Interaction if it has not been registered.
// If the receiver address is already registered, no change is performed.
func (executor *IxExecutor) updateSargaState(ix *types.Interaction) error {
	// Check if the receiver address is an already registered account
	accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	// If account is registered, sarga state does not need to be updated
	if accountRegistered {
		return nil
	}

	// Get dirty object for sarga
	sargaObject := executor.getStateObject(guna.SargaAddress)
	if sargaObject == nil {
		return errors.New("sarga object not found")
	}

	// Add the account genesis information for the new account
	return sargaObject.AddAccountGenesisInfo(ix.Receiver(), ix.Hash())
}

// getStateObject returns the guna.StateObject for the given address.
// Returns a nil, if there is no state object for the address.
func (executor *IxExecutor) getStateObject(addr types.Address) *guna.StateObject {
	return executor.objects[addr]
}

// fetchStateObjects retrieves all required state objects for the given types.Interaction.
// If the receiver is not a registered account, this function will also retrieve
// the genesis state object and create a state object for the new account.
func (executor *IxExecutor) fetchStateObjects(ix *types.Interaction) error {
	// Fetch state object for sender if valid and not already available in the executor
	if sender := ix.Sender(); !sender.IsNil() && executor.getStateObject(sender) == nil {
		// Retrieve the dirty object for the sender from the state manager
		senderObject, err := executor.state.GetDirtyObject(sender)
		if err != nil {
			return errors.Wrap(err, "state object fetch failed")
		}

		// Add sender state object and its snapshot to the executor
		executor.objects[sender] = senderObject
		executor.snapshots[sender] = senderObject.Copy()
	}

	// Fetch state object for receiver if valid
	if receiver := ix.Receiver(); !receiver.IsNil() {
		var receiverObject *guna.StateObject

		// Check if the receiver address is an already registered account
		accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
		if err != nil {
			return errors.Wrap(err, "state object fetch failed")
		}

		if !accountRegistered {
			// Retrieve the dirty object for genesis (sarga) address
			genesisObject, err := executor.state.GetDirtyObject(guna.SargaAddress)
			if err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}

			// Add genesis state object and its snapshot to the executor
			executor.objects[guna.SargaAddress] = genesisObject
			executor.snapshots[guna.SargaAddress] = genesisObject.Copy()

			// Create a new dirty state object for the account
			receiverObject = executor.state.CreateDirtyObject(receiver, types.AccTypeFromIxType(ix.Type()))
		} else {
			// Retrieve the dirty object for the receiver from the state manager
			if receiverObject, err = executor.state.GetDirtyObject(receiver); err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}
		}

		// Add receiver state object and its snapshot to the executor
		executor.objects[receiver] = receiverObject
		executor.snapshots[receiver] = receiverObject.Copy()
	}

	return nil
}
