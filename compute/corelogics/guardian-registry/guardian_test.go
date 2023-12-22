package guardianregistry

import (
	"testing"

	"github.com/sarvalabs/go-moi-engineio"
	pisatestlib "github.com/sarvalabs/go-pisa/testlib"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
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
	pisatestlib.LogicTestSuite
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
	suite.Equal(uint64(4865), consumed)

	input := SetupInput{
		EnforceApprovals:     true,
		EnforceNodeLimits:    true,
		EnforceDeviceLimits:  true,
		LimitKYC:             3,
		LimitKYB:             5,
		LimitDevice:          4,
		Master:               "master",
		Approvers:            []common.Address{common.HexToAddress(approverAddr1), common.HexToAddress(approverAddr2)},
		PreApprovedKramaIDs:  []string{"abc", "def"},
		PreApprovedAddresses: []common.Address{common.HexToAddress(guardAddr1), common.HexToAddress(guardAddr2)},
		Guardians: []Guardian{
			{"master", "ghi", "xy-z0", []byte{0}, common.HexToAddress(guardAddr3), []byte{0}},
			{"master", "ijk", "xy-z0", []byte{0}, common.HexToAddress(guardAddr4), []byte{0}},
		},
	}

	calldata, _ := polo.Polorize(input, polo.DocStructs())
	_, _, errdata := suite.CallRaw(engineio.DeployerCallsite, "Setup!", calldata)
	suite.Nil(errdata)
}

type SetupInput struct {
	EnforceApprovals    bool `polo:"enforceApprovals"`
	EnforceNodeLimits   bool `polo:"enforceNodeLimits"`
	EnforceDeviceLimits bool `polo:"enforceDeviceLimits"`

	LimitKYC    uint64 `polo:"limitKYC"`
	LimitKYB    uint64 `polo:"limitKYB"`
	LimitDevice uint64 `polo:"limitDevice"`

	Master               string           `polo:"master"`
	Approvers            []common.Address `polo:"approvers"`
	PreApprovedKramaIDs  []string         `polo:"preApprovedKramaIDs"`
	PreApprovedAddresses []common.Address `polo:"preApprovedAddresses"`

	Guardians []Guardian `polo:"guardians"`
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
	calldata := make(polo.Document)
	_ = calldata.Set("operatorID", "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649")
	_ = calldata.Set("guardian", Guardian{
		Operator:        "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		KramaID:         "kramaid-1",
		DeviceID:        "xy-z1",
		PublicKey:       []byte{0},
		IncentiveWallet: common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"),
		ExtraData:       []byte{0},
	}, polo.DocStructs())

	// guardian := Guardian{
	//	Operator:        "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
	//	KramaID:         "kramaid-1",
	//	DeviceID:        "xy-z1",
	//	PublicKey:       []byte{0},
	//	IncentiveWallet: common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"),
	//	ExtraData:       []byte{0},
	//}
	//
	// doc, _ := polo.PolorizeDocument(guardian)
	// calldata.SetRaw("guardian", doc.Bytes())

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

	calldata := make(polo.Document)
	_ = calldata.Set("kramaID", "kramaid-1")

	guardian := Guardian{
		Operator:        "0352fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649",
		KramaID:         "kramaid-1",
		DeviceID:        "xy-z2",
		PublicKey:       []byte{0},
		IncentiveWallet: common.HexToAddress("0x0000000000000000000000000000000000000000000000000000000000000010"),
		ExtraData:       []byte{0},
	}

	doc, _ := polo.PolorizeDocument(guardian)
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
