package compute

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"

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
	ctx *engineio.RuntimeContext,
	tank *FuelTank,
	transition *state.Transition,
) *common.IxOpResult {
	// Create a new op result
	opResult := common.NewIxOpResult(op.Type())

	// Obtain the deployer and logic account state objects
	deployer, _ := transition.GetObject(op.SenderID())
	logicacc, _ := transition.GetObject(op.Target())

	// Create an event stream to emit the events on
	consumption, receiptPayload, logs, err := DeployLogic(ctx, op, op.Manifest(), logicacc, deployer, transition, tank)

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption.Compute, consumption.Storage) {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	if err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	// Set the logs in the receipt
	opResult.SetLogs(logs)

	// Set the result payload
	if receiptPayload != nil {
		common.SetResultPayload(opResult, *receiptPayload)

		// Set the status of the receipt based on the error stat
		if receiptPayload.Error != nil {
			return opResult.WithStatus(common.ResultExceptionRaised)
		}
	}

	if err = addNewAccountsToSargaAccount(transition, op.Interaction.Hash(), op.Target()); err != nil {
		return opResult.WithStatus(common.ResultExceptionRaised)
	}

	return opResult.WithStatus(common.ResultOk)
}

// DeployLogic deploys the given manifest code into the given state object.
// Deployment behavior can be extended with LogicDeployOption functions to set fuel
// limit for the deployment or provide deployer state or deployment call parameters.
// Uses unlimited fuel limit unless otherwise specified with the DeployFuelLimit option.
// Does not perform a call to deployer callsite unless specified with DeploymentCall.
func DeployLogic(
	ctx *engineio.RuntimeContext,
	op *common.IxOp,
	manifest []byte,
	logicState *state.Object,
	deployerState *state.Object,
	transition *state.Transition,
	fueltank *FuelTank,
) (
	*engineio.FuelGauge, *common.LogicDeployResult, []common.Log, error,
) {
	// Generate basic deployment config
	deployer := &logicDeployer{
		manifest:      manifest,
		logicState:    logicState,
		deployerState: deployerState,
		fuel:          NewFuelTank(fueltank.Level()),
	}

	// Compile the manifest bytes into a LogicDescriptor
	descriptor, err := deployer.compileManifest()
	if err != nil {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()), nil, nil, err
	}

	// Generate the logic object and deploy it to state
	logicObject, err := deployer.deployLogicObject(logicState.Identifier(), descriptor)
	if err != nil {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()), nil, nil, err
	}

	// If no callsite is provided -> return the logic ID and fuel consumption
	if op.Callsite() == "" {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()),
			&common.LogicDeployResult{LogicID: logicObject.ID},
			nil, nil
	}

	if err = ctx.Runtime.SpawnLogic(
		logicObject.LogicID(),
		descriptor.Artifact,
		logicState.FetchLogicStorageObject(),
		nil,
	); err != nil {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()),
			nil, nil,
			errors.Wrap(err, "failed to create logic in runtime")
	}

	// Call the logic deployer to set up logic state
	result := ctx.Runtime.Call(logicObject.LogicID(), op, transition, engineio.NewFuelGauge(deployer.fuel.Level()))

	if !deployer.fuel.Exhaust(result.ComputeEffort, result.StorageEffort) {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()), nil, nil, errors.New("insufficient fuel")
	}

	// Check the execution result
	if result.IsError() {
		return engineio.NewFuelGauge(deployer.fuel.Consumed()), &common.LogicDeployResult{Error: result.Err}, result.Logs, nil
	}

	fmt.Println("logic successfully invoked", logicState.Identifier())

	// Return the total fuel consumed and the logic ID
	return engineio.NewFuelGauge(deployer.fuel.Consumed()),
		&common.LogicDeployResult{LogicID: logicObject.ID},
		result.Logs,
		nil
}

type logicDeployer struct {
	manifest      []byte
	fuel          *FuelTank
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
	engine, ok := engineio.FetchEngine(manifest.Engine().Kind)
	if !ok {
		return nil, errors.Errorf("unsupported manifest engine: %v", manifest.Engine().Kind)
	}

	// Check that the Manifest Instance is PISA
	if manifest.Engine().Kind != engineio.PISA {
		return nil, errors.New("invalid manifest: manifest engine is not PISA")
	}

	rawArtifact, consumed, err := engine.CompileManifest(
		engineio.ManifestKindFromIdentifier(deployer.logicState.Identifier()),
		deployer.logicState.Identifier(),
		manifest,
		*engineio.NewFuelGauge(deployer.fuel.Level()))

	if !deployer.fuel.Exhaust(consumed.Compute, consumed.Storage) {
		return nil, errors.New("insufficient fuel: could not compile manifest")
	}

	if err != nil {
		return nil, errors.Wrap(err, "could not compile manifest")
	}

	// Create a new manifest compiler
	return &engineio.LogicDescriptor{
		Engine:       manifest.Engine().Kind,
		Artifact:     rawArtifact,
		ManifestHash: manifest.Hash(), // TODO: This is expensive, optimize this.
		ManifestData: deployer.manifest,
	}, nil
}

func (deployer logicDeployer) deployLogicObject(
	logicID identifiers.Identifier,
	descriptor *engineio.LogicDescriptor,
) (*state.LogicObject, error) {
	// Create a logic object and attach it to the state object
	err := deployer.logicState.CreateLogic(logicID, *descriptor)
	if err != nil {
		return nil, err
	}

	// Fetch the logic object from the state object
	logicObject, err := deployer.logicState.FetchLogicObject(logicID)
	if err != nil {
		return nil, err
	}

	// Exhaust fuel for state deploy
	if !deployer.fuel.Exhaust(FuelLogicObjectDeployment, 0) {
		return nil, errors.New("insufficient fuel: could not deploy logic object to state")
	}

	return logicObject, nil
}
