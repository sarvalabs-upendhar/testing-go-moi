package guardianregistry

import (
	"fmt"
	"testing"

	pisatest "github.com/sarvalabs/go-pisa/moi/testing"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
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
	pisatest.LogicTestSuite
}

func TestGuardianTestSuite(t *testing.T) {
	guardian := new(GuardianTestSuite)
	suite.Run(t, guardian)
}

func (suite *GuardianTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./guardian.yaml")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := common.CreateAddressFromString("guardian-registry")
	logicID := common.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(logicID, manifest, address, common.HexToAddress(approverAddr1))
	suite.Equal(engineio.NewFuel(100), consumed)

	calldata := make(polo.Document)
	_ = calldata.Set("enforceApprovals", true)
	_ = calldata.Set("enforceNodeLimits", true)
	_ = calldata.Set("enforceDeviceLimits", true)
	_ = calldata.Set("limitKYC", 3)
	_ = calldata.Set("limitKYB", 5)
	_ = calldata.Set("limitDevice", 4)
	_ = calldata.Set("master", "master")
	_ = calldata.Set("preApprovedKramaIDs", []string{"abc", "def"})
	_ = calldata.Set("preApprovedAddresses", []common.Address{common.HexToAddress(guardAddr1), common.HexToAddress(guardAddr2)}) //nolint:lll
	_ = calldata.Set("approvers", []common.Address{common.HexToAddress(approverAddr1), common.HexToAddress(approverAddr2)})      //nolint:lll

	buffer := polo.NewPolorizer()

	guardians := []Guardian{
		{"master", "ghi", "xy-z0", []byte{0}, common.HexToAddress(guardAddr3), []byte{0}},
		{"master", "ijk", "xy-z0", []byte{0}, common.HexToAddress(guardAddr4), []byte{0}},
	}

	for _, guardian := range guardians {
		doc, _ := polo.DocumentEncode(guardian)

		err := buffer.PolorizeAny(doc.Bytes())
		if err != nil {
			fmt.Println(err)

			return
		}
	}

	calldata.SetRaw("guardians", buffer.Packed())

	_, _, errdata := suite.CallRaw(engineio.DeployerCallsite, "Setup!", calldata.Bytes())
	suite.Nil(errdata)
}

type Guardian struct {
	Operator        string
	KramaID         string
	DeviceID        string
	PublicKey       []byte
	IncentiveWallet common.Address
	ExtraData       []byte
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
	_, _, except := suite.Call("AddOperator!", map[string]any{
		"operatorMOIId":       "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		"verificationDetails": map[string]any{"Kind": "kyc", "Proof": []byte{0}},
	})
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
	guardian := Guardian{
		Operator:        "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		KramaID:         "kramaid-1",
		DeviceID:        "xy-z1",
		PublicKey:       []byte{0},
		IncentiveWallet: common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"),
		ExtraData:       []byte{0},
	}

	calldata := make(polo.Document)
	_ = calldata.Set("operatorID", "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649")
	doc, _ := polo.DocumentEncode(guardian)
	calldata.SetRaw("guardian", doc.Bytes())

	_, _, except := suite.Call("AddOperator!", map[string]any{
		"operatorMOIId":       "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		"verificationDetails": map[string]any{"Kind": "kyc", "Proof": []byte{0}},
	})
	suite.Nil(except)

	_, _, except = suite.Call("Approve!", map[string]any{
		"approvedKramaIDs": []string{"kramaid-1", "kramaid-2"},
	})
	suite.Nil(except)

	_, _, except = suite.CallRaw(engineio.InvokableCallsite, "Register!", calldata.Bytes())
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

	guardian := Guardian{
		Operator:        "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		KramaID:         "kramaid-1",
		DeviceID:        "xy-z2",
		PublicKey:       []byte{0},
		IncentiveWallet: common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"),
		ExtraData:       []byte{0},
	}

	calldata := make(polo.Document)
	_ = calldata.Set("kramaID", "kramaid-1")
	doc, _ := polo.DocumentEncode(guardian)
	calldata.SetRaw("updatedGuardiansDetails", doc.Bytes())

	_, _, except := suite.CallRaw(engineio.InvokableCallsite, "UpdateGuardianDetails!", calldata.Bytes())
	suite.Nil(except)

	_, outputs, except := suite.Call("GetGuardianByKramaID", map[string]any{"kramaID": "kramaid-1"})
	suite.Nil(except)

	encoded, _ := engineio.EncodeValues(outputs["guardian"], nil)
	suite.Equal(doc.Bytes(), encoded)
}

func (suite *GuardianTestSuite) TestAddIncentives() {
	_, _, except := suite.Call("AddIncentives!", map[string]any{
		"kramaIDs": []string{"abc", "def", "ghi", "jkl"},
		"amounts":  []int64{200, 400, 350, 500},
	})
	suite.Nil(except)

	_, outputs, except := suite.Call("GetIncentives", map[string]any{"kramaID": "abc"})
	suite.Equal(int64(0xc8), outputs["incentive"])
	suite.Nil(except)

	_, outputs, except = suite.Call("GetIncentives", map[string]any{"kramaID": "jkl"})
	suite.Equal(int64(0x1f4), outputs["incentive"])
	suite.Nil(except)
}
