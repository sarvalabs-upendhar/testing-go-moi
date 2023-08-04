package manifests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
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

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000), common.NilAddress)
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

func (suite *EnvCodesTestSuite) TestGetTimeStamp() {
	consumed, output, except := suite.Call("GetTimeStamp", nil)
	suite.Equal(time.Now().Unix(), output["time"])
	suite.Equal(engineio.NewFuel(110), consumed)
	suite.Nil(except)
}

func (suite *EnvCodesTestSuite) TestClusterID() {
	consumed, output, except := suite.Call("GetCluster", nil)
	suite.Equal("Test", output["cluster"])
	suite.Equal(engineio.NewFuel(110), consumed)
	suite.Nil(except)
}

func (suite *EnvCodesTestSuite) TestGetIxnType() {
	consumed, output, except := suite.Call("GetIxnType", nil)
	suite.Equal("IxLogicInvoke", output["ixnType"])
	suite.Equal(engineio.NewFuel(110), consumed)
	suite.Nil(except)
}
