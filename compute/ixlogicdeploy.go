package compute

import (
	"context"
	"math"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicDeploy performs the given IxLogicDeploy Operation.
// The stateObjectRetriever must contain state objects for the sender and Target of the op.
//
// The IxOp must have a LogicPayload with a Manifest and the output receipt will have a LogicDeployResult.
// The logic manifest is verified, compiled and deployed on to a new account and any deployer call is executed.
func RunLogicDeploy(
	op *common.IxOp,
	ctx *common.ExecutionContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	status := common.ResultOk

	// Create a new op result
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the deployer and logic account state objects
	deployer := transition.GetObject(op.SenderID())
	logicacc := transition.GetObject(op.Target())

	// Create an options chain
	options := make([]LogicDeployOption, 0, 3)
	// Append deploy options for deployer state and fuel limit
	options = append(options, DeployFuelLimit(tank.Level()))

	// Create an event stream to emit the events on
	eventstream := NewEventStream(op.LogicID())

	consumption, receiptPayload, err := DeployLogic(ctx, op, logicacc, deployer, eventstream, options...)
	if err != nil {
		status = common.ResultStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		status = common.ResultFuelExhausted
	}

	// Set the result payload
	if receiptPayload != nil {
		common.SetResultPayload(opResult, *receiptPayload)

		// Set the status of the receipt based on the error stat
		if receiptPayload.Error != nil {
			status = common.ResultExceptionRaised
		}
	}

	if err = addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		status = common.ResultStateReverted
	}

	// Set the logs in the receipt
	opResult.SetLogs(eventstream.Collect())
	opResult.SetStatus(status)

	return opResult
}

// LogicDeployOption is an option for DeployLogic and modifies the logic deployment behaviour
type LogicDeployOption func(*logicDeployer) error

// DeployFuelLimit returns a LogicDeployOption to provide the fuel limit for logic deployment.
func DeployFuelLimit(limit uint64) LogicDeployOption {
	return func(config *logicDeployer) error {
		config.fueltank = NewFuelTank(limit)

		return nil
	}
}

