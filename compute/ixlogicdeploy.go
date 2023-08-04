package compute

import (
	"context"
	"math/big"

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
func RunLogicDeploy(ix *common.Interaction, tank *engineio.FuelTank, objects state.ObjectMap) (*common.Receipt, error) {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return nil, errors.Wrap(err, "could not find logic payload")
	}

	if payload.Manifest == nil {
		return nil, errors.New("missing manifest for logic deploy")
	}

	// Generate the address of the target logic account
	logicAddress := common.NewAccountAddress(ix.Nonce(), ix.Sender())
	// Obtain the deployer and logic account state objects
	deployer := objects.GetObject(ix.Sender())
	logicacc := objects.GetObject(logicAddress)

	// Create an options chain
	options := make([]LogicDeployOption, 0, 3)
	// Append deploy options for deployer state and fuel limit
	options = append(options, DeployerState(deployer))
	options = append(options, DeployFuelLimit(tank.Level()))

	// If no callsite is provided, do not append an option for the deployment call
	if payload.Callsite != "" {
		options = append(options, DeploymentCall(payload.Callsite, payload.Calldata))
	}

	consumption, receiptPayload, err := DeployLogic(payload.Manifest, logicacc, options...)
	if err != nil {
		return nil, err
	}

	// Exhaust fuel from tank
	if !tank.Exhaust(consumption) {
		return nil, common.ErrInsufficientFuel
	}

	// Generate a new receipt and set the fuel consumption
	receipt := common.NewReceipt(ix)
	receipt.SetFuelUsed(tank.Consumed)

	// Set the status of the receipt
	if receiptPayload.Error != nil {
		receipt.Status = common.ReceiptFailed
	}

	// Set the extra data of the receipt
	if err = receipt.SetExtraData(receiptPayload); err != nil {
		return nil, err
	}

	return receipt, nil
}

// LogicDeployOption is an option for DeployLogic and modifies the logic deployment behaviour
type LogicDeployOption func(*logicDeployer) error

// DeployerState returns a LogicDeployOption to provide the state object of the deploying account.
func DeployerState(deployer *state.Object) LogicDeployOption {
	return func(config *logicDeployer) error {
		config.deployerState = deployer

		return nil
	}
}

// DeploymentCall returns a LogicDeployOption to provide the deployer callsite and calldata for state setup
func DeploymentCall(callsite string, calldata []byte) LogicDeployOption {
	return func(config *logicDeployer) error {
		config.deployment = &struct {
			callsite string
			calldata []byte
		}{callsite, calldata}

		return nil
	}
}

// DeployFuelLimit returns a LogicDeployOption to provide the fuel limit for logic deployment.
func DeployFuelLimit(limit engineio.Fuel) LogicDeployOption {
	return func(config *logicDeployer) error {
		config.fueltank = engineio.NewFuelTank(limit)

		return nil
	}
}

// DeployGenesisLogic deploys the manifest from the given payload into the given state object.
// The deployer call is performed with the callsite and calldata in the payload.
// Deployment occurs without a fuel limit.
func DeployGenesisLogic(state *state.Object, payload *common.LogicPayload) (common.LogicID, error) {
	_, receipt, err := DeployLogic(payload.Manifest, state, DeploymentCall(payload.Callsite, payload.Calldata))
	if err != nil {
		return "", errors.Wrap(err, "deployment failed")
	}

	if receipt.Error != nil {
		return "", errors.Errorf("deployment call failed: %#x", receipt.Error)
	}

	return receipt.LogicID, nil
}

