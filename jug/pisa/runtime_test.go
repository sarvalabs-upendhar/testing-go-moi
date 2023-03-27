package pisa

import (
	"context"
	"encoding/hex"
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

	manifestHexERC20 = "0e4f065ede01302e312e302f064e504953410fef030e9e14ce27be2bce2f9e39ae438e4ffe59ce689e7c8e9301" +
		"8eaa01eec0014f0006505e73746174653f06ae0170657273697374656e749f010eee01fe038e068e0a3f0006466e616d657374726" +
		"96e673f0316760173796d626f6c737472696e673f03167602737570706c7975696e7436344f031696010362616c616e6365736d61" +
		"705b616464726573735d75696e7436344f0316b60104616c6c6f77616e6365736d61705b616464726573735d6d61705b616464726" +
		"573735d75696e7436346f03168e01ae0101726f7574696e651f00af010676fe01900b900b9e0b536565646572216465706c6f7965" +
		"727f0eee01fe038e063f0006466e616d65737472696e673f0316760173796d626f6c737472696e673f03167602737570706c79756" +
		"96e7436343f03167603736565646572616464726573735f06f603f003100000140000100001140001100002140002130103100203" +
		"410102001401036f031690019e0102636f6e7374616e742f066675696e7436343078303330616f031680018601037479706564656" +
		"66d61705b616464726573735d75696e7436346f03168e01ae0104726f7574696e651f00af010646d001de01d003de034e616d6569" +
		"6e766f6b61626c651f0e3f0006466e616d65737472696e674f0006e00130783133303030303131303030306f03168e01ae0105726" +
		"f7574696e651f00af010666f001fe0190049e0453796d626f6c696e766f6b61626c651f0e3f00066673796d626f6c737472696e67" +
		"4f0006e00130783133303030313131303030306f03168e01be0106726f7574696e651f0302bf0106860190029e02e004ee0444656" +
		"3696d616c73696e766f6b61626c651f0e4f00068601646563696d616c7375696e7436344f0006c002307830353031303030323036" +
		"30303131303030306f03168e01ae0107726f7574696e651f00bf0106b601c002ce02e004ee04546f74616c537570706c79696e766" +
		"f6b61626c651f0e3f000666737570706c7975696e7436344f0006e00130783133303030323131303030306f03168e01ae0108726f" +
		"7574696e651f00bf01069601ae02be04e006ee0642616c616e63654f66696e766f6b61626c651f0e3f00064661646472616464726" +
		"573731f0e3f00067662616c616e636575696e7436344f0006c0033078313330303033313030313030343030323030303131313032" +
		"30306f03168e01ae0109726f7574696e651f00bf01069601ae02ae07800a8e0a416c6c6f77616e6365696e766f6b61626c653f0e8" +
		"e023f0006566f776e6572616464726573734f03168601017370656e646572616464726573731f0e4f00069601616c6c6f77616e63" +
		"6575696e7436344f0006a005307831333030303431303031303034303032303030313130303330313430303430323033313130343" +
		"0306f03168e01ce010a726f7574696e652f000303bf010686019e02ce09800b8e0b417070726f766521696e766f6b61626c655f0e" +
		"8e02ce043f0006566f776e6572616464726573734f03168601017370656e646572616464726573733f03167602616d6f756e74756" +
		"96e7436341f0e3f0006266f6b626f6f6c5f06960790071300041001004002000133030256030501040a0303040501040007043c02" +
		"0401100301100402410203044100010214000404000156001100006f03168e01ae010b726f7574696e651f00bf01069601ae02ee0" +
		"8a00aae0a5472616e7366657221696e766f6b61626c655f0efe01de033f00064666726f6d616464726573733f03163601746f6164" +
		"64726573733f03167602616d6f756e7475696e7436341f0e3f0006266f6b626f6f6c5f06a608a0081300031001004002000110030" +
		"2500403020501050a03040504000111000000015a0402034100010410050140040005590604034100050614000304000156001100" +
		"006f03168e01ae010c726f7574696e651f00af010656ee01ae06e007ee074d696e7421696e766f6b61626c653f0e8e023f0006666" +
		"16d6f756e7475696e7436343f0316560161646472616464726573731f0e3f0006266f6b626f6f6c4f0006e00a3078313330303032" +
		"313030313030353930303030303131343030303231333030303331303032303134303033303030323539303330333031343130303" +
		"0323033313430303033303430303031353630303131303030306f03168e01ae010d726f7574696e651f00af010656ee01ae06e007" +
		"ee074275726e21696e766f6b61626c653f0e8e023f000666616d6f756e7475696e7436343f0316560161646472616464726573731" +
		"f0e3f0006266f6b626f6f6c4f0006c00f307831333030303331303031303134303032303030313130303330303530303430333032" +
		"303530313035304130333034303530343030303131313030303030303031354130343032303334313030303130343134303030333" +
		"13330303032354130303030303331343030303230343030303135363030313130303030"

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

	// Decode hex string into manifest bytes
	manifestData, err := hex.DecodeString(manifestHexERC20)
	if err != nil {
		suite.T().Fatalf("manifest hex decode: %v", err)
	}

	manifest, err := engineio.NewManifest(manifestData, engineio.POLO)
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
