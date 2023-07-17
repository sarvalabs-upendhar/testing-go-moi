package manifests

import (
	"context"
	"math/big"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	gtypes "github.com/sarvalabs/go-moi/state"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

type LogicTestSuite struct {
	suite.Suite

	fuel        engineio.Fuel
	logic       engineio.LogicDriver
	runtime     engineio.EngineRuntime
	environment engineio.EnvDriver

	internal         *engineio.DebugContextDriver
	internalSnapshot *engineio.DebugContextDriver

	sender         *engineio.DebugContextDriver
	senderSnapshot *engineio.DebugContextDriver
}

func (suite *LogicTestSuite) SetupTest() {
	suite.internalSnapshot = suite.internal.Copy()
	suite.senderSnapshot = suite.sender.Copy()
}

func (suite *LogicTestSuite) TearDownTest() {
	suite.internal = suite.internalSnapshot.Copy()
	suite.sender = suite.senderSnapshot.Copy()
}

func (suite *LogicTestSuite) Initialize(
	manifest *engineio.Manifest,
	expectedLogicID common.LogicID,
	logicAddress common.Address,
	fuel engineio.Fuel,
	sender common.Address,
) engineio.Fuel {
	runtime, _ := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	// Compile the Manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(fuel, manifest)
	if err != nil {
		suite.T().Fatalf("Compile Failed! Error: %v\n", err)
	}

	// Generate a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(logicAddress, descriptor)
	// Check if logic ID was generated correctly
	suite.Equal(expectedLogicID, logicObject.LogicID(), "unexpected logic id")

	// Generate a new storage object
	logicCtx := engineio.NewDebugContextDriver(logicAddress, logicObject.LogicID())

	senderCtx := engineio.NewDebugContextDriver(sender, logicObject.LogicID())
	if common.Address.IsNil(sender) {
		senderCtx = engineio.NewDebugContextDriver(randomAddress(), logicObject.LogicID())
	}

	suite.fuel = fuel
	suite.runtime = runtime
	suite.logic = logicObject

	suite.internal = logicCtx
	suite.sender = senderCtx
	suite.environment = engineio.NewEnvObject(time.Now().Unix(), big.NewInt(1))

	return consumed
}

func (suite *LogicTestSuite) CallRaw(kind engineio.CallsiteKind, callsite string, calldata []byte) (engineio.Fuel, []byte, []byte) { //nolint:lll
	ixn := engineio.NewIxnObject(kind.IxnType(), callsite, calldata)

	result, err := suite.Run(ixn)
	if err != nil {
		suite.T().Fatalf("Call Failed! Error: %v\n", err)
	}

	if !result.Ok() {
		return result.Consumed, nil, result.Error
	}

	return result.Consumed, result.Outputs, nil
}

func (suite *LogicTestSuite) Call(callsite string, inputs map[string]any) (engineio.Fuel, map[string]any, []byte) {
	ixn, encoder, err := suite.EncodeInputs(callsite, inputs)
	if err != nil {
		suite.T().Fatalf("Invalid Call: %v", err)
	}

	result, err := suite.Run(ixn)
	if err != nil {
		suite.T().Fatalf("Call Failed! Error: %v\n", err)
	}

	return suite.DecodeOutputs(result, encoder)
}

func (suite *LogicTestSuite) Run(ixn *engineio.IxnObject) (*engineio.CallResult, error) {
	// Create a PISA Engine for the executor
	executor, err := suite.runtime.SpawnEngine(suite.fuel, suite.logic, suite.internal, suite.environment)
	if err != nil {
		suite.T().Fatalf("Bootstrap Failed: %v", err)
	}

	return executor.Call(context.Background(), ixn, suite.sender)
}

func (suite *LogicTestSuite) DecodeOutputs(result *engineio.CallResult, encoder engineio.CallEncoder) (
	engineio.Fuel, map[string]any, []byte,
) {
	// Check if the result is Ok
	if !result.Ok() {
		return result.Consumed, nil, result.Error
	}

	if len(result.Outputs) == 0 {
		return result.Consumed, make(map[string]any), nil
	}

	decoded, err := encoder.DecodeOutputs(result.Outputs)
	if err != nil {
		suite.T().Fatalf("Failed to Decode Outputs: %v", err)
	}

	return result.Consumed, decoded, nil
}

func (suite *LogicTestSuite) EncodeInputs(callsite string, inputs map[string]any) (
	*engineio.IxnObject, engineio.CallEncoder, error,
) {
	site, ok := suite.logic.GetCallsite(callsite)
	if !ok {
		return nil, nil, errors.Errorf("callsite '%v' does not exist", callsite)
	}

	encoder, err := suite.runtime.GetCallEncoder(site, suite.logic)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate calldata encoder for callsite '%v'", callsite)
	}

	if len(inputs) == 0 {
		return engineio.NewIxnObject(common.IxLogicInvoke, callsite, nil), encoder, nil
	}

	calldata, err := encoder.EncodeInputs(inputs, nil)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to encode calldata from inputs for callsite '%v'", callsite)
	}

	return engineio.NewIxnObject(site.Kind.IxnType(), callsite, calldata), encoder, nil
}

// randomAddress generates a random types.Address.
func randomAddress() common.Address {
	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return common.BytesToAddress(address)
}

func must[T any](object T, err error) T {
	if err != nil {
		panic(err)
	}

	return object
}
