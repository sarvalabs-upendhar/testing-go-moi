package compute

import (
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-pisa"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/state"
)

func init() {
	engineio.RegisterRuntime(pisa.NewRuntime(), crypto.Cryptographer(0))
}

// Manager represents a type for managing interaction execution across multiple consensus clusters.
// It also manages execution environment generation for logic execution.
type Manager struct {
	logger    hclog.Logger
	config    *config.ExecutionConfig
	metrics   *Metrics
	state     StateManager
	executors sync.Map
}

// NewManager creates a new compute.Manager instance
// for a given state interface, logger and ExecutionConfig
func NewManager(state StateManager, logger hclog.Logger, config *config.ExecutionConfig, metrics *Metrics) *Manager {
	return &Manager{
		state:   state,
		config:  config,
		logger:  logger.Named("Compute-Manager"),
		metrics: metrics,
	}
}

// SpawnExecutor generates a new IxExecutor instance with a given fuel limit.
func (manager *Manager) SpawnExecutor() *IxExecutor {
	return &IxExecutor{
		mgr:   manager,
		state: manager.state,

		baseline:     NewTransition(),
		transition:   NewTransition(),
		metrics:      manager.metrics,
		commitHashes: make(common.AccStateHashes),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The generated executor instances is indexed by the given Cluster ID.
func (manager *Manager) ExecuteInteractions(
	ixs common.Interactions,
	ctx *common.ExecutionContext,
) (
	common.Receipts, common.AccStateHashes, error,
) {
	// Spawn a new IxExecutor instance
	executor := manager.SpawnExecutor()
	// Execute all the given interactions
	if err := executor.Execute(ixs, ctx); err != nil {
		return nil, nil, err
	}

	// Update the state objects in the state manager
	if err := manager.state.UpdateStateObjects(executor.transition.objects); err != nil {
		return nil, nil, errors.Wrap(err, "failed to update state objects")
	}

	// Store the executor into the execution instances indexed by the cluster ID
	manager.executors.Store(ctx.Cluster, executor)
	// Generate the execution receipts and return
	return executor.Receipts(), executor.commitHashes, nil
}

func (manager *Manager) InteractionCall(
	ctx *common.ExecutionContext,
	ix *common.Interaction,
	hashes map[identifiers.Address]common.Hash,
) (*common.Receipt, error) {
	// Fetch state objects for the interaction
	objects, err := FetchIxStateObjects(manager.state, ix, hashes)
	if err != nil {
		return nil, err
	}

	transition := &Transition{
		objects: objects,
	}

	// Run the interaction and return the receipt
	return manager.runInteraction(ix, ctx, transition, false)
}

func (manager *Manager) runInteraction(
	ix *common.Interaction, ctx *common.ExecutionContext,
	transition *Transition, useIxFuelLimit bool,
) (
	receipt *common.Receipt, err error,
) {
	var tank *FuelTank

	if useIxFuelLimit {
		// Determine the tank limit from the interaction
		tank = NewFuelTank(ix.FuelLimit())

		// Check that the sender has sufficient balance
		if ok, _ := transition.objects.GetObject(ix.Sender()).HasSufficientFuel(ix.Cost()); !ok {
			receipt = common.NewReceipt(ix)
			receipt.Status = common.ReceiptFuelExhausted

			return receipt, nil
		}
	} else {
		// Determine the tank limit from the node configuration
		tank = NewFuelTank(manager.config.FuelLimit)
	}

	ixtype := ix.Type()
	// Lookup the runner for the interaction type
	runner := lookupIxRunner(ixtype)

	// Set up a defer function to recover from any panic
	// that may occur while executing the interaction
	defer func() {
		if trace := recover(); trace != nil {
			err = errors.New("execution failed: executor panicked!")

			manager.logger.Debug("EXECUTION PANIC OCCURRED", "trace:", trace)
		}
	}()

	// Call the interaction runner and get the receipt
	receipt = runner(ix, ctx, tank, transition.objects)

	return receipt, nil
}

// Revert reverts any state transition performed by an executor for a given Cluster ID and deletes the dirty objects
// Returns an error if no executor exists for the cluster ID or if any error occurs during the state revert.
func (manager *Manager) Revert(cluster common.ClusterID) error {
	// Attempt to load an executor instance for the cluster ID
	instance, ok := manager.executors.Load(cluster)
	if !ok {
		return common.ErrExecutorNotFound
	}

	// Assert the instance into an IxExecutor
	executor, ok := instance.(*IxExecutor)
	if !ok {
		return common.ErrInterfaceConversion
	}

	for addr := range executor.transition.objects {
		executor.state.Cleanup(addr)
	}

	// Revert executor state
	executor.transition = executor.baseline

	return nil
}

// Cleanup removes the executor instance for the given Cluster ID if one exists.
func (manager *Manager) Cleanup(cluster common.ClusterID) {
	manager.executors.Delete(cluster)
}

func (manager *Manager) ValidateLogicDeploy(ix *common.Interaction, data []byte) error {
	manifest, err := engineio.NewManifest(data, engineio.POLO)
	if err != nil {
		return err
	}

	runtime, ok := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	if !ok {
		return errors.New("failed to get runtime for logic")
	}

	logicDescriptor, _, err := runtime.CompileManifest(ix.FuelLimit(), manifest)
	if err != nil {
		return err
	}

	logicObject := state.NewLogicObject(ix.Receiver(), logicDescriptor)

	if ix.Callsite() == "" {
		return nil
	}

	return runtime.ValidateCalldata(logicObject, ix)
}

func (manager *Manager) ValidateLogicInvoke(ix *common.Interaction) error {
	stateObject, err := manager.state.GetLatestStateObject(ix.Receiver())
	if err != nil {
		return err
	}

	logicObject, err := stateObject.FetchLogicObject(ix.LogicID())
	if err != nil {
		return err
	}

	runtime, ok := engineio.FetchEngineRuntime(logicObject.Engine())
	if !ok {
		return errors.New("failed to get runtime for logic")
	}

	return runtime.ValidateCalldata(logicObject, ix)
}
