package pisa

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	gtypes "github.com/sarvalabs/moichain/guna/types"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

const (
	addressERC20 = "21544c5299db3231e6b0c0a98d5b880debd8af2c7eda2eae3cc6de7572418b60"
	logicidERC20 = "02000021544c5299db3231e6b0c0a98d5b880debd8af2c7eda2eae3cc6de7572418b60"

	//nolint:lll
	manifestHexERC20 = `0e8f0206165e9e25de288ea001d0a201d0a20131504953413f0ede14af020306d302e602b305c6059308a608b30cc60c6e616d65202020202020205b20737472696e67205d0173796d626f6c20202020205b20737472696e67205d02737570706c7920202020205b2075696e743634205d0362616c616e6365732020205b206d61705b616464726573735d75696e743634205d04616c6c6f77616e636573205b206d61705b616464726573735d6d61705b616464726573735d75696e743634205d8f01060ee009e009ee09ef010306d301e601d303e603d305e6056e616d65205b737472696e675d0173796d626f6c205b737472696e675d02737570706c79205b75696e7436345d03736565646572205b616464726573735d3f06f003100000140000100001140001100002140002130103100203410102001401036f0306a301b60175696e7436342831302901626f6f6c287472756529ef04030ed304ee04f3098e0ac310de10e316fe1683209e20a32dbe2dc340de40a354be549362ae627f06404ec002ce024e616d652f03066e616d65205b737472696e675d2f0660130000110000017f06606e80038e0353796d626f6c2f030673796d626f6c205b737472696e675d2f0660130001110000029f010680018e01c003ce03446563696d616c732f0306646563696d616c73205b75696e7436345d3f069001050100000600110000039f0106b001be01d003de03546f74616c537570706c792f0306737570706c79205b75696e7436345d2f0660130002110000049f01069e01ae03d005de0542616c616e63654f662f030661646472205b616464726573735d2f030662616c616e6365205b75696e7436345d3f06d00113000310010040020001110200059f01069e019e06e008ee08416c6c6f77616e63656f0306f30186026f776e6572205b616464726573735d017370656e646572205b616464726573735d2f0306616c6c6f77616e6365205b75696e7436345d3f06c0021300041001004002000110030140040203110400069f01068e01de08900a9e0a417070726f766521af010306f30186029304a6046f776e6572205b616464726573735d017370656e646572205b616464726573735d02616d6f756e74205b75696e7436345d2f03066f6b205b626f6f6c5d3f0690071300041001004002000133030256030501040a0303040501040007043c02040110030110040241020304410001021400040400015600110000079f01069e018e08c009ce095472616e7366657221af010306e301f601b303c60366726f6d205b616464726573735d01746f205b616464726573735d02616d6f756e74205b75696e7436345d2f03066f6b205b626f6f6c5d3f06a00813000310010040020001100302500403020501050a03040504000111000000015a040203410001041005014004000559060403410005061400030400015600110000088f01065eae05e006ee064d696e74216f0306f3018602616d6f756e74205b75696e7436345d0161646472205b616464726573735d2f03066f6b205b626f6f6c5d3f06a005130002100100590000011400021300031002014003000259030301410002031400030400015600110000098f01065eae05e006ee064275726e216f0306f3018602616d6f756e74205b75696e7436345d0161646472205b616464726573735d2f03066f6b205b626f6f6c5d3f06d00713000310010140020001100300500403020501050a03040504000111000000015a040203410001041400031300025a00000314000204000156001100002f03066d61705b616464726573735d75696e743634`

	erc20Addr1 = "ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34"
	erc20Addr2 = "0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"
	erc20Addr3 = "916f62f7bdf311ba46d9df465283aa4e97a5dcf36e4c0761bc7d87d32557f8cc"
	erc20Addr4 = "39c1124554d32aa82c4388105c89b3805cbb2c461f4615de0a0e16457d4c7cac"
	erc20Addr5 = "62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa"
	erc20Addr6 = "fafe52ec42a85db6441eaf1c438daa4e97a5dcf36e4c0761bc7d87d32557f8cc"
)

type ERC20TestSuite struct {
	suite.Suite

	factory Factory
	logic   ctypes.Logic

	storage  *ctypes.MemStorage
	snapshot *ctypes.MemStorage
}

func TestERC20TestSuite(t *testing.T) {
	suite.Run(t, new(ERC20TestSuite))
}

