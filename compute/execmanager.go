package compute

import (
	"log"
	"sync"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/config"
	"github.com/sarvalabs/moichain/compute/engineio"
	"github.com/sarvalabs/moichain/compute/pisa"
	"github.com/sarvalabs/moichain/state"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

// ExecutionManager represents a type for managing interaction execution across multiple consensus clusters.
// It also manages execution environment generation for logic execution.
type ExecutionManager struct {
	logger hclog.Logger
	config *config.ExecutionConfig

	state     stateManager
	executors sync.Map
}

// stateManager describes a state management interface
type stateManager interface {
	// Revert must revert the state of an address to given state.Object
	Revert(*state.Object) error
	// IsAccountRegistered must return whether an address has an existing, registered account
	IsAccountRegistered(common.Address) (bool, error)

	// GetDirtyObject must retrieve the state.Object for a given types.Address
	GetDirtyObject(common.Address) (*state.Object, error)
	// CreateDirtyObject must generate a new dirty state.Object for the given types.Address
	CreateDirtyObject(common.Address, common.AccountType) *state.Object
	// GetLatestStateObject must return the latest state.Object for the given types.Address
	GetLatestStateObject(addr common.Address) (*state.Object, error)
}

// NewExecutionManager creates a new ExecutionManager instance
// for a given state interface, logger and ExecutionConfig
func NewExecutionManager(
	state stateManager,
	logger hclog.Logger,
	config *config.ExecutionConfig,
) *ExecutionManager {
	return &ExecutionManager{
		state:  state,
		config: config,
		logger: logger.Named("Execution-Manager"),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The generated executor instances is indexed by the given Cluster ID.
func (exec *ExecutionManager) ExecuteInteractions(
	cluster common.ClusterID,
	ixs common.Interactions,
	delta common.ContextDelta,
) (
	common.Receipts, error,
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

		objects:   make(map[common.Address]*state.Object),
		snapshots: make(map[common.Address]*state.Object),

		receipts:     make(map[common.Hash]*common.Receipt),
		commitHashes: make(map[common.Address]common.Hash),
	}
}

// Revert reverts any state transition performed by an executor for a given Cluster ID.
// Returns an error if no executor exists for the cluster ID or if any error occurs during the state revert.
func (exec *ExecutionManager) Revert(cluster common.ClusterID) error {
	// Attempt to load an executor instance for the cluster ID
	instance, ok := exec.executors.Load(cluster)
	if !ok {
		return common.ErrExecutorNotFound
	}

	// Assert the instance into an IxExecutor
	executor, ok := instance.(*IxExecutor)
	if !ok {
		return common.ErrInterfaceConversion
	}

	// Revert executor state
	return executor.Revert()
}

// Cleanup removes the executor instance for the given Cluster ID, if one exists.
func (exec *ExecutionManager) Cleanup(cluster common.ClusterID) {
	exec.executors.Delete(cluster)
}

func (exec *ExecutionManager) LogicCall(
	logicID common.LogicID,
	sender common.Address,
	callsite string,
	calldata []byte,
) (engineio.Fuel, *common.LogicInvokeReceipt, error) {
	logicStateObject, err := exec.state.GetLatestStateObject(logicID.Address())
	if err != nil {
		return nil, nil, err
	}

	senderStateObject, err := exec.state.GetLatestStateObject(sender)
	if err != nil {
		return nil, nil, err
	}

	options := make([]LogicInvokeOption, 0, 3)
	// Append invoker options for invoker state and fuel limit
	options = append(options, InvokerState(senderStateObject))
	options = append(options, InvokeCall(callsite, calldata))

	return InvokeLogic(logicID, logicStateObject, options...)
}

func (exec *ExecutionManager) createFuelTank(ix *common.Interaction) *engineio.FuelTank {
	switch ix.FuelLimit().Cmp(exec.config.FuelLimit) {
	case -1:
		// If Ix Fuel Limit < Node Fuel Limit
		return engineio.NewFuelTank(ix.FuelLimit())
	default:
		// If Ix Fuel Limit >= Node Fuel Limit
		return engineio.NewFuelTank(exec.config.FuelLimit)
	}
}
