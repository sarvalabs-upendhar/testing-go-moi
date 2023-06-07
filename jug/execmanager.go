package jug

import (
	"log"
	"sync"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/types"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

// ExecutionManager represents a type for managing interaction execution across multiple consensus clusters.
// It also manages execution environment generation for logic execution.
type ExecutionManager struct {
	logger hclog.Logger
	config *common.ExecutionConfig

	state     state
	executors sync.Map
}

// state describes a state management interface
type state interface {
	// Revert must revert the state of an address to given guna.StateObject
	Revert(*guna.StateObject) error
	// IsAccountRegistered must return whether an address has an existing, registered account
	IsAccountRegistered(types.Address) (bool, error)

	// GetDirtyObject must retrieve the guna.StateObject for a given types.Address
	GetDirtyObject(types.Address) (*guna.StateObject, error)
	// CreateDirtyObject must generate a new dirty guna.StateObject for the given types.Address
	CreateDirtyObject(types.Address, types.AccountType) *guna.StateObject
}

// NewExecutionManager creates a new ExecutionManager instance
// for a given state interface, logger and ExecutionConfig
func NewExecutionManager(
	state state,
	logger hclog.Logger,
	config *common.ExecutionConfig,
) *ExecutionManager {
	return &ExecutionManager{
		state:  state,
		config: config,
		logger: logger.Named("Execution"),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The generated executor instances is indexed by the given Cluster ID.
func (exec *ExecutionManager) ExecuteInteractions(
	cluster types.ClusterID,
	ixs types.Interactions,
	delta types.ContextDelta,
) (
	types.Receipts, error,
) {
	// Spawn a new IxExecutor instance
	executor := exec.SpawnExecutor()
	// Execute all the given interactions
	if err := executor.Execute(ixs, delta); err != nil {
		if err := executor.Revert(); err != nil {
			log.Fatal(err) // todo: this should not happen
		}

		return nil, err
	}

	// Store the executor into the execution instances indexed by the cluster ID
	exec.executors.Store(cluster, executor)
	// Generate the execution receipts and return
	return executor.Receipts(), nil
}

// SpawnExecutor generates a new IxExecutor instance with a given fuel limit.
func (exec *ExecutionManager) SpawnExecutor() *IxExecutor {
	return &IxExecutor{
		exec:  exec,
		state: exec.state,

		// tank:  engineio.NewFuelTank(engineio.Fuel(fuelLimit)),

		objects:   make(map[types.Address]*guna.StateObject),
		snapshots: make(map[types.Address]*guna.StateObject),

		receipts:     make(map[types.Hash]*types.Receipt),
		commitHashes: make(map[types.Address]types.Hash),
	}
}

// Revert reverts any state transition performed by an executor for a given Cluster ID.
// Returns an error if no executor exists for the cluster ID or if any error occurs during the state revert.
func (exec *ExecutionManager) Revert(cluster types.ClusterID) error {
	// Attempt to load an executor instance for the cluster ID
	instance, ok := exec.executors.Load(cluster)
	if !ok {
		return types.ErrExecutorNotFound
	}

	// Assert the instance into an IxExecutor
	executor, ok := instance.(*IxExecutor)
	if !ok {
		return types.ErrInterfaceConversion
	}

	// Revert executor state
	return executor.Revert()
}

// Cleanup removes the executor instance for the given Cluster ID, if one exists.
func (exec *ExecutionManager) Cleanup(cluster types.ClusterID) {
	exec.executors.Delete(cluster)
}

func (exec *ExecutionManager) createFuelTank(ix *types.Interaction) *engineio.FuelTank {
	switch ix.FuelLimit().Cmp(exec.config.FuelLimit) {
	case -1:
		// If Ix Fuel Limit < Node Fuel Limit
		return engineio.NewFuelTank(ix.FuelLimit())
	default:
		// If Ix Fuel Limit >= Node Fuel Limit
		return engineio.NewFuelTank(exec.config.FuelLimit)
	}
}
