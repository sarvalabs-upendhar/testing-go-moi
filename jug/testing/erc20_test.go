package testing

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

const (
	erc20Addr1 = "ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34"
	erc20Addr2 = "0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"
	erc20Addr3 = "916f62f7bdf311ba46d9df465283aa4e97a5dcf36e4c0761bc7d87d32557f8cc"
	erc20Addr4 = "39c1124554d32aa82c4388105c89b3805cbb2c461f4615de0a0e16457d4c7cac"
	erc20Addr5 = "62dbd666303ff4dfa7bf390e1eaf1d6be58df23ab6ac5adc0de54fada389acaa"
	erc20Addr6 = "fafe52ec42a85db6441eaf1c438daa4e97a5dcf36e4c0761bc7d87d32557f8cc"
)

type ERC20TestSuite struct {
	LogicTestSuite

	filename string
}

func TestERC20TestSuiteJSON(t *testing.T) {
	erc20 := new(ERC20TestSuite)
	erc20.filename = "./../manifests/erc20.json"
	suite.Run(t, erc20)
}

func TestERC20TestSuiteYAML(t *testing.T) {
	erc20 := new(ERC20TestSuite)
	erc20.filename = "./../manifests/erc20.yaml"
	suite.Run(t, erc20)
}

func (suite *ERC20TestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile(suite.filename)
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID, _ := types.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, 5000)
	suite.Equal(engineio.Fuel(100), consumed)

	// Generate the init data and document encode it
	inputs, _ := polo.DocumentEncode(struct {
		Name   string        `polo:"name"`
		Symbol string        `polo:"symbol"`
		Supply uint64        `polo:"supply"`
		Seeder types.Address `polo:"seeder"`
	}{
		"MOI-Token", "MOI", 100000000, types.HexToAddress(erc20Addr1),
	})

	consumed = suite.Deploy(engineio.NewIxnObject(types.IxLogicDeploy, "Seeder!", inputs.Bytes()))
	suite.Equal(engineio.Fuel(490), consumed)
}

func (suite *ERC20TestSuite) TestReadMethods() {
	consumed, outputs, except := suite.Call("Name", nil)
	suite.Equal("MOI-Token", outputs["name"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	consumed, outputs, except = suite.Call("Symbol", nil)
	suite.Equal("MOI", outputs["symbol"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	consumed, outputs, except = suite.Call("Decimals", nil)
	suite.Equal(uint64(10), outputs["decimals"])
	suite.Equal(engineio.Fuel(36), consumed)
	suite.Nil(except)

	consumed, outputs, except = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), outputs["supply"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)
}

func (suite *ERC20TestSuite) TestApproval() {
	// Approve Addr2 to spend 500 tokens of Addr1
	consumed, output, except := suite.Call("Approve!", map[string]any{
		"owner":   types.HexToAddress(erc20Addr1),
		"spender": types.HexToAddress(erc20Addr2),
		"amount":  500,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(297), consumed)
	suite.Nil(except)

	// Check allowance of Addr2 on Addr1 tokens (must be 500)
	consumed, output, except = suite.Call("Allowance", map[string]any{
		"owner":   types.HexToAddress(erc20Addr1),
		"spender": types.HexToAddress(erc20Addr2),
	})
	suite.Equal(uint64(500), output["allowance"])
	suite.Equal(engineio.Fuel(85), consumed)
	suite.Nil(except)

	// Check allowance of Addr3 on Addr2 tokens (must be 0)
	consumed, output, except = suite.Call("Allowance", map[string]any{
		"owner":   types.HexToAddress(erc20Addr2),
		"spender": types.HexToAddress(erc20Addr3),
	})
	suite.Equal(uint64(0), output["allowance"])
	suite.Equal(engineio.Fuel(85), consumed)
	suite.Nil(except)
}

func (suite *ERC20TestSuite) TestTransfer() {
	// Check balance of Addr1 (must be initial seed amount of 100000000)
	consumed, output, except := suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(100000000), output["balance"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	// Transfer 1000 tokens from Addr1 to Addr4
	consumed, output, except = suite.Call("Transfer!", map[string]any{
		"from":   types.HexToAddress(erc20Addr1),
		"to":     types.HexToAddress(erc20Addr4),
		"amount": 1000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(347), consumed)
	suite.Nil(except)

	// Check balance of Addr1 (must be 100000000 - 1000)
	consumed, output, except = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(99999000), output["balance"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	// Check balance of Addr4 (must be 1000)
	consumed, output, except = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr4)})
	suite.Equal(uint64(1000), output["balance"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	// Transfer 10000 tokens from Addr4 to Addr5 (must fail due to insufficient balance)
	consumed, output, except = suite.Call("Transfer!", map[string]any{
		"from":   types.HexToAddress(erc20Addr4),
		"to":     types.HexToAddress(erc20Addr5),
		"amount": 10000,
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.Fuel(126), consumed)
	suite.Nil(except)
}

func (suite *ERC20TestSuite) TestInflation() {
	// Check total supply of token (must be initial seed amount of 100000000)
	consumed, output, except := suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), output["supply"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	// Burn 10000 tokens from Addr1
	consumed, output, except = suite.Call("Burn!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr1),
		"amount": 10000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(462), consumed)
	suite.Nil(except)

	// Check balance of Addr1 (must be 99990000)
	consumed, output, except = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr1)})
	suite.Equal(uint64(99990000), output["balance"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	// Check total supply of token (must be 99990000)
	consumed, output, except = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(99990000), output["supply"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	// Mint 10000 tokens to Addr6
	consumed, output, except = suite.Call("Mint!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr6),
		"amount": 10000,
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.Fuel(410), consumed)
	suite.Nil(except)

	// Check balance of Addr6 (must be 10000)
	consumed, output, except = suite.Call("BalanceOf", map[string]any{"addr": types.HexToAddress(erc20Addr6)})
	suite.Equal(uint64(10000), output["balance"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	// Check total supply of token (must be 100000000)
	consumed, output, except = suite.Call("TotalSupply", nil)
	suite.Equal(uint64(100000000), output["supply"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	// Burn 1000000 tokens from Addr6 (must fail due to insufficient balance)
	consumed, output, except = suite.Call("Burn!", map[string]any{
		"addr":   types.HexToAddress(erc20Addr6),
		"amount": 100000,
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.Fuel(126), consumed)
	suite.Nil(except)
}
