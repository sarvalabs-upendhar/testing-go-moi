package compute

import (
	"log"
	"math/big"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	pisa "github.com/sarvalabs/go-pisa/moi"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

// Manager represents a type for managing interaction execution across multiple consensus clusters.
// It also manages execution environment generation for logic execution.
type Manager struct {
	logger hclog.Logger
	config *config.ExecutionConfig

	state     StateManager
	executors sync.Map
}

// NewManager creates a new compute.Manager instance
// for a given state interface, logger and ExecutionConfig
func NewManager(state StateManager, logger hclog.Logger, config *config.ExecutionConfig) *Manager {
	return &Manager{
		state:  state,
		config: config,
		logger: logger.Named("Compute-Manager"),
	}
}

// SpawnExecutor generates a new IxExecutor instance with a given fuel limit.
func (manager *Manager) SpawnExecutor() *IxExecutor {
	return &IxExecutor{
		mgr:   manager,
		state: manager.state,

		objects:   make(state.ObjectMap),
		snapshots: make(state.ObjectMap),

		receipts:     make(map[common.Hash]*common.Receipt),
		commitHashes: make(map[common.Address]common.Hash),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The generated executor instances is indexed by the given Cluster ID.
func (manager *Manager) ExecuteInteractions(
	ixs common.Interactions,
	cluster common.ClusterID,
	delta common.ContextDelta,
) (
	common.Receipts, error,
) {
	// Spawn a new IxExecutor instance
	executor := manager.SpawnExecutor()
	// Execute all the given interactions
	if err := executor.Execute(ixs, delta); err != nil {
		if err := executor.Revert(); err != nil {
			log.Fatal(err) // todo: this should not happen
		}

		return nil, err
	}

	// Store the executor into the execution instances indexed by the cluster ID
	manager.executors.Store(cluster, executor)
	// Generate the execution receipts and return
	return executor.Receipts(), nil
}

func (manager *Manager) InteractionCall(
	ix *common.Interaction,
	hashes map[common.Address]common.Hash,
) (*common.Receipt, error) {
	// Fetch state objects for the interaction
	objects, err := FetchIxStateObjects(manager.state, ix, hashes)
	if err != nil {
		return nil, err
	}

	// Run the interaction and return the receipt
	return manager.runInteraction(ix, objects, true)
}

func (manager *Manager) runInteraction(
	ix *common.Interaction,
	objects state.ObjectMap,
	useIxFuelLimit bool,
) (
	*common.Receipt, error,
) {
	// Determine the fuel limit
	var limit *big.Int
	if useIxFuelLimit {
		limit = ix.FuelLimit()
	}

	// Create a fuel tank for the interaction
	tank := manager.createFuelTank(limit)

	// Determine the required balance for MOI Token.
	// Must be the sum of the fuel limit for the ix and the transfer value for the MOI Token
	requiredBalance := new(big.Int).Add(tank.Level(), ix.MOITokenValue())
	// Check that the sender has sufficient balance of MOI Tokens
	ok, err := objects.GetObject(ix.Sender()).HasFuel(requiredBalance)
	if err != nil {
		return nil, errors.Wrap(err, "execution failed: fuel check")
	}

	if !ok {
		return nil, errors.Errorf("execution failed: insufficient fuel")
	}

	ixtype := ix.Type()
	// Lookup the runner for the interaction type
	runner, ok := LookupIxRunner(ixtype)
	if !ok {
		return nil, errors.Wrapf(common.ErrInvalidInteractionType, "execution failed (%v)", ixtype)
	}

	// Call the interaction runner and get the receipt
	receipt, err := runner(ix, tank, objects)
	if err != nil {
		return nil, errors.Wrapf(err, "execution failed (%v)", ixtype)
	}

	return receipt, nil
}

// createFuelTank creates a new engineio.FuelTank for a given fuel limit.
// If no limit is provided (limit == nil), then the `execution.fuel_limit`
// parameter from the node configuration will be used as the fuel limit
func (manager *Manager) createFuelTank(limit *big.Int) *FuelTank {
	// If no limit is provided, determine limit from execution config
	if limit == nil {
		return NewFuelTank(manager.config.FuelLimit)
	}

	// Return fuel tank with given limit
	return NewFuelTank(limit)
}

// Revert reverts any state transition performed by an executor for a given Cluster ID.
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

	// Revert executor state
	return executor.Revert()
}

// Cleanup removes the executor instance for the given Cluster ID, if one exists.
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
