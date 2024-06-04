package compute

import (
	"context"
	"math"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

// RunLogicDeploy performs the given IxLogicDeploy interaction.
// The stateObjectRetriever must contain state objects for the sender and receiver of the Interaction.
//
// The Interaction must have a LogicPayload with a Manifest and the output receipt will have a LogicDeployReceipt.
// The logic manifest is verified, compiled and deployed on to a new account and any deployer call is executed.
func RunLogicDeploy(
	ix *common.Interaction,
	ctx *common.ExecutionContext,
	tank *FuelTank,
	objects *state.Transition,
) *common.Receipt {
	// Generate a new receipt
	receipt := common.NewReceipt(ix)

	// Generate the address of the target logic account
	logicAddress := common.NewAccountAddress(ix.Nonce(), ix.Sender())
	// Obtain the deployer and logic account state objects
	deployer := objects.GetObject(ix.Sender())
	logicacc := objects.GetObject(logicAddress)

	// Create an options chain
	options := make([]LogicDeployOption, 0, 3)
	// Append deploy options for deployer state and fuel limit
	options = append(options, DeployFuelLimit(tank.Level()))

	// Create an event stream to emit the events on
	eventstream := NewEventStream(ix.LogicID())

	consumption, receiptPayload, err := DeployLogic(ctx, ix, logicacc, deployer, eventstream, options...)
	if err != nil {
		receipt.Status = common.ReceiptStateReverted
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		receipt.Status = common.ReceiptFuelExhausted
	}

	// Set the fuel consumption
	receipt.SetFuelUsed(tank.Consumed)

	// Set the extra data of the receipt
	if receiptPayload != nil {
		common.SetReceiptExtraData(receipt, *receiptPayload)

		// Set the status of the receipt based on the error stat
		if receiptPayload.Error != nil {
			receipt.Status = common.ReceiptExceptionRaised
		}
	}

	// Set the logs in the receipt
	receipt.SetLogs(eventstream.GetAsLogs())

	return receipt
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
	ix *common.Interaction,
	logicState *state.Object,
	deployerState *state.Object,
	eventstream *EventStream,
	opts ...LogicDeployOption,
) (
	uint64, *common.LogicDeployReceipt, error,
) {
	// Generate basic deployment config
	deployer := &logicDeployer{
		manifest:      ix.Manifest(),
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
	if ix.Callsite() == "" {
		return deployer.fueltank.Consumed, &common.LogicDeployReceipt{LogicID: logicObject.ID}, nil
	}

	// Call the logic deployer to set up logic state
	result, err := deployer.callDeployer(ix, ctx, logicObject, eventstream)
	if err != nil {
		return 0, nil, err
	}

	// Check the execution result
	if !result.Ok() {
		return deployer.fueltank.Consumed, &common.LogicDeployReceipt{Error: result.Error()}, nil
	}

	// Return the total fuel consumed and the logic ID
	return deployer.fueltank.Consumed, &common.LogicDeployReceipt{LogicID: logicObject.ID}, nil
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
	ixn *common.Interaction,
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
		deployer.logicState.GenerateLogicContextObject(logic.ID),
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
		deployerCtx = deployer.deployerState.GenerateLogicContextObject(logic.ID)
	}

	// Perform execution call on the engine
	result, err := instance.Call(context.Background(), ixn, deployerCtx)
	if err != nil {
		return nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !deployer.fueltank.Exhaust(result.Fuel()) {
		return nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	return result, nil
}
