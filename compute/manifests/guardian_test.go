package manifests

import (
	"fmt"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"
)

const (
	guardAddr1    = "0000000000000000000000000000000000000000000000000000000000000001"
	guardAddr2    = "0000000000000000000000000000000000000000000000000000000000000002"
	guardAddr3    = "0000000000000000000000000000000000000000000000000000000000000005"
	guardAddr4    = "0x0000000000000000000000000000000000000000000000000000000000000006"
	approverAddr1 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649"
	approverAddr2 = "d3c6a3bbfd78192e8456dc2f9e52b22ddeaf1a94390ab5d80daf4db00c6bd306"
)

type GuardianTestSuite struct {
	LogicTestSuite

	filename string
}

func TestGuardianTestSuiteYAML(t *testing.T) {
	guardian := new(GuardianTestSuite)
	guardian.filename = "./../manifests/guardian-registry/guardian.yaml"
	suite.Run(t, guardian)
}

func TestGuardianTestSuiteASM(t *testing.T) {
	guardian := new(GuardianTestSuite)
	guardian.filename = "./../manifests/guardian-registry/guardianASM.yaml"
	suite.Run(t, guardian)
}

type Guardian struct {
	Operator        string
	KramaID         string
	DeviceID        string
	PublicKey       []byte
	IncentiveWallet common.Address
	ExtraData       []byte
}

func (suite *GuardianTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile(suite.filename)
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID := common.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000), common.HexToAddress(approverAddr1)) //nolint:lll
	suite.Equal(engineio.NewFuel(100), consumed)

	callData := make(polo.Document)
	_ = callData.Set("enforceApprovals", true)
	_ = callData.Set("enforceNodeLimits", true)
	_ = callData.Set("enforceDeviceLimits", true)
	_ = callData.Set("limitKYC", 3)
	_ = callData.Set("limitKYB", 5)
	_ = callData.Set("limitDevice", 4)
	_ = callData.Set("master", "master")
	_ = callData.Set("preApprovedKramaIDs", []string{"abc", "def"})
	_ = callData.Set("preApprovedAddresses", []common.Address{common.HexToAddress(guardAddr1), common.HexToAddress(guardAddr2)}) //nolint:lll
	_ = callData.Set("approvers", []common.Address{common.HexToAddress(approverAddr1), common.HexToAddress(approverAddr2)})

	gbuffer := polo.NewPolorizer()

	guardians := []Guardian{
		{"master", "ghi", "xy-z0", []byte{0}, common.HexToAddress(guardAddr3), []byte{0}},
		{"master", "ijk", "xy-z0", []byte{0}, common.HexToAddress(guardAddr4), []byte{0}},
	}

	for _, guardian := range guardians {
		gdoc, _ := polo.DocumentEncode(guardian)

		err := gbuffer.PolorizeAny(gdoc.Bytes())
		if err != nil {
			fmt.Println(err)

			return
		}
	}

	callData.SetRaw("guardians", gbuffer.Packed())

	_, _, errdata := suite.CallRaw(engineio.DeployerCallsite, "Setup!", callData.Bytes())
	suite.Nil(errdata)
}

func (suite *GuardianTestSuite) TestReadMethods() {
	_, outputs, except := suite.Call("GetTotalGuardiansCount", nil)
	suite.Equal(uint64(2), outputs["count"])
	suite.Nil(except)

	_, outputs, except = suite.Call("GetNodeLimits", map[string]any{})
	suite.Nil(except)
	suite.Equal(uint64(3), outputs["NodeLimitKYC"])
	suite.Equal(uint64(5), outputs["NodeLimitKYB"])

	_, outputs, except = suite.Call("IsApproved", map[string]any{"kramaID": "abc"})
	suite.Equal(true, outputs["isPresent"])
	suite.Nil(except)

	_, outputs, except = suite.Call("IsApproved", map[string]any{"kramaID": "def"})
	suite.Equal(true, outputs["isPresent"])
	suite.Nil(except)
}

func (suite *GuardianTestSuite) TestAddOperator() {
	_, _, except := suite.Call("AddOperator!", map[string]any{"operatorMOIId": "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649", "verificationDetails": map[string]any{"Kind": "kyc", "Proof": []byte{0}}}) //nolint:lll
	suite.Nil(except)

	_, outputs, except := suite.Call("GetOperatorsCount", map[string]any{})
	suite.Equal(uint64(1), outputs["count"])
	suite.Nil(except)
}

