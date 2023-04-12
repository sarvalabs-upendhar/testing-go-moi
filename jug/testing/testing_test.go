package testing

import (
	"context"
	"math/rand"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/types"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

type LogicTestSuite struct {
	suite.Suite

	logic   engineio.LogicDriver
	runtime engineio.EngineRuntime

	internal *DebugContextDriver
	snapshot *DebugContextDriver
}

func (suite *LogicTestSuite) SetupTest() {
	suite.snapshot = suite.internal.Copy()
}

func (suite *LogicTestSuite) TearDownTest() {
	suite.internal = suite.snapshot.Copy()
}

func (suite *LogicTestSuite) Initialize(
	manifest *engineio.Manifest,
	expectedLogicID types.LogicID,
	logicAddress types.Address,
) engineio.Fuel {
	runtime, _ := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	// Compile the Manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(500, manifest)
	if err != nil {
		suite.T().Fatalf("Compile Failed! Error: %v\n", err)
	}

	// Generate a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(logicAddress, descriptor)
	// Check if logic ID was generated correctly
	suite.Equal(expectedLogicID, logicObject.LogicID(), "unexpected logic id")

	// Generate a new storage object
	logicCtx := NewDebugContextDriver(logicAddress, logicObject.LogicID())

	suite.runtime = runtime
	suite.logic = logicObject
	suite.internal = logicCtx

	return consumed
}

func (suite *LogicTestSuite) Deploy(ixn *engineio.IxnObject) engineio.Fuel {
	// Create a PISA Engine for the executor
	executor, err := suite.runtime.SpawnEngine(500, suite.logic, suite.internal, nil)
	if err != nil {
		suite.T().Fatalf("Engine Bootstrap Failed! Error: %v\n", err)
	}

	// Execute the deployer function
	result := executor.Call(context.Background(), ixn, nil)
	if result.ErrCode != 0 {
		suite.T().Fatalf("Initialise Failed! Error: [%v] %v\n", result.ErrCode, result.ErrMessage)
	}

	return result.Fuel
}

func (suite *LogicTestSuite) Call(callsite string, inputs map[string]any) (engineio.Fuel, map[string]any) {
	ixn, encoder, err := suite.EncodeInputs(callsite, inputs)
	if err != nil {
		suite.T().Fatalf("Invalid Call: %v", err)
	}

	result := suite.Run(ixn)

	return suite.DecodeOutputs(result, encoder)
}

func (suite *LogicTestSuite) Run(ixn *engineio.IxnObject) *engineio.CallResult {
	// Create a PISA Engine for the executor
	executor, err := suite.runtime.SpawnEngine(500, suite.logic, suite.internal, nil)
	if err != nil {
		suite.T().Fatalf("Bootstrap Failed: %v", err)
	}

	return executor.Call(context.Background(), ixn, nil)
}

func (suite *LogicTestSuite) DecodeOutputs(result *engineio.CallResult, encoder engineio.CallEncoder) (
	engineio.Fuel, map[string]any,
) {
	// Check if the result is Ok
	if !result.Ok() {
		suite.T().Fatalf("Execute Failed | Error: [%v] %v", result.ErrCode, result.ErrMessage)
	}

	if len(result.Outputs) == 0 {
		return result.Fuel, make(map[string]any)
	}

	decoded, err := encoder.DecodeOutputs(result.Outputs)
	if err != nil {
		suite.T().Fatalf("Failed to Decode Outputs: %v", err)
	}

	return result.Fuel, decoded
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
		return engineio.NewIxnObject(types.IxLogicInvoke, callsite, make(polo.Document)), encoder, nil
	}

	calldata, err := encoder.EncodeInputs(inputs)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to encode calldata from inputs for callsite '%v'", callsite)
	}

	return engineio.NewIxnObject(types.IxLogicInvoke, callsite, calldata), encoder, nil
}

