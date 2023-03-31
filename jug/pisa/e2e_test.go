package pisa

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

const (
	addressERC20 = "21544c5299db3231e6b0c0a98d5b880debd8af2c7eda2eae3cc6de7572418b60"
	logicidERC20 = "08000021544c5299db3231e6b0c0a98d5b880debd8af2c7eda2eae3cc6de7572418b60"

	erc20Addr1 = "ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34"
	erc20Addr2 = "0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"
	erc20Addr3 = "916f62f7bdf311ba46d9df465283aa4e97a5dcf36e4c0761bc7d87d32557f8cc"
	erc20Addr4 = "39c1124554d32aa82c4388105c89b3805cbb2c461f4615de0a0e16457d4c7cac"
	erc20Addr5 = "62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa"
	erc20Addr6 = "fafe52ec42a85db6441eaf1c438daa4e97a5dcf36e4c0761bc7d87d32557f8cc"
)

func init() {
	engineio.RegisterEngineRuntime(NewRuntime())
}

type ERC20TestSuite struct {
	suite.Suite

	logic    engineio.LogicDriver
	manifest *engineio.Manifest
	runtime  engineio.EngineRuntime

	internal *DebugContextDriver
	snapshot *DebugContextDriver
}

func TestERC20TestSuite(t *testing.T) {
	suite.Run(t, new(ERC20TestSuite))
}

func (suite *ERC20TestSuite) SetupSuite() {
	// Read manifest file (JSON)
	data, err := ioutil.ReadFile("./../manifests/erc20.json")
	if err != nil {
		suite.T().Fatalf("manifest hex decode: %v", err)
	}

	// Decode manifest file into Manifest object
	manifest, err := engineio.NewManifest(data, engineio.JSON)
	if err != nil {
		suite.T().Fatalf("manifest decode: %v", err)
	}

	runtime, _ := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	// Compile the Manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(500, manifest)
	if err != nil {
		suite.T().Fatalf("Compile Failed! Error: %v\n", err)
	}

	suite.Equal(engineio.Fuel(100), consumed)

	// Generate a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(types.HexToAddress(addressERC20), descriptor)
	// Check if logic ID was generated correctly
	suite.Equal(logicidERC20, logicObject.ID.Hex(), "unexpected logic id")

	// Generate a new storage object
	logicCtx := NewDebugContextDriver(types.HexToAddress(addressERC20), logicObject.LogicID())

	// Create a PISA Engine for the executor
	executor, err := runtime.SpawnEngine(500, logicObject, logicCtx, nil)
	if err != nil {
		suite.T().Fatalf("Engine Bootstrap Failed! Error: %v\n", err)
	}

	// Generate the init data and document encode it
	inputs, _ := polo.DocumentEncode(struct {
		Name   string        `polo:"name"`
		Symbol string        `polo:"symbol"`
		Supply uint64        `polo:"supply"`
		Seeder types.Address `polo:"seeder"`
	}{
		"MOI-Token", "MOI", 100000000, types.HexToAddress(erc20Addr1),
	})

	ixn := engineio.NewIxnObject(types.IxLogicDeploy, "Seeder!", inputs)
	// Execute the deployer function
	result := executor.Call(context.Background(), ixn, nil)
	if result.ErrCode != 0 {
		suite.T().Fatalf("Initialise Failed! Error: [%v] %v\n", result.ErrCode, result.ErrMessage)
	}

	suite.Equal(engineio.Fuel(150), result.Fuel)

	suite.logic = logicObject
	suite.internal = logicCtx
	suite.runtime = runtime
	suite.manifest = manifest
}

func (suite *ERC20TestSuite) SetupTest() {
	suite.snapshot = suite.internal.Copy()
}

func (suite *ERC20TestSuite) TearDownTest() {
	suite.internal = suite.snapshot.Copy()
}

func (suite *ERC20TestSuite) TestReadMethods() {
	consumed, outputs := suite.Call("Name", nil)
	suite.Equal("MOI-Token", outputs["name"])
	suite.Equal(engineio.Fuel(70), consumed)

	consumed, outputs = suite.Call("Symbol", nil)
	suite.Equal("MOI", outputs["symbol"])
	suite.Equal(engineio.Fuel(70), consumed)

	consumed, outputs = suite.Call("Decimals", nil)
	suite.Equal(uint64(10), outputs["decimals"])
	suite.Equal(engineio.Fuel(80), consumed)

	consumed, outputs = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), outputs["supply"])
	suite.Equal(engineio.Fuel(70), consumed)
}