func (suite *GuardianTestSuite) TestApprove() {
	_, outputs, except := suite.Call("IsApproved", map[string]any{"kramaID": "kramaid-1"})
	suite.Equal(false, outputs["isPresent"])
	suite.Nil(except)

	_, _, except = suite.Call("Approve!", map[string]any{"approvedKramaIDs": []string{"kramaid-1", "kramaid-2"}})
	suite.Nil(except)

	_, outputs, except = suite.Call("IsApproved", map[string]any{"kramaID": "kramaid-1"})
	suite.Equal(true, outputs["isPresent"])
	suite.Nil(except)

	_, outputs, except = suite.Call("IsApproved", map[string]any{"kramaID": "kramaid-2"})
	suite.Equal(true, outputs["isPresent"])
	suite.Nil(except)
}

func (suite *GuardianTestSuite) TestRegister() {
	callData := make(polo.Document)
	guardian := Guardian{"0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649", "kramaid-1", "xy-z1", []byte{0}, common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"), []byte{0}} //nolint:lll
	_ = callData.Set("operatorID", "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649")
	gdoc, _ := polo.DocumentEncode(guardian)
	callData.SetRaw("guardian", gdoc.Bytes())

	_, _, except := suite.Call("AddOperator!", map[string]any{"operatorMOIId": "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649", "verificationDetails": map[string]any{"Kind": "kyc", "Proof": []byte{0}}}) //nolint:lll
	suite.Nil(except)

	_, _, except = suite.Call("Approve!", map[string]any{"approvedKramaIDs": []string{"kramaid-1", "kramaid-2"}})
	suite.Nil(except)

	_, _, except = suite.CallRaw(engineio.InvokableCallsite, "Register!", callData.Bytes())
	suite.Nil(except)
}

func (suite *GuardianTestSuite) TestIncreaseNodeLimit() {
	_, _, except := suite.Call("IncreaseNodeLimit!", map[string]any{"kind": "kyc", "delta": 4})
	suite.Nil(except)

	_, _, except = suite.Call("IncreaseNodeLimit!", map[string]any{"kind": "kyb", "delta": 2})
	suite.Nil(except)

	_, outputs, except := suite.Call("GetNodeLimits", map[string]any{})
	suite.Nil(except)
	suite.Equal(uint64(4), outputs["NodeLimitKYC"])
	suite.Equal(uint64(2), outputs["NodeLimitKYB"])
}

func (suite *GuardianTestSuite) TestUpdateGuardian() {
	suite.TestApprove()
	suite.TestAddOperator()
	suite.TestRegister()

	callData1 := make(polo.Document)
	guardian := Guardian{"0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649", "kramaid-1", "xy-z2", []byte{0}, common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"), []byte{0}} //nolint:lll
	_ = callData1.Set("kramaID", "kramaid-1")
	gdoc1, _ := polo.DocumentEncode(guardian)
	callData1.SetRaw("updatedGuardiansDetails", gdoc1.Bytes())
	callData1.SetRaw("updatedGuardiansDetails", gdoc1.Bytes())

	_, _, except := suite.CallRaw(engineio.InvokableCallsite, "UpdateGuardianDetails!", callData1.Bytes())
	suite.Nil(except)

	_, outputs, except := suite.Call("GetGuardianByKramaID", map[string]any{"kramaID": "kramaid-1"})
	suite.Nil(except)

	encoded, _ := engineio.EncodeValues(outputs["guardian"], nil)
	suite.Equal(gdoc1.Bytes(), encoded)
}

func (suite *GuardianTestSuite) TestAddIncentives() {
	_, _, except := suite.Call("AddIncentives!", map[string]any{"kramaIDs": []string{"abc", "def", "ghi", "jkl"}, "amounts": []uint64{200, 400, 350, 500}}) //nolint:lll
	suite.Nil(except)

	_, outputs, except := suite.Call("GetIncentives", map[string]any{"kramaID": "abc"})
	suite.Equal(uint64(0xc8), outputs["incentive"])
	suite.Nil(except)

	_, outputs, except = suite.Call("GetIncentives", map[string]any{"kramaID": "jkl"})
	suite.Equal(uint64(0x1f4), outputs["incentive"])
	suite.Nil(except)
}
