package testing

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/types"
)

type CallingTestSuite struct {
	LogicTestSuite
}

func TestCallingTestSuite(t *testing.T) {
	suite.Run(t, new(CallingTestSuite))
}

func (suite *CallingTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./../manifests/calling.yaml")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID := types.NewLogicIDv0(false, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000))
	suite.Equal(engineio.NewFuel(100), consumed)
}

func (suite *CallingTestSuite) TestExceptionDepth() {
	consumed, output, except := suite.Call("OuterFunc", nil)
	suite.Nil(output)
	suite.Equal(engineio.NewFuel(190), consumed)
	suite.NotNil(except)

	exception := new(pisa.Exception)
	err := polo.Depolorize(exception, except)
	suite.NoError(err)

	suite.Equal(&pisa.Exception{
		Class: "string",
		Error: "hello!",
		Trace: []string{
			"root.start()",
			"root.OuterFunc() [0x3] ... [0x2: CALLR 0x3 0x2 0x0]",
			"root.MidFunc() [0x2] ... [0x2: CALLR 0x3 0x2 0x0]",
			"root.InnerFunc() [0x1] ... [0x2: THROW 0x0]",
		},
	}, exception)
}

func (suite *CallingTestSuite) TestMethodJoin() {
	consumed, output, except := suite.Call("CombineStrings", map[string]any{"a": "FOO", "b": "BOO"})
	suite.Equal(map[string]any{"result": "FOOBOO"}, output)
	suite.Equal(engineio.NewFuel(120), consumed)
	suite.Nil(except)
}

func (suite *CallingTestSuite) TestBuiltinSHA256() {
	consumed, output, except := suite.Call("Sha256", map[string]any{"data": []byte{0xab, 0xcd}})
	suite.Equal(map[string]any{"hash": uint256.MustFromDecimal("8249936900697619105054612752331407201215506680094776679256130082075110804075").ToBig()}, output) //nolint:lll
	suite.Equal(engineio.NewFuel(135), consumed)
	suite.Nil(except)
}
