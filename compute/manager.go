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
func (manager *Manager) SpawnExecutor(
	logger hclog.Logger,
	cfg *config.ExecutionConfig,
	transition *state.Transition,
) *IxExecutor {
	return &IxExecutor{
		logger:       logger,
		cfg:          cfg,
		transition:   transition,
		metrics:      manager.metrics,
		commitHashes: make(common.AccountStateHashes),
	}
}

// ExecuteInteractions executes the given interactions with an IxExecutor.
// The executor instance is indexed by the given Cluster ID.
func (manager *Manager) ExecuteInteractions(
	transition *state.Transition,
	ixs common.Interactions,
	ctx *common.ExecutionContext,
) (
	common.AccountStateHashes, error,
) {
	// Spawn a new IxExecutor instance
	executor := manager.SpawnExecutor(manager.logger, manager.config, transition)
	// Execute all the given interactions
	if err := executor.Execute(ixs, ctx); err != nil {
		return nil, err
	}

	// Generate the execution receipts and return
	return executor.commitHashes, nil
}

func AddActorsToRuntime(ix *common.Interaction, runtime engineio.Runtime, transition *state.Transition) error {
	// Iterate over all the actors in the transition
	for id, info := range ix.Participants() {
		if id == common.SargaAccountID || id == common.SystemAccountID {
			continue
		}

		if info.IsGenesis || runtime.ActorExists(id) {
			continue
		}

		if id.IsLogic() { // TODO: || id.IsAsset() {
			logicObject, err := transition.GetObject(id).FetchLogicObject(id)
			if err != nil {
				return errors.Wrap(err, "failed to fetch logic object")
			}

			if err = runtime.CreateLogic(
				id,
				logicObject.Artifact,
				transition.GetObject(id).GenerateLogicStorageObject(),
				nil,
			); err != nil {
				return errors.Wrap(err, "failed to create logic actor")
			}

			continue
		}

		if err := runtime.CreateActor(
			id,
			transition.GetObject(id).GenerateLogicStorageObject(),
			nil,
		); err != nil {
			return errors.Wrap(err, "failed to create actor")
		}
	}

	return nil
}

func (manager *Manager) InteractionCall(
	ctx *common.ExecutionContext,
	ix *common.Interaction,
	transition *state.Transition,
) (*common.Receipt, error) {
	// Run the interaction and return the receipt
	executor := manager.SpawnExecutor(manager.logger, manager.config, transition)

	engine, ok := engineio.FetchEngine(engineio.PISA)
	if !ok {
		return nil, errors.New("Engine not found")
	}

	runtime := engine.Runtime(ctx.Time)

	if err := AddActorsToRuntime(ix, runtime, transition); err != nil {
		return nil, err
	}

	return executor.runInteraction(
		ix,
		&engineio.RuntimeContext{
			ClusterContext: ctx,
			Runtime:        runtime,
		}, transition, false)
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
