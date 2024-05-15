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
	execContext  *common.ExecutionContext

	mgr   *Manager
	state StateManager

	baseline   *Transition
	transition *Transition

	metrics *Metrics

	commitHashes common.AccStateHashes
}

// Execute executes all the given Interactions with their context delta.
// Returns an error if the execution of any interaction fails.
func (executor *IxExecutor) Execute(ixs common.Interactions, ctx *common.ExecutionContext) error {
	// Update the interaction and the context delta into the executor
	executor.Interactions = ixs
	executor.execContext = ctx

	// Load all the state objects for all interaction participants
	objects, err := executor.LoadStateObjects(ctx.Participants)
	if err != nil {
		return errors.Wrap(err, "execution failed")
	}

	executor.baseline.objects = objects
	executor.baseline.receipts = make(map[common.Hash]*common.Receipt)

	executor.transition = executor.baseline.Copy()
	checkpoint := executor.transition.Copy()

	for _, ix := range executor.Interactions {
		// Execute the interaction using the transition state
		if err = executor.executeInteraction(ix, ctx); err != nil {
			// If the receiver account is new, delete the object
			accountRegistered, _ := executor.state.IsAccountRegistered(ix.Receiver())
			if !accountRegistered {
				executor.deleteObject(ix.Receiver())
				delete(checkpoint.objects, ix.Receiver())
			}

			executor.transition = checkpoint
			executor.metrics.captureNumOfExecutionFailure(1)

			return errors.Wrap(err, "execution failed")
		}

		// After successful execution, update the executor checkpoint
		checkpoint = executor.transition
	}

	// Update the context for participants
	if err = executor.UpdateContext(); err != nil {
		return errors.Wrap(err, "execution failed")
	}

	// Commit the state objects of all interaction participants
	if err := executor.CommitStateObjects(); err != nil {
		return errors.Wrap(err, "execution failed")
	}

	return nil
}

func (executor *IxExecutor) executeInteraction(
	ix *common.Interaction,
	ctx *common.ExecutionContext,
) error {
	// Run the interaction
	receipt, err := executor.mgr.runInteraction(ix, ctx, executor.transition, true)
	if err != nil {
		return err
	}

	if receipt.Status >= common.ReceiptStateReverted {
		return errors.New("state must be reverted")
	}

	// Increment the nonce of the sender address
	if ix.Sender() != identifiers.NilAddress {
		executor.transition.objects.GetObject(ix.Sender()).IncrementNonce(1)
	}

	// Set the receipt to the transition
	executor.setReceipt(ix.Hash(), receipt)

	// Deduct fuel for the ix execution from the sender
	executor.transition.objects.GetObject(ix.Sender()).DeductFuel(new(big.Int).SetUint64(receipt.FuelUsed))

	// Update Sarga state if the interaction receiver if it is an unregistered (new) account
	if err := executor.updateSargaState(ix); err != nil {
		return errors.Wrap(err, "execution failed")
	}

	return nil
}

// Receipts returns all the execution receipts in the executor as
// types.Receipt objects in a map index by their interaction hash.
func (executor *IxExecutor) Receipts() common.Receipts {
	return executor.transition.receipts
}

// setReceipt sets a common.Receipt object to the executor's cache
func (executor *IxExecutor) setReceipt(ixhash common.Hash, receipt *common.Receipt) {
	executor.transition.receipts[ixhash] = receipt
}

// UpdateContext updates the context of the participant accounts using context delta
func (executor *IxExecutor) UpdateContext() error {
	for address, object := range executor.transition.objects {
		delta, ok := executor.execContext.ContextDelta()[address]
		if !ok {
			continue
		}

		// Check if the account is registered
		accountRegistered, err := executor.state.IsAccountRegistered(object.Address())
		if err != nil {
			return err
		}

		// Create a context for the account state object if it is a new account
		if !accountRegistered {
			if _, err := object.CreateContext(delta.BehaviouralNodes, delta.RandomNodes); err != nil {
				return errors.Wrap(common.ErrContextCreation, err.Error())
			}

			continue
		}

		_, err = object.UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
		if err != nil {
			return errors.Wrap(common.ErrContextCreation, err.Error())
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
	sargaObject := executor.transition.objects.GetObject(common.SargaAddress)
	if sargaObject == nil {
		return errors.New("sarga object not found")
	}

	// Add the account genesis information for the new account
	return sargaObject.AddAccountGenesisInfo(ix.Receiver(), ix.Hash())
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the Object of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects() error {
	for address, object := range executor.transition.objects {
		h, err := object.Commit()
		if err != nil {
			return err
		}

		executor.commitHashes.SetStateHash(address, h)
		executor.commitHashes.SetContextHash(address, object.ContextHash())
	}

	return nil
}

func (executor *IxExecutor) LoadStateObjects(
	ps map[identifiers.Address]common.IxParticipant,
) (state.ObjectMap, error) {
	// Create a new objects map
	objects := make(state.ObjectMap)

	for addr, p := range ps {
		if p.IsGenesis {
			objects[addr] = executor.state.CreateStateObject(addr, p.AccType)

			continue
		}

		obj, err := executor.state.GetLatestStateObject(addr)
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		objects[addr] = obj
	}

	return objects, nil
}

func (executor *IxExecutor) deleteObject(addr identifiers.Address) {
	delete(executor.transition.objects, addr)
	delete(executor.baseline.objects, addr)
}
