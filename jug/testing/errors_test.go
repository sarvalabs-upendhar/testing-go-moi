package testing

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/types"
)

type ErrorsTestSuite struct {
	LogicTestSuite
}

func TestErrorsTestSuite(t *testing.T) {
	suite.Run(t, new(ErrorsTestSuite))
}

func (suite *ErrorsTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./../manifests/errors.yaml")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID, _ := types.NewLogicIDv0(false, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, 5000)
	suite.Equal(engineio.Fuel(100), consumed)
}

func (suite *ErrorsTestSuite) TestGetThrownError() {
	consumed, output, except := suite.Call("GetThrownError", nil)
	suite.Nil(output)
	suite.Equal(engineio.Fuel(60), consumed)
	suite.Equal(except, must(polo.Polorize(pisa.Exception{
		Class: "string",
		Error: "hello!",
		Trace: []string{
			"root.start()",
			"root.GetThrownError() [0x2] ... [0x2: THROW 0x0]",
		},
	})))
}

func (suite *ErrorsTestSuite) TestGetSystemError() {
	consumed, output, except := suite.Call("GetSystemError", map[string]any{"a": "foo", "b": "bar"})
	suite.Nil(output)
	suite.Equal(engineio.Fuel(10), consumed)
	suite.Equal(except, must(polo.Polorize(pisa.Exception{
		Class: "builtin.ValueError",
		Error: "cannot add with string registers",
		Trace: []string{
			"root.start()",
			"root.GetSystemError() [0x3] ... [0x2: ADD 0x2 0x1 0x0]",
		},
	})))
}

func (suite *ErrorsTestSuite) TestGetOverflowError() {
	consumed, output, except := suite.Call("GetOverflowError", map[string]any{"value": 10})
	suite.Nil(output)
	suite.Equal(engineio.Fuel(75), consumed)
	suite.Equal(except, must(polo.Polorize(pisa.Exception{
		Class: "builtin.OverflowError",
		Error: "subtraction overflow",
		Trace: []string{
			"root.start()",
			"root.GetOverflowError() [0x4] ... [0x4: SUB 0x2 0x0 0x1]",
		},
	})))
}

func (suite *ErrorsTestSuite) TestGetConditionalError() {
	consumed, output, except := suite.Call("GetConditionalError", map[string]any{"fail": false})
	suite.Equal(output, map[string]any{})
	suite.Equal(engineio.Fuel(51), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetConditionalError", map[string]any{"fail": true})
	suite.Nil(output)
	suite.Equal(engineio.Fuel(100), consumed)
	suite.Equal(except, must(polo.Polorize(pisa.Exception{
		Class: "string",
		Error: "failed!",
		Trace: []string{
			"root.start()",
			"root.GetConditionalError() [0x6] ... [0x6: THROW 0x2]",
		},
	})))
}
