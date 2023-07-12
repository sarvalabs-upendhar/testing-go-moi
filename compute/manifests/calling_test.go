package manifests

import (
	"encoding/hex"
	"testing"

	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
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
	logicID := common.NewLogicIDv0(false, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000))
	suite.Equal(engineio.NewFuel(100), consumed)
}

func (suite *CallingTestSuite) TestExceptionDepth() {
	consumed, output, except := suite.Call("OuterFunc", nil)
	suite.Nil(output)
	suite.Equal(engineio.NewFuel(150), consumed)
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

func (suite *CallingTestSuite) TestBuiltinKeccak256() {
	consumed, output, except := suite.Call("Keccak256", map[string]any{"data": []byte{0xab, 0xcd}})
	suite.Equal(map[string]any{"hash": uint256.MustFromDecimal("6814789511218234881821863707284136101580313219688317995938232648907603067483").ToBig()}, output) //nolint:lll
	suite.Equal(engineio.NewFuel(135), consumed)
	suite.Nil(except)
}

func (suite *CallingTestSuite) TestBuiltinBLAKE2b() {
	consumed, output, except := suite.Call("Blake2b", map[string]any{"data": []byte{0xab, 0xcd}})
	suite.Equal(map[string]any{"hash": uint256.MustFromDecimal("67859110136991185311493348371284861541977304781806664142055349856124235179253").ToBig()}, output) //nolint:lll
	suite.Equal(engineio.NewFuel(135), consumed)
	suite.Nil(except)
}

func (suite *CallingTestSuite) TestBuiltinVerify() {
	pubKeyInHex := "5f2c7306be02b16d0f1ae75ae3fdbedf10b970d98c7646ec5e9beaf325a2e004"
	expectedSignature := "0146304402201b2f03875387cb1964d70d414bae4fc15fc9b244967d973676a" +
		"7087e628e15bc02207ee005106a92fa003e877ee5d773ded75ac6317672fc3b27def321be3b18d7ca03"

	pubKeyBytes, _ := hex.DecodeString(pubKeyInHex)
	sig, _ := hex.DecodeString(expectedSignature)

	_, output, except := suite.Call("Verify", map[string]any{"data": []byte("Hello MOI user, this is the message being signed"), "signature": sig, "pubkey": pubKeyBytes}) //nolint:lll
	suite.Equal(map[string]any{"ok": true}, output)
	suite.Nil(except)
}
