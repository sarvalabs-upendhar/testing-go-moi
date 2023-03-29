package pisa

import (
	"context"
	"io/ioutil"
	"testing"

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
	engineio.RegisterElementRegistry(engineio.PISA, ElementRegistry())
}

type ERC20TestSuite struct {
	suite.Suite

	factory Factory
	logic   engineio.LogicDriver

	internal *engineio.DebugContextDriver
	snapshot *engineio.DebugContextDriver
}

func TestERC20TestSuite(t *testing.T) {
	suite.Run(t, new(ERC20TestSuite))
}

func (suite *ERC20TestSuite) SetupSuite() {
	// Setup factory and fuel tank
	suite.factory = NewFactory()

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

	// Create a PISA Engine for the compiler
	compiler := suite.factory.NewEngine()
	// Compile the Manifest into a LogicDescriptor
	descriptor, consumed, err := compiler.Compile(context.Background(), 500, manifest)
	if err != nil {
		suite.T().Fatalf("Compile Failed! Error: %v\n", err)
	}

	suite.Equal(engineio.Fuel(100), consumed)

	// Generate a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(types.HexToAddress(addressERC20), descriptor)
	// Check if logic ID was generated correctly
	suite.Equal(logicidERC20, logicObject.ID.Hex(), "unexpected logic id")

	// Generate a new storage object
	logicCtx := engineio.NewDebugContextDriver(types.HexToAddress(addressERC20), logicObject.LogicID())

	// Create a PISA Engine for the executor
	executor := suite.factory.NewEngine()
	if err = executor.Bootstrap(context.Background(), 500, logicObject, logicCtx, nil); err != nil {
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
	result := executor.Call(context.Background(), engineio.DeployerCallsite, ixn, nil)
	if result.ErrCode != 0 {
		suite.T().Fatalf("Initialise Failed! Error: [%v] %v\n", result.ErrCode, result.ErrMessage)
	}

	suite.Equal(engineio.Fuel(150), result.Fuel)

	suite.logic = logicObject
	suite.internal = logicCtx
}

func (suite *ERC20TestSuite) SetupTest() {
	suite.snapshot = suite.internal.Copy()
}

func (suite *ERC20TestSuite) TearDownTest() {
	suite.internal = suite.snapshot.Copy()
}

func (suite *ERC20TestSuite) TestReadMethods() {
	consumed, name := suite.callmethodName()
	suite.Equal("MOI-Token", name)
	suite.Equal(engineio.Fuel(70), consumed)

	consumed, symbol := suite.callmethodSymbol()
	suite.Equal("MOI", symbol)
	suite.Equal(engineio.Fuel(70), consumed)

	consumed, decimals := suite.callmethodDecimals()
	suite.Equal(uint64(10), decimals)
	suite.Equal(engineio.Fuel(80), consumed)

	consumed, supply := suite.callmethodTotalSupply()
	suite.Equal(uint64(100000000), supply)
	suite.Equal(engineio.Fuel(70), consumed)
}

func (suite *ERC20TestSuite) TestApproval() {
	// Approve Addr2 to spend 500 tokens of Addr1
	consumed, ok := suite.callmethodApprove(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr2), 500)
	suite.Equal(true, ok)
	suite.Equal(engineio.Fuel(210), consumed)

	// Check allowance of Addr2 on Addr1 tokens (must be 500)
	consumed, allowance := suite.callmethodAllowance(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr2))
	suite.Equal(uint64(500), allowance)
	suite.Equal(engineio.Fuel(110), consumed)

	// Check allowance of Addr3 on Addr2 tokens (must be 0)
	consumed, allowance = suite.callmethodAllowance(types.HexToAddress(erc20Addr2), types.HexToAddress(erc20Addr3))
	suite.Equal(uint64(0), allowance)
	suite.Equal(engineio.Fuel(110), consumed)
}

func (suite *ERC20TestSuite) TestTransfer() {
	// Check balance of Addr1 (must be initial seed amount of 100000000)
	consumed, balance := suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(100000000), balance)
	suite.Equal(engineio.Fuel(90), consumed)

	// Transfer 1000 tokens from Addr1 to Addr4
	consumed, ok := suite.callmethodTransfer(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr4), 1000)
	suite.Equal(true, ok)
	suite.Equal(engineio.Fuel(230), consumed)

	// Check balance of Addr1 (must be 100000000 - 1000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(99999000), balance)
	suite.Equal(engineio.Fuel(90), consumed)

	// Check balance of Addr4 (must be 1000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr4))
	suite.Equal(uint64(1000), balance)
	suite.Equal(engineio.Fuel(90), consumed)

	// Transfer 10000 tokens from Addr4 to Addr5 (must fail due to insufficient balance)
	consumed, ok = suite.callmethodTransfer(types.HexToAddress(erc20Addr4), types.HexToAddress(erc20Addr5), 10000)
	suite.Equal(false, ok)
	suite.Equal(engineio.Fuel(150), consumed)
}