// DeployLogic deploys the given manifest code into the given state object.
// Deployment behavior can be extended with LogicDeployOption functions to set fuel
// limit for the deployment or provide deployer state or deployment call parameters.
// Uses unlimited fuel limit unless otherwise specified with the DeployFuelLimit option.
// Does not perform a call to deployer callsite unless specified with DeploymentCall.
func DeployLogic(manifest []byte, state *state.Object, opts ...LogicDeployOption) (
	engineio.Fuel, *common.LogicDeployReceipt, error,
) {
	// Generate basic deployment config
	deployer := &logicDeployer{
		manifest:   manifest,
		logicState: state,
		fueltank: engineio.NewFuelTank(func() engineio.Fuel {
			fuel, _ := new(big.Int).SetString("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 0)

			return fuel
		}()),
	}

	// Apply all deployment options on the config
	for _, opt := range opts {
		if err := opt(deployer); err != nil {
			return nil, nil, err
		}
	}

	// Compile the manifest bytes into a LogicDescriptor
	descriptor, err := deployer.compileManifest()
	if err != nil {
		return nil, nil, err
	}

	// Generate the logic object and deploy it to state
	logicObject, err := deployer.deployLogicObject(descriptor)
	if err != nil {
		return nil, nil, err
	}

	// No deployment call defined -> return the logic ID and fuel consumption
	if deployer.deployment == nil {
		return deployer.fueltank.Consumed, &common.LogicDeployReceipt{LogicID: logicObject.LogicID()}, nil
	}

	// Call the logic deployer to set up logic state
	result, err := deployer.callDeployer(logicObject)
	if err != nil {
		return nil, nil, err
	}

	// Check the execution result
	if !result.Ok() {
		return deployer.fueltank.Consumed, &common.LogicDeployReceipt{Error: result.Error}, nil
	}

	// Return the total fuel consumed and the logic ID
	return deployer.fueltank.Consumed, &common.LogicDeployReceipt{LogicID: logicObject.LogicID()}, nil
}

type logicDeployer struct {
	manifest []byte

	fueltank      *engineio.FuelTank
	logicState    *state.Object
	deployerState *state.Object

	deployment *struct {
		callsite string
		calldata []byte
	}
}

func (deployer logicDeployer) compileManifest() (*engineio.LogicDescriptor, error) {
	// Decode the manifest data into a engineio.Manifest
	manifest, err := engineio.NewManifest(deployer.manifest, engineio.POLO)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode manifest")
	}

	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	if !ok {
		return nil, errors.Errorf("unsupported manifest engine: %v", manifest.Header().LogicEngine())
	}

	// Compile the manifest into a LogicDescriptor
	logicDescriptor, consumed, err := runtime.CompileManifest(deployer.fueltank.Level(), manifest)
	if err != nil {
		return nil, errors.Wrap(err, "manifest compile failed")
	}

	// Exhaust fuel for compile
	if !deployer.fueltank.Exhaust(consumed) {
		return nil, errors.New("insufficient fuel: could not compile manifest")
	}

	return logicDescriptor, nil
}

func (deployer logicDeployer) deployLogicObject(descriptor *engineio.LogicDescriptor) (*state.LogicObject, error) {
	// Create a logic object and attach it to the state object
	logicID, err := deployer.logicState.CreateLogic(descriptor)
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

func (deployer logicDeployer) callDeployer(logic *state.LogicObject) (*engineio.CallResult, error) {
	// Check if logic has a persistent state
	if ok := logic.StateMatrix.Persistent(); !ok {
		return nil, errors.New("cannot call deployer for logic without persistent state")
	}

	// Get runtime for logic engine
	runtime, _ := engineio.FetchEngineRuntime(logic.Engine())

	// Create a new engine for the execution
	engine, err := runtime.SpawnEngine(
		deployer.fueltank.Level(), logic, deployer.logicState.GenerateLogicContextObject(logic.LogicID()), envObject{},
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not bootstrap engine")
	}

	// Create an IxnObject
	// todo: we should accept the raw interaction somehow
	ixn := ixnObject{
		kind:     common.IxLogicDeploy,
		price:    big.NewInt(1),
		limit:    deployer.fueltank.Capacity,
		callsite: deployer.deployment.callsite,
		calldata: deployer.deployment.calldata,
	}

	// Declare context driver
	var deployerCtx engineio.CtxDriver
	// Create the deployer context driver if not nil
	if deployer.deployerState != nil {
		deployerCtx = deployer.deployerState.GenerateLogicContextObject(logic.LogicID())
	}

	// Perform execution call on the engine
	result, err := engine.Call(context.Background(), ixn, deployerCtx)
	if err != nil {
		return nil, errors.Wrap(err, "could not perform call")
	}

	// Exhaust fuel for deployer call
	if !deployer.fueltank.Exhaust(result.Consumed) {
		return nil, errors.New("insufficient fuel: could not call logic deployer")
	}

	return result, nil
}
