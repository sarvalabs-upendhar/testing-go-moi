package manifests

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/compute/engineio"
)

type EnvCodesTestSuite struct {
	LogicTestSuite
}

func TestEnvCodesTestSuite(t *testing.T) {
	suite.Run(t, new(EnvCodesTestSuite))
}

func (suite *EnvCodesTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./../manifests/envcodes.yaml")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID := common.NewLogicIDv0(false, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000))
	suite.Equal(engineio.NewFuel(100), consumed)
}

func (suite *EnvCodesTestSuite) TestGetLogicAddress() {
	consumed, output, except := suite.Call("GetLogicAddress", nil)
	suite.Equal(map[string]any{"addr": [32]byte(suite.logic.LogicID().Address())}, output)
	suite.Equal(engineio.NewFuel(55), consumed)
	suite.Nil(except)
}

func (suite *EnvCodesTestSuite) TestGetSenderAddress() {
	consumed, output, except := suite.Call("GetSenderAddress", nil)
	suite.Equal(map[string]any{"addr": [32]byte(suite.sender.Address())}, output)
	suite.Equal(engineio.NewFuel(55), consumed)
	suite.Nil(except)
}
