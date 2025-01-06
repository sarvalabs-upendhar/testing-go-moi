package compute

import (
	"math/big"

	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// IxExecutor represents an executor instance that handles the
// execution of a group of Interactions within a single cluster.
type IxExecutor struct {
	Interactions common.Interactions
	execContext  *common.ExecutionContext

	mgr *Manager

	transition *state.Transition

	metrics *Metrics

	commitHashes common.AccStateHashes
}

// Execute executes all the given Interactions with their context delta.
// Returns an error if the execution of any interaction fails.
func (executor *IxExecutor) Execute(ixs common.Interactions, ctx *common.ExecutionContext) error {
	// Update the interaction and the context delta into the executor
	executor.Interactions = ixs
	executor.execContext = ctx

	checkpoint := executor.transition.Snapshot()

	for _, ix := range executor.Interactions.IxList() {
		// Execute the interaction using the transition state
		if err := executor.executeInteraction(ix, ctx); err != nil {
			executor.transition.UpdateSnapshot(checkpoint)
			executor.metrics.captureNumOfExecutionFailure(1)

			return errors.Wrap(err, "execution failed")
		}

		// After successful execution, update the executor checkpoint
		checkpoint = executor.transition.Snapshot()
	}

	// Update the context for participants
	if err := executor.UpdateContext(); err != nil {
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
	snapshot := executor.transition.Snapshot()

	receipt, err := executor.mgr.runInteraction(ix, ctx, executor.transition, true)
	if err != nil {
		return err
	}

	if receipt.Status >= common.ReceiptStateReverted {
		executor.transition.UpdateSnapshot(snapshot)
	}

	// Increment the sequenceID of the sender address
	if ix.SenderAddr() != identifiers.NilAddress {
		executor.transition.IncrementSequenceID(ix.SenderAddr(), ix.SenderKeyID())
	}

	// Set the receipt to the transition
	executor.transition.SetReceipt(ix.Hash(), receipt)

	// Deduct fuel for the ix execution from the sender
	executor.transition.DeductFuel(
		ix.SenderAddr(),
		new(big.Int).Mul(ix.FuelPrice(), new(big.Int).SetUint64(receipt.FuelUsed)),
	)

	return nil
}

// UpdateContext updates the context of the participant accounts using context delta
func (executor *IxExecutor) UpdateContext() error {
	for address, object := range executor.transition.Objects() {
		delta, ok := executor.execContext.ContextDelta()[address]
		if !ok {
			continue
		}

		// Create a context for the account state object if it is a new account
		if executor.transition.IsGenesis(object.Address()) {
			if _, err := object.CreateContext(delta.BehaviouralNodes, delta.RandomNodes); err != nil {
				return errors.Wrap(common.ErrContextCreation, err.Error())
			}

			continue
		}

		_, err := object.UpdateContext(delta.BehaviouralNodes, delta.RandomNodes)
		if err != nil {
			return errors.Wrap(common.ErrContextCreation, err.Error())
		}
	}

	return nil
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the Object of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects() error {
	for addr, ps := range executor.Interactions.Participants() {
		obj := executor.transition.GetObject(addr)

		previousHash, err := obj.Data().Hash()
		if err != nil {
			return err
		}

		if ps.LockType > common.MutateLock {
			executor.commitHashes.SetStateHash(addr, previousHash)
			executor.commitHashes.SetContextHash(addr, obj.Data().ContextHash)

			continue
		}

		newHash, err := obj.Commit()
		if err != nil {
			return err
		}

		if newHash == previousHash {
			executor.commitHashes.SetStateHash(addr, common.NilHash)
			executor.commitHashes.SetContextHash(addr, common.NilHash)

			continue
		}

		executor.commitHashes.SetStateHash(addr, newHash)
		executor.commitHashes.SetContextHash(addr, obj.ContextHash())
	}

	return nil
}
