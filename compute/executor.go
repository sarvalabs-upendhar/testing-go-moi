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

	// Increment the sequenceID of the sender id
	if ix.SenderID() != identifiers.Nil {
		executor.transition.IncrementSequenceID(ix.SenderID(), ix.SenderKeyID())
	}

	// Set the receipt to the transition
	executor.transition.SetReceipt(ix.Hash(), receipt)

	// Deduct fuel for the ix execution from the sender
	executor.transition.DeductFuel(
		ix.SenderID(),
		new(big.Int).Mul(ix.FuelPrice(), new(big.Int).SetUint64(receipt.FuelUsed)),
	)

	return nil
}

// UpdateContext updates the context of the participant accounts using context delta
func (executor *IxExecutor) UpdateContext() error {
	for id, object := range executor.transition.Objects() {
		delta, ok := executor.execContext.ContextDelta()[id]
		if !ok {
			continue
		}

		// Create a context for the account state object if it is a new account
		if executor.transition.IsGenesis(object.Identifier()) {
			if err := object.CreateContext(delta.ConsensusNodes); err != nil {
				return errors.Wrap(common.ErrContextCreation, err.Error())
			}

			continue
		}

		err := object.UpdateContext(delta.ConsensusNodes)
		if err != nil {
			return errors.Wrap(common.ErrContextCreation, err.Error())
		}
	}

	return nil
}

// CommitStateObjects commits all StateObjects of the interaction participants to the state db.
// If the interaction receiver is a new account, the Object of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects() error {
	for id, ps := range executor.Interactions.Participants() {
		obj := executor.transition.GetObject(id)

		previousHash, err := obj.Data().Hash()
		if err != nil {
			return err
		}

		if ps.LockType > common.MutateLock {
			executor.commitHashes.SetStateHash(id, previousHash)
			executor.commitHashes.SetContextHash(id, obj.Data().ContextHash)

			continue
		}

		newHash, err := obj.Commit()
		if err != nil {
			return err
		}

		if newHash == previousHash {
			executor.commitHashes.SetStateHash(id, common.NilHash)
			executor.commitHashes.SetContextHash(id, common.NilHash)

			continue
		}

		executor.commitHashes.SetStateHash(id, newHash)
		executor.commitHashes.SetContextHash(id, obj.ContextHash())
	}

	return nil
}
