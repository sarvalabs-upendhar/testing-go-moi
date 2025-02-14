package compute

import (
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/state"
)

func init() {
	engineio.RegisterEngine(pisa.NewEngine())
}

// Manager represents a type for managing interaction execution across multiple consensus clusters.
// It also manages execution environment generation for logic execution.
type Manager struct {
	logger  hclog.Logger
	config  *config.ExecutionConfig
	metrics *Metrics
}

// NewManager creates a new compute.Manager instance
// for a given state interface, logger and ExecutionConfig
func NewManager(logger hclog.Logger, config *config.ExecutionConfig, metrics *Metrics) *Manager {
	return &Manager{
		config:  config,
		logger:  logger.Named("Compute-Manager"),
		metrics: metrics,
	}
}

// SpawnExecutor generates a new IxExecutor instance with a given fuel limit.
func (manager *Manager) SpawnExecutor(transition *state.Transition) *IxExecutor {
	return &IxExecutor{
		mgr:          manager,
		transition:   transition,
		metrics:      manager.metrics,
		commitHashes: make(common.AccStateHashes),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The executor instance is indexed by the given Cluster ID.
func (manager *Manager) ExecuteInteractions(
	transition *state.Transition,
	ixs common.Interactions,
	ctx *common.ExecutionContext,
) (
	common.AccStateHashes, error,
) {
	// Spawn a new IxExecutor instance
	executor := manager.SpawnExecutor(transition)
	// Execute all the given interactions
	if err := executor.Execute(ixs, ctx); err != nil {
		return nil, err
	}

	// Generate the execution receipts and return
	return executor.commitHashes, nil
}

func (manager *Manager) InteractionCall(
	ctx *common.ExecutionContext,
	ix *common.Interaction,
	transition *state.Transition,
) (*common.Receipt, error) {
	// Run the interaction and return the receipt
	return manager.runInteraction(ix, ctx, transition, false)
}

func (manager *Manager) runInteraction(
	ix *common.Interaction, ctx *common.ExecutionContext,
	transition *state.Transition, useIxFuelLimit bool,
) (
	receipt *common.Receipt, err error,
) {
	var tank *FuelTank

	receipt = common.NewReceipt(ix)

	if useIxFuelLimit {
		// Determine the tank limit from the interaction
		tank = NewFuelTank(ix.FuelLimit())

		// Check that the sender has sufficient balance
		if ok, _ := transition.HasSufficientFuel(ix.SenderID(), ix.Cost()); !ok {
			receipt.Status = common.ReceiptInsufficientFuel

			return receipt, nil
		}
	} else {
		// Determine the tank limit from the node configuration
		tank = NewFuelTank(manager.config.FuelLimit)
	}

	// Set up a defer function to recover from any panic
	// that may occur while executing the interaction
	defer func() {
		if trace := recover(); trace != nil {
			err = errors.New("execution failed: executor panicked!")

			manager.logger.Debug("EXECUTION PANIC OCCURRED", "trace:", trace)
		}
	}()

	receipt.IxOps = make([]*common.IxOpResult, 0)

	for idx, op := range ix.Ops() {
		// Lookup the runner for the operation type
		runner := lookupOpRunner(op.Type())
		// Call the interaction runner and get the result
		opResult := runner(ix.GetIxOp(idx), ctx, tank, transition)

		receipt.AddIxOpResult(opResult)

		if opResult.Status >= common.ResultStateReverted {
			manager.logger.Trace("ixOp reverted", "op-hash", op.Hash(), "status", opResult.Status)

			receipt.SetStatus(common.ReceiptStateReverted)
			// in case of any error while executing the ixOp, we should consume total full
			receipt.SetFuelUsed(ix.FuelLimit())

			return receipt, err
		}
	}

	receipt.SetFuelUsed(tank.Consumed)

	return receipt, err
}

func addNewAccountsToSargaAccount(
	transition *state.Transition,
	ixHash common.Hash,
	ids ...identifiers.Identifier,
) error {
	// get sarga object
	sargaObject := transition.GetObject(common.SargaAccountID)

	for _, id := range ids {
		if !transition.IsGenesis(id) {
			continue
		}

		if sargaObject == nil {
			return errors.New("sarga object not found")
		}

		registered, err := sargaObject.IsAccountRegistered(id)
		if err != nil {
			return err
		}

		if registered {
			return common.ErrAccountAlreadyRegistered
		}

		// Add the genesis account information of the new account
		err = sargaObject.AddAccountGenesisInfo(id, ixHash)
		if err != nil {
			return err
		}
	}

	return nil
}
