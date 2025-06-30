package compute

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
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

	commitHashes common.AccountStateHashes
}

// Execute executes all the given Interactions with their context delta.
// Returns an error if the execution of any interaction fails.
func (executor *IxExecutor) Execute(ixs common.Interactions, ctx *common.ExecutionContext) error {
	// Update the interaction and the context delta into the executor
	executor.Interactions = ixs
	executor.execContext = ctx

	for _, ix := range executor.Interactions.IxList() {
		checkpoint := executor.transition.Snapshot()

		// Execute the interaction using the transition state
		if err := executor.executeInteraction(ix, ctx, checkpoint); err != nil {
			executor.transition.UpdateSnapshot(checkpoint)
			executor.metrics.captureNumOfExecutionFailure(1)

			return errors.Wrap(err, "execution failed")
		}
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
	snapshot *state.Transition,
) error {
	// Run the interaction
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

// SuccessIxParticipants returns a map indicating whether a participant is part of a successful interaction.
func (executor *IxExecutor) SuccessIxParticipants() map[identifiers.Identifier]bool {
	var (
		ps = make(map[identifiers.Identifier]bool)
		rs = executor.transition.Receipts()
	)

	for _, ix := range executor.Interactions.IxList() {
		// mark sender of failed ixn as success as it requires state change
		ps[ix.SenderID()] = true

		if rs.IsSuccess(ix.Hash()) {
			for id := range ix.Participants() {
				ps[id] = true
			}
		}
	}

	return ps
}

// CommitObject defines the methods for participant state management
type CommitObject interface {
	Data() *common.Account
	Commit() (common.Hash, error)
	ContextHash() common.Hash
}

// commitObject commits the state object of a participant to state db.
func (executor *IxExecutor) commitObject(
	ps common.ParticipantInfo, obj CommitObject,
	isSuccess map[identifiers.Identifier]bool,
) error {
	if !isSuccess[ps.ID] {
		executor.commitHashes.SetStateHash(ps.ID, common.NilHash)
		executor.commitHashes.SetContextHash(ps.ID, common.NilHash)

		return nil
	}

	previousHash, err := obj.Data().Hash()
	if err != nil {
		return err
	}

	if ps.LockType > common.MutateLock {
		executor.commitHashes.SetStateHash(ps.ID, previousHash)
		executor.commitHashes.SetContextHash(ps.ID, obj.Data().ContextHash)

		return nil
	}

	newHash, err := obj.Commit()
	if err != nil {
		return err
	}

	executor.commitHashes.SetStateHash(ps.ID, newHash)
	executor.commitHashes.SetContextHash(ps.ID, obj.ContextHash())

	return nil
}

// CommitStateObjects commits all state objects of the interaction participants including the system object to
// the state db. If the interaction receiver is a new account, the state object of the sarga account is also committed.
func (executor *IxExecutor) CommitStateObjects() error {
	participants := executor.Interactions.Participants()
	isSuccess := executor.SuccessIxParticipants()

	for id, ps := range participants {
		var object CommitObject

		if id == common.SystemAccountID {
			object = executor.transition.GetSystemObject()
		} else {
			object = executor.transition.GetObject(id)
		}

		err := executor.commitObject(ps, object, isSuccess)
		if err != nil {
			return err
		}
	}

	return nil
}
