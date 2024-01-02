package compute

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// IxExecutor represents an executor instance that handles the
// execution of a group of Interactions within a single cluster.
type IxExecutor struct {
	Interactions common.Interactions
	contextDelta common.ContextDelta

	mgr   *Manager
	state StateManager

	objects   state.ObjectMap
	snapshots state.ObjectMap

	receipts     map[common.Hash]*common.Receipt
	commitHashes map[identifiers.Address]common.Hash
}

// Execute executes all the given Interactions with their context delta.
// Returns an error if the execution of any interaction fails.
func (executor *IxExecutor) Execute(ixs common.Interactions, ctx *common.ExecutionContext) error {
	// Update the interaction and the context delta into the executor
	executor.Interactions = ixs
	executor.contextDelta = ctx.ContextDelta()

	for _, ix := range executor.Interactions {
		// Load the state objects for interaction participants
		if err := executor.LoadStateObjects(ix); err != nil {
			return errors.Wrap(err, "execution failed")
		}

		// Run the interaction
		receipt, err := executor.mgr.runInteraction(ix, ctx, executor.objects, true)
		if err != nil {
			return err
		}

		// Increment the nonce of the sender address
		if ix.Sender() != identifiers.NilAddress {
			executor.objects.GetObject(ix.Sender()).IncrementNonce(1)
		}

		// Set the receipt to the executor
		executor.setReceipt(ix.Hash(), receipt)
		// Deduct fuel for the ix execution from the sender
		executor.objects.GetObject(ix.Sender()).DeductFuel(new(big.Int).SetUint64(receipt.FuelUsed))

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

// Revert reverts all state objects modified by the IxExecutor to their original state.
func (executor *IxExecutor) Revert() error {
	for _, ix := range executor.Interactions {
		// Revert sender state object to snapshot state
		if !ix.Sender().IsNil() {
			if err := executor.state.Revert(executor.snapshots.GetObject(ix.Sender())); err != nil {
				return err // This should not happen
			}
		}

		// Revert receiver state object to snapshot state
		if !ix.Receiver().IsNil() {
			if err := executor.state.Revert(executor.snapshots.GetObject(ix.Receiver())); err != nil {
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
			if err := executor.state.Revert(executor.snapshots.GetObject(common.SargaAddress)); err != nil {
				return err // This should not happen
			}
		}
	}

	return nil
}

// Receipts returns all the execution receipts in the executor as
// types.Receipt objects in a map index by their interaction hash.
func (executor *IxExecutor) Receipts() common.Receipts {
	return executor.receipts
}

// getReceipt returns a common.Receipt object for a given interaction hash from executor cache
func (executor *IxExecutor) getReceipt(ix *common.Interaction) (*common.Receipt, bool) {
	// Return receipt from executor's receipts if it exists
	receipt, ok := executor.receipts[ix.Hash()]
	return receipt, ok //nolint:nlreturn
}

// setReceipt sets a common.Receipt object to the executor's cache
func (executor *IxExecutor) setReceipt(ixhash common.Hash, receipt *common.Receipt) {
	executor.receipts[ixhash] = receipt
}

// UpdateContext updates the context of the participant accounts updates the context hashes for
// the interaction receipt with the updated context hashes for all the interaction participants.
// If the interaction receiver is a new account, the context and hash for the sarga account is also updated.
func (executor *IxExecutor) UpdateContext(ix *common.Interaction, contextDelta common.ContextDelta) error {
	// Retrieve the receipt for the interaction
	receipt, ok := executor.getReceipt(ix)
	if !ok {
		return errors.New("cannot find receipt for interaction")
	}

	for addr, delta := range contextDelta {
		// For the address of each delta group in the context delta, determine action
		// based on whether it is the sender, receiver or the sarga address
		switch addr {
		// Receiver Address
		case ix.Receiver():
			var hash common.Hash

			// Check if the account is registered
			accountRegistered, err := executor.state.IsAccountRegistered(addr)
			if err != nil {
				return err
			}

			// Create a context for the account state object if it is a new account
			if !accountRegistered {
				if hash, err = executor.objects.GetObject(addr).CreateContext(
					delta.BehaviouralNodes,
					delta.RandomNodes,
				); err != nil {
					return errors.Wrap(common.ErrContextCreation, err.Error())
				}

				// Update the execution receipt for the interaction with the new context hash for the receiver
				receipt.Hashes.SetContextHash(addr, hash)

				continue
			}

			// If the account already exists, fall
			// through and update an existing context
			fallthrough

		// Sender Address / Sarga Address
		case ix.Sender(), common.SargaAddress:
			// Retrieve the state object for address and update its context
			hash, err := executor.objects.GetObject(addr).UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
			if err != nil {
				return errors.Wrap(common.ErrUpdatingContext, err.Error())
			}

			// Update the execution receipt for the interaction with the new context hash for the sender
			receipt.Hashes.SetContextHash(addr, hash)
		}
	}

	return nil
}

// updateSargaState will update the Sarga Object with the account genesis
// information for the receiver of an Interaction if it has not been registered.
// If the receiver address is already registered, no change is performed.
func (executor *IxExecutor) updateSargaState(ix *common.Interaction) error {
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
	sargaObject := executor.objects.GetObject(common.SargaAddress)
	if sargaObject == nil {
		return errors.New("sarga object not found")
	}

	// Add the account genesis information for the new account
	return sargaObject.AddAccountGenesisInfo(ix.Receiver(), ix.Hash())
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the Object of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects(ix *common.Interaction) error {
	// Retrieve the receipt for the interaction
	receipt, ok := executor.getReceipt(ix)
	if !ok {
		return errors.New("cannot find receipt for interaction")
	}

	// Commit the sender state object (if it exists)
	if !ix.Sender().IsNil() {
		senderHash, err := executor.objects.GetObject(ix.Sender()).Commit()
		if err != nil {
			return err
		}

		receipt.Hashes.SetStateHash(ix.Sender(), senderHash)
	}

	// Commit the receiver state object (if it exists)
	if !ix.Receiver().IsNil() {
		receiverHash, err := executor.objects.GetObject(ix.Receiver()).Commit()
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
		genesisHash, err := executor.objects.GetObject(common.SargaAddress).Commit()
		if err != nil {
			return err
		}

		receipt.Hashes.SetStateHash(common.SargaAddress, genesisHash)
	}

	return nil
}

func (executor *IxExecutor) LoadStateObjects(ix *common.Interaction) error {
	// Fetch state object for sender if valid and not already available in the executor
	if sender := ix.Sender(); !sender.IsNil() && executor.objects.GetObject(sender) == nil {
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
		var receiverObject *state.Object

		// Check if the receiver address is an already registered account
		accountRegistered, err := executor.state.IsAccountRegistered(ix.Receiver())
		if err != nil {
			return errors.Wrap(err, "state object fetch failed")
		}

		if !accountRegistered {
			// Retrieve the dirty object for genesis (sarga) address
			genesisObject, err := executor.state.GetDirtyObject(common.SargaAddress)
			if err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}

			// Add genesis state object and its snapshot to the executor
			executor.objects[common.SargaAddress] = genesisObject
			executor.snapshots[common.SargaAddress] = genesisObject.Copy()

			// Create a new dirty state object for the account
			receiverObject = executor.state.CreateDirtyObject(receiver, common.AccTypeFromIxType(ix.Type()))
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