// DeployLogic deploys the given manifest code into the given state object.
// Deployment behavior can be extended with LogicDeployOption functions to set fuel
// limit for the deployment or provide deployer state or deployment call parameters.
// Uses unlimited fuel limit unless otherwise specified with the DeployFuelLimit option.
// Does not perform a call to deployer callsite unless specified with DeploymentCall.
func DeployLogic(
	ctx *common.ExecutionContext,
	op *common.IxOp,
	logicState *state.Object,
	deployerState *state.Object,
	eventstream *EventStream,
	opts ...LogicDeployOption,
) (
	uint64, *common.LogicDeployResult, error,
) {
	// Generate basic deployment config
	deployer := &logicDeployer{
		manifest:      op.Manifest(),
		logicState:    logicState,
		deployerState: deployerState,
		fueltank:      NewFuelTank(math.MaxUint64),
	}

	// Apply all deployment options on the config
	for _, opt := range opts {
		if err := opt(deployer); err != nil {
			return 0, nil, err
		}
	}

	// Compile the manifest bytes into a LogicDescriptor
	descriptor, err := deployer.compileManifest()
	if err != nil {
		return 0, nil, err
	}

	// Generate the logic object and deploy it to state
	logicObject, err := deployer.deployLogicObject(descriptor)
	if err != nil {
		return 0, nil, err
	}

	// If no callsite is provided -> return the logic ID and fuel consumption
	if op.Callsite() == "" {
		return deployer.fueltank.Consumed, &common.LogicDeployResult{LogicID: logicObject.ID}, nil
	}

	// Call the logic deployer to set up logic state
	result, err := deployer.callDeployer(op, ctx, logicObject, eventstream)
	if err != nil {
		return 0, nil, err
	}

	// Check the execution result
	if !result.Ok() {
		return deployer.fueltank.Consumed, &common.LogicDeployResult{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the logic ID
	return deployer.fueltank.Consumed, &common.LogicDeployResult{LogicID: logicObject.ID}, nil
}

type logicDeployer struct {
	manifest []byte

	fueltank      *FuelTank
	logicState    *state.Object
	deployerState *state.Object
}

func (deployer logicDeployer) compileManifest() (*engineio.LogicDescriptor, error) {
	// Decode the manifest data into a engineio.Manifest
	manifest, err := engineio.NewManifest(deployer.manifest, common.POLO)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode manifest")
	}

	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngine(manifest.Engine().Kind)
	if !ok {
		return nil, errors.Errorf("unsupported manifest engine: %v", manifest.Engine().Kind)
	}

	// Compile the manifest into a LogicDescriptor
	logicDescriptor, consumed, err := runtime.CompileManifest(manifest, deployer.fueltank.Level())
	if err != nil {
		return nil, errors.Wrap(err, "manifest compile failed")
	}

	// Exhaust fuel for compile
	if !deployer.fueltank.Exhaust(consumed) {
		return nil, errors.New("insufficient fuel: could not compile manifest")
	}

	return &logicDescriptor, nil
}

func (deployer logicDeployer) deployLogicObject(descriptor *engineio.LogicDescriptor) (*state.LogicObject, error) {
	// Create a logic object and attach it to the state object
	logicID, err := deployer.logicState.CreateLogic(*descriptor)
	if err != nil {
		return nil, err
	}

	// Fetch the logic object from the state object
	logicObject, err := deployer.logicState.FetchLogicObject(logicID)
	if err != nil {
		return nil, err
	}

	// Exhaust fuel for state deploy
	if !deployer.fueltank.Exhaust(FuelLogicObjectDeployment) {
		return nil, errors.New("insufficient fuel: could not deploy logic object to state")
	}

	return logicObject, nil
}

func (deployer logicDeployer) callDeployer(
	op *common.IxOp,
	ctx *common.ExecutionContext,
	logic *state.LogicObject,
	eventstream *EventStream,
) (engineio.CallResult, error) {
	// Check if logic has a persistent state
	if _, ok := logic.PersistentState(); !ok {
		return nil, errors.New("cannot call deployer for logic without persistent state")
	}

	// Get runtime for logic engine
	engine, _ := engineio.FetchEngine(logic.Engine())
	// Create a new engine for the execution
	instance, err := engine.SpawnInstance(
		logic,
		deployer.fueltank.Level(),
		deployer.logicState.GenerateLogicStorageObject(logic.ID),
		ctx,
		eventstream,
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Declare context driver
	var deployerCtx engineio.StateDriver
	// Create the deployer context driver if not nil
	if deployer.deployerState != nil {
		// If the logic describes an ephemeral state
		if _, ok := logic.EphemeralState(); ok {
			// We need to initialise the logic tree for the deployer as well
			if err = deployer.deployerState.InitLogicStorage(logic.LogicID()); err != nil {
				return nil, err
			}
		}

		deployerCtx = deployer.deployerState.GenerateLogicStorageObject(logic.ID)
	}

	// Perform execution call on the engine
	result, err := instance.Call(context.Background(), op, deployerCtx)
	if err != nil {
		return nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !deployer.fueltank.Exhaust(result.Fuel()) {
		return nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	return result, nil
}

func (manager *Manager) ValidateLogicDeploy(op *common.IxOp) error {
	// Check that the manifest decodes correctly
	manifest, err := engineio.NewManifest(op.Manifest(), common.POLO)
	if err != nil {
		return err
	}

	runtime, ok := engineio.FetchEngine(manifest.Kind())
	if !ok {
		return errors.New("failed to get runtime for logic")
	}

	// Attempt to compile the manifest into logic descriptor with fuel limit
	descriptor, _, err := runtime.CompileManifest(manifest, op.FuelLimit())
	if err != nil {
		return err
	}

	if op.Callsite() == "" {
		// If no callsite is provided, skip calldata validation
		return nil
	}

	// Create a logic object from the descriptor
	logic := state.NewLogicObject(op.Target(), descriptor)
	// TODO: this logic object is wasted after creation, consider caching somewhere?

	return runtime.ValidateCalldata(logic, op)
}
