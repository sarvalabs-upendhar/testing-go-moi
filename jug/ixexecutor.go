package jug

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/types"
)

// IxExecutor represents an executor instance that handles the
// execution of a group of Interactions within a single cluster.
type IxExecutor struct {
	Interactions types.Interactions
	contextDelta types.ContextDelta

	state state
	exec  *ExecutionManager
	// tank  *engineio.FuelTank

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

		// Create a fuel tank for the interaction
		tank := executor.exec.createFuelTank(ix)

		// Determine the required balance for MOI Token.
		// Must be the sum of the fuel limit for the ix and the transfer value for the MOI Token
		requiredBalance := new(big.Int).Add(tank.Level(), ix.MOITokenValue())
		// Check that the sender has sufficient balance of MOI Tokens
		ok, err := executor.getStateObject(ix.Sender()).HasFuel(requiredBalance)
		if err != nil {
			return errors.Wrap(err, "execution failed: fuel check")
		}

		if !ok {
			return errors.Errorf("execution failed: insufficient fuel")
		}

		if ix.Sender() != types.NilAddress {
			executor.getStateObject(ix.Sender()).IncrementNonce(1)
		}

		// Retrieve the receipt for the interaction
		receipt := executor.getReceipt(ix)

		switch ixtype := ix.Type(); ixtype {
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
					return errors.Wrapf(err, "execution failed (%v)", ixtype)
				}

				// Update fuel consumption on the receipt
				receipt.IncreaseFuelUsed(fuelConsumed)
				// Exhaust fuel from tank
				if !tank.Exhaust(fuelConsumed) {
					return errors.Wrapf(types.ErrInsufficientFuel, "execution failed (%v)", ixtype)
				}
			}

		// Asset Create Interaction
		case types.IxAssetCreate:
			payload, err := ix.GetAssetPayload()
			if err != nil {
				return err
			}

			if payload.Create == nil {
				return errors.Errorf("execution failed (%v): asset create payload is empty", ixtype)
			}

			// Perform asset creation and record fuel consumed
			fuelConsumed, assetReceipt, err := executor.CreateAsset(
				executor.getStateObject(ix.Sender()),
				executor.getStateObject(ix.Receiver()),
				*payload.Create,
			)
			if err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

			// Update fuel consumption on the receipt
			receipt.IncreaseFuelUsed(fuelConsumed)
			// Exhaust fuel from tank
			if !tank.Exhaust(fuelConsumed) {
				return errors.Wrapf(types.ErrInsufficientFuel, "execution failed (%v)", ixtype)
			}

			if err = receipt.SetExtraData(assetReceipt); err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

		case types.IxAssetMint:
			payload, err := ix.GetAssetPayload()
			if err != nil {
				return err
			}

			if payload.Mint == nil {
				return errors.Errorf("execution failed (%v): asset mint payload is empty", ixtype)
			}

			// Update asset supply and record fuel consumed
			fuelConsumed, assetReceipt, err := executor.MintAsset(
				executor.getStateObject(ix.Sender()),
				executor.getStateObject(ix.Receiver()),
				*payload.Mint,
			)
			if err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

			// Update fuel consumption
			receipt.IncreaseFuelUsed(fuelConsumed)
			// Exhaust fuel from tank
			if !tank.Exhaust(fuelConsumed) {
				return errors.Wrapf(types.ErrInsufficientFuel, "execution failed (%v)", ixtype)
			}

			if err = receipt.SetExtraData(assetReceipt); err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

		case types.IxAssetBurn:
			payload, err := ix.GetAssetPayload()
			if err != nil {
				return err
			}

			if payload.Mint == nil {
				return errors.Errorf("execution failed (%v): asset burn payload is empty", ixtype)
			}

			// Update asset supply and record fuel consumed
			fuelConsumed, assetReceipt, err := executor.BurnAsset(
				executor.getStateObject(ix.Sender()),
				executor.getStateObject(ix.Receiver()),
				*payload.Mint,
			)
			if err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

			// Update fuel consumption
			receipt.IncreaseFuelUsed(fuelConsumed)
			// Exhaust fuel from tank
			if !tank.Exhaust(fuelConsumed) {
				return errors.Wrapf(types.ErrInsufficientFuel, "execution failed (%v)", ixtype)
			}

			if err = receipt.SetExtraData(assetReceipt); err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

		// Deploy Logic Interaction
		case types.IxLogicDeploy:
			payload, err := ix.GetLogicPayload()
			if err != nil {
				return err
			}

			if payload.Manifest == nil {
				return errors.Errorf("execution failed (%v): missing manifest for logic deploy", ixtype)
			}

			logicAddress := types.NewAccountAddress(ix.Nonce(), ix.Sender())

			// Perform logic deploy and record fuel consumed
			fuelConsumed, deployReceipt, err := executor.LogicDeploy(
				tank,
				executor.getStateObject(logicAddress),
				executor.getStateObject(ix.Sender()),
				payload,
			)
			if err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

			if deployReceipt.Error != nil {
				receipt.Status = types.ReceiptFailed
			}

			// Update fuel consumption on the receipt
			receipt.IncreaseFuelUsed(fuelConsumed)
			// Exhaust fuel from tank
			if !tank.Exhaust(fuelConsumed) {
				return errors.Wrapf(types.ErrInsufficientFuel, "execution failed (%v)", ixtype)
			}

			if err = receipt.SetExtraData(deployReceipt); err != nil {
				return errors.Wrap(err, "execution failed (IxLogicDeploy)")
			}

		// Invoke Logic Interaction
		case types.IxLogicInvoke:
			payload, err := ix.GetLogicPayload()
			if err != nil {
				return err
			}

			// Perform logic invoke and record fuel consumed
			fuelConsumed, invokeReceipt, err := executor.LogicInvoke(
				tank,
				executor.getStateObject(payload.Logic.Address()),
				executor.getStateObject(ix.Sender()),
				payload,
			)
			if err != nil {
				return errors.Wrapf(err, "execution failed (%v)", ixtype)
			}

			if invokeReceipt.Error != nil {
				receipt.Status = types.ReceiptFailed
			}

			// Update fuel consumption on the receipt
			receipt.IncreaseFuelUsed(fuelConsumed)
			// Exhaust fuel from tank
			if !tank.Exhaust(fuelConsumed) {
				return errors.Wrapf(types.ErrInvalidInteractionType, "execution failed (%v)", ixtype)
			}

			if err = receipt.SetExtraData(invokeReceipt); err != nil {
				return errors.Wrap(err, "execution failed (IxLogicExecute)")
			}

		default:
			return errors.Wrapf(types.ErrInvalidInteractionType, "execution failed (%v)", ixtype)
		}

		// Deduct fuel for the ix execution from the sender
		executor.getStateObject(ix.Sender()).DeductFuel(tank.Consumed)

		// Update Sarga state if the interaction receiver if it is an unregistered (new) account
		if err := executor.updateSargaState(ix); err != nil {
			return errors.Wrap(err, "execution failed")
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
	receipt := executor.getReceipt(ix)

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
				receipt.Hashes.SetContextHash(addr, hash)

				continue
			}

			// If the account already exists, fall
			// through and update an existing context
			fallthrough

		// Sender Address / Sarga Address
		case ix.Sender(), types.SargaAddress:
			// Retrieve the state object for address and update its context
			hash, err := executor.getStateObject(addr).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(types.ErrUpdatingContext, err.Error())
			}

			// Update the execution receipt for the interaction with the new context hash for the sender
			receipt.Hashes.SetContextHash(addr, hash)
		}
	}

	return nil
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the StateObject of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects(ix *types.Interaction) error {
	// Retrieve the receipt for the interaction
	receipt := executor.getReceipt(ix)

	// Commit the sender state object (if it exists)
	if !ix.Sender().IsNil() {
		senderHash, err := executor.getStateObject(ix.Sender()).Commit()
		if err != nil {
			return err
		}

		receipt.Hashes.SetStateHash(ix.Sender(), senderHash)
	}

	// Commit the receiver state object (if it exists)
	if !ix.Receiver().IsNil() {
		receiverHash, err := executor.getStateObject(ix.Receiver()).Commit()
		if err != nil {
			return err
		}

		receipt.Hashes.SetStateHash(ix.Receiver(), receiverHash)
	}

	// Check if the receiver account is registered
	accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	// If the receiver account is new (unregistered),
	// commit the state object of the sarga account
	if !accountRegistered {
		genesisHash, err := executor.getStateObject(types.SargaAddress).Commit()
		if err != nil {
			return err
		}

		receipt.Hashes.SetStateHash(types.SargaAddress, genesisHash)
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
			if err := executor.state.Revert(executor.snapshots[types.SargaAddress]); err != nil {
				return err // This should not happen
			}
		}
	}

	return nil
}

// getReceipt returns the types.Receipt object for a given interaction hash.
// If there is no receipt for the hash, a new Receipt is generated, stored and returned.
func (executor *IxExecutor) getReceipt(ix *types.Interaction) *types.Receipt {
	// Return receipt from executor's receipts if it exists
	if receipt, ok := executor.receipts[ix.Hash()]; ok {
		// Return the existing receipt
		return receipt
	}

	// Create new receipt for interaction hash if it does not exist
	receipt := &types.Receipt{
		IxHash:   ix.Hash(),
		IxType:   ix.Type(),
		Hashes:   make(types.ReceiptAccHashes),
		FuelUsed: new(big.Int),
	}

	// Set the created receipt into the executor
	executor.receipts[ix.Hash()] = receipt
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
	sargaObject := executor.getStateObject(types.SargaAddress)
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
			genesisObject, err := executor.state.GetDirtyObject(types.SargaAddress)
			if err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}

			// Add genesis state object and its snapshot to the executor
			executor.objects[types.SargaAddress] = genesisObject
			executor.snapshots[types.SargaAddress] = genesisObject.Copy()

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