func (suite *ERC20TestSuite) TestApproval() {
	// Approve Addr2 to spend 500 tokens of Addr1
	consumed, output := suite.Call("Approve!", map[string]any{
		"owner":   types.HexToAddress(erc20Addr1),
		"spender": types.HexToAddress(erc20Addr2),
		"amount":  500,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(210), consumed)

	// Check allowance of Addr2 on Addr1 tokens (must be 500)
	consumed, output = suite.Call("Allowance", map[string]any{
		"owner":   types.HexToAddress(erc20Addr1),
		"spender": types.HexToAddress(erc20Addr2),
	})
	suite.Equal(uint64(500), output["allowance"])
	suite.Equal(engineio.Fuel(110), consumed)

	// Check allowance of Addr3 on Addr2 tokens (must be 0)
	consumed, output = suite.Call("Allowance", map[string]any{
		"owner":   types.HexToAddress(erc20Addr2),
		"spender": types.HexToAddress(erc20Addr3),
	})
	suite.Equal(uint64(0), output["allowance"])
	suite.Equal(engineio.Fuel(110), consumed)
}

func (suite *ERC20TestSuite) TestTransfer() {
	// Check balance of Addr1 (must be initial seed amount of 100000000)
	consumed, output := suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(100000000), output["balance"])
	suite.Equal(engineio.Fuel(90), consumed)

	// Transfer 1000 tokens from Addr1 to Addr4
	consumed, output = suite.Call("Transfer!", map[string]any{
		"from":   types.HexToAddress(erc20Addr1),
		"to":     types.HexToAddress(erc20Addr4),
		"amount": 1000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(230), consumed)

	// Check balance of Addr1 (must be 100000000 - 1000)
	consumed, output = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(99999000), output["balance"])
	suite.Equal(engineio.Fuel(90), consumed)

	// Check balance of Addr4 (must be 1000)
	consumed, output = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr4)})
	suite.Equal(uint64(1000), output["balance"])
	suite.Equal(engineio.Fuel(90), consumed)

	// Transfer 10000 tokens from Addr4 to Addr5 (must fail due to insufficient balance)
	consumed, output = suite.Call("Transfer!", map[string]any{
		"from":   types.HexToAddress(erc20Addr4),
		"to":     types.HexToAddress(erc20Addr5),
		"amount": 10000,
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.Fuel(150), consumed)
}

func (suite *ERC20TestSuite) TestInflation() {
	// Check total supply of token (must be initial seed amount of 100000000)
	consumed, output := suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), output["supply"])
	suite.Equal(engineio.Fuel(70), consumed)

	// Burn 10000 tokens from Addr1
	consumed, output = suite.Call("Burn!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr1),
		"amount": 10000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(220), consumed)

	// Check balance of Addr1 (must be 99990000)
	consumed, output = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(99990000), output["balance"])
	suite.Equal(engineio.Fuel(90), consumed)

	// Check total supply of token (must be 99990000)
	consumed, output = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(99990000), output["supply"])
	suite.Equal(engineio.Fuel(70), consumed)

	// Mint 10000 tokens to Addr6
	consumed, output = suite.Call("Mint!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr6),
		"amount": 10000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(180), consumed)

	// Check balance of Addr6 (must be 10000)
	consumed, output = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr6)})
	suite.Equal(uint64(10000), output["balance"])
	suite.Equal(engineio.Fuel(90), consumed)

	// Check total supply of token (must be 100000000)
	consumed, output = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), output["supply"])
	suite.Equal(engineio.Fuel(70), consumed)

	// Burn 1000000 tokens from Addr6 (must fail due to insufficient balance)
	consumed, output = suite.Call("Burn!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr6),
		"amount": 100000,
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.Fuel(150), consumed)
}

func (suite *ERC20TestSuite) Call(callsite string, inputs map[string]any) (engineio.Fuel, map[string]any) {
	ixn, encoder, err := suite.EncodeInputs(callsite, inputs)
	if err != nil {
		suite.T().Fatalf("Invalid Call: %v", err)
	}

	result := suite.Run(ixn)

	return suite.DecodeOutputs(result, encoder)
}

func (suite *ERC20TestSuite) Run(ixn *engineio.IxnObject) *engineio.CallResult {
	// Create a PISA Engine for the executor
	executor, err := suite.runtime.SpawnEngine(500, suite.logic, suite.internal, nil)
	if err != nil {
		suite.T().Fatalf("Bootstrap Failed: %v", err)
	}

	return executor.Call(context.Background(), ixn, nil)
}

func (suite *ERC20TestSuite) DecodeOutputs(result *engineio.CallResult, encoder engineio.CallEncoder) (
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

func (suite *ERC20TestSuite) EncodeInputs(callsite string, inputs map[string]any) (
	*engineio.IxnObject, engineio.CallEncoder, error,
) {
	site, ok := suite.logic.GetCallsite(callsite)
	if !ok {
		return nil, nil, errors.Errorf("callsite '%v' does not exist", callsite)
	}

	encoder, err := suite.runtime.GetCallEncoderFromManifest(site, suite.manifest)
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
		logicID: ctx.logicID,

		spendable: ctx.spendable.Copy(),
		approvals: make(map[types.Address]types.AssetMap, len(ctx.approvals)),
		lockups:   make(map[types.Address]types.AssetMap, len(ctx.lockups)),

		datastore:  make(map[string][]byte, len(ctx.datastore)),
		logicstate: make(map[string]map[string][]byte, len(ctx.logicstate)),
	}

	for owner, assets := range ctx.approvals {
		copied.approvals[owner] = assets.Copy()
	}

	for owner, assets := range ctx.lockups {
		copied.lockups[owner] = assets.Copy()
	}

	for key, val := range ctx.datastore {
		copied.datastore[key] = val
	}

	for logicID, logicState := range ctx.logicstate {
		copied.logicstate[logicID] = make(map[string][]byte, len(logicState))
		for key, val := range logicState {
			copied.logicstate[logicID][key] = val
		}
	}

	return copied
}