func (suite *ERC20TestSuite) TestInflation() {
	// Check total supply of token (must be initial seed amount of 100000000)
	consumed, supply := suite.callmethodTotalSupply()
	suite.Equal(uint64(100000000), supply)
	suite.Equal(engineio.Fuel(70), consumed)

	// Burn 10000 tokens from Addr1
	consumed, ok := suite.callmethodBurn(types.HexToAddress(erc20Addr1), 10000)
	suite.Equal(true, ok)
	suite.Equal(engineio.Fuel(220), consumed)

	// Check balance of Addr1 (must be 99990000)
	consumed, balance := suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(99990000), balance)
	suite.Equal(engineio.Fuel(90), consumed)

	// Check total supply of token (must be 99990000)
	consumed, supply = suite.callmethodTotalSupply()
	suite.Equal(uint64(99990000), supply)
	suite.Equal(engineio.Fuel(70), consumed)

	// Mint 10000 tokens to Addr6
	consumed, ok = suite.callmethodMint(types.HexToAddress(erc20Addr6), 10000)
	suite.Equal(true, ok)
	suite.Equal(engineio.Fuel(180), consumed)

	// Check balance of Addr6 (must be 10000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr6))
	suite.Equal(uint64(10000), balance)
	suite.Equal(engineio.Fuel(90), consumed)

	// Check total supply of token (must be 100000000)
	consumed, supply = suite.callmethodTotalSupply()
	suite.Equal(uint64(100000000), supply)
	suite.Equal(engineio.Fuel(70), consumed)

	// Burn 1000000 tokens from Addr6 (must fail due to insufficient balance)
	consumed, ok = suite.callmethodBurn(types.HexToAddress(erc20Addr6), 100000)
	suite.Equal(false, ok)
	suite.Equal(engineio.Fuel(150), consumed)
}

func (suite *ERC20TestSuite) performExecution(callsite string, calldata polo.Document) *engineio.CallResult {
	// Create a PISA Engine for the executor
	executor := suite.factory.NewEngine()
	if err := executor.Bootstrap(context.Background(), 500, suite.logic, suite.internal, nil); err != nil {
		suite.T().Fatalf("Bootstrap Failed: %v", err)
	}

	// Execute the invokable function
	ixn := engineio.NewIxnObject(types.IxLogicInvoke, callsite, calldata)
	result := executor.Call(context.Background(), engineio.InvokableCallsite, ixn, nil)
	// Check if the result is Ok
	if !result.Ok() {
		suite.T().Fatalf("Execute Failed '%v' | Error: [%v] %v", callsite, result.ErrCode, result.ErrMessage)
	}

	return result
}

func (suite *ERC20TestSuite) callmethodName() (engineio.Fuel, string) {
	result := suite.performExecution("Name", nil)

	name := new(string)
	if err := result.Outputs.Get("name", name); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *name
}

func (suite *ERC20TestSuite) callmethodSymbol() (engineio.Fuel, string) {
	result := suite.performExecution("Symbol", nil)

	symbol := new(string)
	if err := result.Outputs.Get("symbol", symbol); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *symbol
}

func (suite *ERC20TestSuite) callmethodDecimals() (engineio.Fuel, uint64) {
	result := suite.performExecution("Decimals", nil)

	decimals := new(uint64)
	if err := result.Outputs.Get("decimals", decimals); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *decimals
}

func (suite *ERC20TestSuite) callmethodTotalSupply() (engineio.Fuel, uint64) {
	result := suite.performExecution("TotalSupply", nil)

	supply := new(uint64)
	if err := result.Outputs.Get("supply", supply); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *supply
}

func (suite *ERC20TestSuite) callmethodBalanceOf(address types.Address) (engineio.Fuel, uint64) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)

	result := suite.performExecution("BalanceOf", inputs)

	balance := new(uint64)
	if err := result.Outputs.Get("balance", balance); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *balance
}

func (suite *ERC20TestSuite) callmethodAllowance(owner, spender types.Address) (engineio.Fuel, uint64) {
	inputs := make(polo.Document)
	_ = inputs.Set("owner", owner)
	_ = inputs.Set("spender", spender)

	result := suite.performExecution("Allowance", inputs)

	allowance := new(uint64)
	if err := result.Outputs.Get("allowance", allowance); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *allowance
}

func (suite *ERC20TestSuite) callmethodApprove(owner, spender types.Address, amount uint64) (engineio.Fuel, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("owner", owner)
	_ = inputs.Set("spender", spender)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution("Approve!", inputs)

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodTransfer(from, to types.Address, amount uint64) (engineio.Fuel, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("from", from)
	_ = inputs.Set("to", to)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution("Transfer!", inputs)

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodMint(address types.Address, amount uint64) (engineio.Fuel, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution("Mint!", inputs)

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodBurn(address types.Address, amount uint64) (engineio.Fuel, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution("Burn!", inputs)

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}
