package testing

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

type FlipperTestSuite struct {
	LogicTestSuite
}

func TestFlipperTestSuite(t *testing.T) {
	suite.Run(t, new(FlipperTestSuite))
}

func (suite *FlipperTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./../manifests/flipper.json")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID, _ := types.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, 5000)
	suite.Equal(engineio.Fuel(100), consumed)
}

func (suite *FlipperTestSuite) TestFlipping() {
	consumed, output, except := suite.Call("Mode", nil)
	suite.Equal(false, output["value"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	consumed, _, except = suite.Call("Set!", map[string]any{"value": true})
	suite.Equal(engineio.Fuel(105), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("Mode", nil)
	suite.Equal(true, output["value"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)

	consumed, _, except = suite.Call("Flip!", nil)
	suite.Equal(engineio.Fuel(165), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("Mode", nil)
	suite.Equal(false, output["value"])
	suite.Equal(engineio.Fuel(55), consumed)
	suite.Nil(except)
}