func (suite *ERC20TestSuite) SetupSuite() {
	// Setup factory and fuel tank
	suite.factory = NewFactory()

	// Decode hex string into manifest bytes
	manifest, err := hex.DecodeString(manifestHexERC20)
	if err != nil {
		suite.T().Fatalf("manifest hex decode: %v", err)
	}

	// Create a PISA Engine for the compiler
	compiler := suite.factory.NewExecutionEngine(500)
	// Compile the Manifest into a LogicDescriptor
	descriptor, result := compiler.Compile(context.Background(), manifest)
	if result.ErrCode != 0 {
		suite.T().Fatalf("Compile Failed! Error: [%v] %v\n", result.ErrCode, result.ErrMessage)
	}

	suite.Equal(uint64(50), result.Fuel)

	// Generate a Logic ID
	logicID, err := types.NewLogicIDv0(types.LogicKindSimple, true, false, 0, types.HexToAddress(addressERC20))
	if err != nil {
		suite.T().Fatalf("logic id generation: %v", err)
	}

	// Check if logic ID was generated correctly
	suite.Equal(logicidERC20, logicID.Hex(), "unexpected logic id")

	// Generate a new LogicObject from the LogicDescriptor
	logic := gtypes.NewLogicObject(logicID, descriptor)
	// Generate a new storage object
	storage := ctypes.NewMemStorage(logicID.Address())

	// Generate the init data and document encode it
	inputs, _ := polo.DocumentEncode(struct {
		Name   string        `polo:"name"`
		Symbol string        `polo:"symbol"`
		Supply uint64        `polo:"supply"`
		Seeder types.Address `polo:"seeder"`
	}{
		"MOI-Token", "MOI", 100000000, types.HexToAddress(erc20Addr1),
	})

	// Create an ExecutionOrder
	order := &ctypes.ExecutionOrder{Initialise: true, Callee: storage, Inputs: inputs}
	// Create a PISA Engine for the executor
	executor := suite.factory.NewExecutionEngine(500)
	// Execute the builder function
	result = executor.Execute(context.Background(), logic, order)
	if result.ErrCode != 0 {
		suite.T().Fatalf("Initialise Failed! Error: [%v] %v\n", result.ErrCode, result.ErrMessage)
	}

	suite.Equal(uint64(200), result.Fuel)

	suite.logic = logic
	suite.storage = storage
}

func (suite *ERC20TestSuite) SetupTest() {
	suite.snapshot = suite.storage.Copy()
}

func (suite *ERC20TestSuite) TearDownTest() {
	suite.storage = suite.snapshot.Copy()
}

func (suite *ERC20TestSuite) TestReadMethods() {
	consumed, name := suite.callmethodName()
	suite.Equal(name, "MOI-Token")
	suite.Equal(uint64(70), consumed)

	consumed, symbol := suite.callmethodSymbol()
	suite.Equal(symbol, "MOI")
	suite.Equal(uint64(70), consumed)

	consumed, decimals := suite.callmethodDecimals()
	suite.Equal(decimals, uint64(10))
	suite.Equal(uint64(80), consumed)

	consumed, supply := suite.callmethodTotalSupply()
	suite.Equal(supply, uint64(100000000))
	suite.Equal(uint64(70), consumed)
}

func (suite *ERC20TestSuite) TestApproval() {
	// Approve Addr2 to spend 500 tokens of Addr1
	consumed, ok := suite.callmethodApprove(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr2), 500)
	suite.Equal(true, ok)
	suite.Equal(uint64(210), consumed)

	// Check allowance of Addr2 on Addr1 tokens (must be 500)
	consumed, allowance := suite.callmethodAllowance(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr2))
	suite.Equal(uint64(500), allowance)
	suite.Equal(uint64(110), consumed)

	// Check allowance of Addr3 on Addr2 tokens (must be 0)
	consumed, allowance = suite.callmethodAllowance(types.HexToAddress(erc20Addr2), types.HexToAddress(erc20Addr3))
	suite.Equal(uint64(0), allowance)
	suite.Equal(uint64(110), consumed)
}

func (suite *ERC20TestSuite) TestTransfer() {
	// Check balance of Addr1 (must be initial seed amount of 100000000)
	consumed, balance := suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(100000000), balance)
	suite.Equal(uint64(90), consumed)

	// Transfer 1000 tokens from Addr1 to Addr4
	consumed, ok := suite.callmethodTransfer(types.HexToAddress(erc20Addr1), types.HexToAddress(erc20Addr4), 1000)
	suite.Equal(true, ok)
	suite.Equal(uint64(230), consumed)

	// Check balance of Addr1 (must be 100000000 - 1000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(99999000), balance)
	suite.Equal(uint64(90), consumed)

	// Check balance of Addr4 (must be 1000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr4))
	suite.Equal(uint64(1000), balance)
	suite.Equal(uint64(90), consumed)

	// Transfer 10000 tokens from Addr4 to Addr5 (must fail due to insufficient balance)
	consumed, ok = suite.callmethodTransfer(types.HexToAddress(erc20Addr4), types.HexToAddress(erc20Addr5), 10000)
	suite.Equal(false, ok)
	suite.Equal(uint64(150), consumed)
}