// DebugContextDriver is a debug-only implementation of CtxDriver.
// It is only meant for in-memory state management and offers no persistence
// though it can be encoded and stored at the debugging routine's discretion.
// For production workloads, generate a CtxDriver from the guna.StateObject.
type DebugContextDriver struct {
	address types.Address
	logicID types.LogicID

	spendable types.AssetMap
	approvals map[types.Address]types.AssetMap
	lockups   map[types.Address]types.AssetMap

	datastore  map[string][]byte
	logicstate map[string]map[string][]byte
}

// NewDebugContextDriver generates a new DebugContextDriver for a given Address
func NewDebugContextDriver(addr types.Address, logic types.LogicID) *DebugContextDriver {
	return &DebugContextDriver{
		address: addr,
		logicID: logic,

		spendable: make(types.AssetMap),
		approvals: make(map[types.Address]types.AssetMap),
		lockups:   make(map[types.Address]types.AssetMap),

		datastore:  make(map[string][]byte),
		logicstate: make(map[string]map[string][]byte),
	}
}

// Polorize implements the polo.Polorizable interface for DebugContextDriver
func (ctx DebugContextDriver) Polorize() (*polo.Polorizer, error) {
	// Create a new Polorizer
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(ctx.address); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.spendable); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.approvals); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.lockups); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.datastore); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.logicstate); err != nil {
		return nil, err
	}

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for DebugContextDriver
func (ctx *DebugContextDriver) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	if err = depolorizer.Depolorize(&ctx.address); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.spendable); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.approvals); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.lockups); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.datastore); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.logicstate); err != nil {
		return err
	}

	return nil
}

// Address returns the address of the DebugContextDriver account
// Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) Address() types.Address {
	return ctx.address
}

// LogicID returns the logic ID of the DebugContextDriver scope.
// Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) LogicID() types.LogicID {
	return ctx.logicID
}

// GetStorageEntry retrieves some []byte data for a given key from the DebugContextDriver.
// Returns error if there is no data for the key. Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) GetStorageEntry(key []byte) ([]byte, bool) {
	tree, ok := ctx.logicstate[string(ctx.logicID)]
	if !ok {
		return nil, false
	}

	val, ok := tree[string(key)]
	if !ok {
		return nil, false
	}

	return val, true
}

// SetStorageEntry inserts a key-value pair of data into the DebugContextDriver.
// If data already exists for the key, it is overwritten. Implements the CtxDriver interface for DebugContextDriver.
func (ctx *DebugContextDriver) SetStorageEntry(key, val []byte) bool {
	tree, ok := ctx.logicstate[string(ctx.logicID)]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[string(key)] = val
	ctx.logicstate[string(ctx.logicID)] = tree

	return true
}

// Copy generates a copy of the DebugContextDriver
func (ctx *DebugContextDriver) Copy() *DebugContextDriver {
	copied := &DebugContextDriver{
		address: ctx.address,

		spendable: ctx.spendable.Copy(),
		approvals: make(map[types.Address]types.AssetMap, len(ctx.approvals)),
		lockups:   make(map[types.Address]types.AssetMap, len(ctx.lockups)),

		datastore:  make(map[string][]byte, len(ctx.datastore)),
		logicstate: make(map[string]map[string][]byte, len(ctx.logicstate)),
	}

	copied.logicID = make(types.LogicID, len(ctx.logicID))
	copy(copied.logicID, ctx.logicID)

	for owner, assets := range ctx.approvals {
		copied.approvals[owner] = assets.Copy()
	}

	for owner, assets := range ctx.lockups {
		copied.lockups[owner] = assets.Copy()
	}

	for key, value := range ctx.datastore {
		v := make([]byte, len(value))
		copy(v, value)
		copied.datastore[key] = v
	}

	for logicID, logicState := range ctx.logicstate {
		copied.logicstate[logicID] = make(map[string][]byte, len(logicState))

		for key, value := range logicState {
			v := make([]byte, len(value))
			copy(v, value)
			copied.logicstate[logicID][key] = v
		}
	}

	return copied
}

// randomAddress generates a random types.Address.
func randomAddress() types.Address {
	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return types.BytesToAddress(address)
}