func (suite *ERC20TestSuite) TestInflation() {
	// Check total supply of token (must be initial seed amount of 100000000)
	consumed, supply := suite.callmethodTotalSupply()
	suite.Equal(uint64(100000000), supply)
	suite.Equal(uint64(70), consumed)

	// Burn 10000 tokens from Addr1
	consumed, ok := suite.callmethodBurn(types.HexToAddress(erc20Addr1), 10000)
	suite.Equal(true, ok)
	suite.Equal(uint64(220), consumed)

	// Check balance of Addr1 (must be 99990000)
	consumed, balance := suite.callmethodBalanceOf(types.HexToAddress(erc20Addr1))
	suite.Equal(uint64(99990000), balance)
	suite.Equal(uint64(90), consumed)

	// Check total supply of token (must be 99990000)
	consumed, supply = suite.callmethodTotalSupply()
	suite.Equal(uint64(99990000), supply)
	suite.Equal(uint64(70), consumed)

	// Mint 10000 tokens to Addr6
	consumed, ok = suite.callmethodMint(types.HexToAddress(erc20Addr6), 10000)
	suite.Equal(true, ok)
	suite.Equal(uint64(180), consumed)

	// Check balance of Addr6 (must be 10000)
	consumed, balance = suite.callmethodBalanceOf(types.HexToAddress(erc20Addr6))
	suite.Equal(uint64(10000), balance)
	suite.Equal(uint64(90), consumed)

	// Check total supply of token (must be 100000000)
	consumed, supply = suite.callmethodTotalSupply()
	suite.Equal(uint64(100000000), supply)
	suite.Equal(uint64(70), consumed)

	// Burn 1000000 tokens from Addr6 (must fail due to insufficient balance)
	consumed, ok = suite.callmethodBurn(types.HexToAddress(erc20Addr6), 100000)
	suite.Equal(false, ok)
	suite.Equal(uint64(150), consumed)
}

func (suite *ERC20TestSuite) performExecution(order *ctypes.ExecutionOrder) *ctypes.ExecutionResult {
	// Create a PISA Engine for the executor
	executor := suite.factory.NewExecutionEngine(500)
	// Execute the builder function
	result := executor.Execute(context.Background(), suite.logic, order)
	if !suite.Equal(result.ErrCode, uint64(0)) {
		suite.T().Fatalf("Execute Failed '%v' | Error: [%v] %v", order.Callsite, result.ErrCode, result.ErrMessage)
	}

	return result
}

func (suite *ERC20TestSuite) callmethodName() (uint64, string) {
	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Name", Callee: suite.storage, Inputs: nil})

	name := new(string)
	if err := result.Outputs.Get("name", name); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *name
}

func (suite *ERC20TestSuite) callmethodSymbol() (uint64, string) {
	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Symbol", Callee: suite.storage, Inputs: nil})

	symbol := new(string)
	if err := result.Outputs.Get("symbol", symbol); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *symbol
}

func (suite *ERC20TestSuite) callmethodDecimals() (uint64, uint64) {
	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Decimals", Callee: suite.storage, Inputs: nil})

	decimals := new(uint64)
	if err := result.Outputs.Get("decimals", decimals); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *decimals
}

func (suite *ERC20TestSuite) callmethodTotalSupply() (uint64, uint64) {
	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "TotalSupply", Callee: suite.storage, Inputs: nil})

	supply := new(uint64)
	if err := result.Outputs.Get("supply", supply); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *supply
}

func (suite *ERC20TestSuite) callmethodBalanceOf(address types.Address) (uint64, uint64) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "BalanceOf", Callee: suite.storage, Inputs: inputs})

	balance := new(uint64)
	if err := result.Outputs.Get("balance", balance); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *balance
}

func (suite *ERC20TestSuite) callmethodAllowance(owner, spender types.Address) (uint64, uint64) {
	inputs := make(polo.Document)
	_ = inputs.Set("owner", owner)
	_ = inputs.Set("spender", spender)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Allowance", Callee: suite.storage, Inputs: inputs})

	allowance := new(uint64)
	if err := result.Outputs.Get("allowance", allowance); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *allowance
}

func (suite *ERC20TestSuite) callmethodApprove(owner, spender types.Address, amount uint64) (uint64, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("owner", owner)
	_ = inputs.Set("spender", spender)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Approve!", Callee: suite.storage, Inputs: inputs})

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodTransfer(from, to types.Address, amount uint64) (uint64, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("from", from)
	_ = inputs.Set("to", to)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Transfer!", Callee: suite.storage, Inputs: inputs})

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodMint(address types.Address, amount uint64) (uint64, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Mint!", Callee: suite.storage, Inputs: inputs})

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}

func (suite *ERC20TestSuite) callmethodBurn(address types.Address, amount uint64) (uint64, bool) {
	inputs := make(polo.Document)
	_ = inputs.Set("addr", address)
	_ = inputs.Set("amount", amount)

	result := suite.performExecution(&ctypes.ExecutionOrder{Callsite: "Burn!", Callee: suite.storage, Inputs: inputs})

	ok := new(bool)
	if err := result.Outputs.Get("ok", ok); err != nil {
		suite.T().Fatalf("Output Decode Failed: %v", err)
	}

	return result.Fuel, *ok
}
