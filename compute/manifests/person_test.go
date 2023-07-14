package manifests

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

type PersonTestSuite struct {
	LogicTestSuite
}

func TestPersonTestSuite(t *testing.T) {
	suite.Run(t, new(PersonTestSuite))
}

func (suite *PersonTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.ReadManifestFile("./../manifests/person.yaml")
	if err != nil {
		suite.T().Fatalf("manifest read failed: %v", err)
	}

	address := randomAddress()
	logicID := common.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, engineio.NewFuel(5000), common.NilAddress)
	suite.Equal(engineio.NewFuel(100), consumed)
}

func (suite *PersonTestSuite) TestGetNameOf() {
	consumed, output, except := suite.Call("GetNameOf", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal("Joe", output["name"])
	suite.Equal(engineio.NewFuel(20), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetNameOf", map[string]any{
		"person": map[string]any{"name": "Marco"},
	})
	suite.Equal("Marco", output["name"])
	suite.Equal(engineio.NewFuel(20), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestDoubleAge() {
	consumed, output, except := suite.Call("DoubleAge", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal(map[string]any{"name": "Joe", "age": uint64(80), "gender": "male"}, output["person"])
	suite.Equal(engineio.NewFuel(65), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("DoubleAge", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26},
	})
	suite.Equal(map[string]any{"name": "Marco", "age": uint64(52)}, output["person"])
	suite.Equal(engineio.NewFuel(65), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestPersonStorage() {
	consumed, _, except := suite.Call("StorePerson!", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal(engineio.NewFuel(185), consumed)
	suite.Nil(except)

	consumed, output, except := suite.Call("GetPerson", map[string]any{"name": "Joe"})
	suite.Equal(map[string]any{"name": "Joe", "age": uint64(40), "gender": "male"}, output["person"])
	suite.Equal(engineio.NewFuel(70), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetPerson", map[string]any{"name": "Marco"})
	suite.Equal(map[string]any{}, output["person"])
	suite.Equal(engineio.NewFuel(70), consumed)
	suite.Nil(except)

	consumed, _, except = suite.Call("StorePerson!", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26, "gender": "male"},
	})
	suite.Equal(engineio.NewFuel(185), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetPerson", map[string]any{"name": "Marco"})
	suite.Equal(map[string]any{"name": "Marco", "age": uint64(26), "gender": "male"}, output["person"])
	suite.Equal(engineio.NewFuel(70), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestRenamePerson() {
	consumed, output, except := suite.Call("RenamePerson", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26, "gender": "male"},
		"name":   "Carlo",
	})
	suite.Equal(map[string]any{"name": "Carlo", "age": uint64(26), "gender": "male"}, output["res"])
	suite.Equal(engineio.NewFuel(135), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("RenamePerson", map[string]any{
		"person": map[string]any{"name": "Linda", "age": 45, "gender": "female"},
		"name":   "Elle",
	})
	suite.Equal(map[string]any{"name": "Elle", "age": uint64(45), "gender": "female"}, output["res"])
	suite.Equal(engineio.NewFuel(135), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestCheckNameAlpha() {
	consumed, output, except := suite.Call("CheckNameAlpha", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26, "gender": "male"},
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.NewFuel(170), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("CheckNameAlpha", map[string]any{
		"person": map[string]any{"name": "Sam1", "age": 26, "gender": "male"},
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.NewFuel(170), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestCheckPersonAge() {
	consumed, output, except := suite.Call("CheckPersonAge", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26, "gender": "male"},
	})
	suite.Equal(true, output["ok"])
	suite.Equal(engineio.NewFuel(145), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("CheckPersonAge", map[string]any{
		"person": map[string]any{"name": "Sam1", "age": 10, "gender": "male"},
	})
	suite.Equal(false, output["ok"])
	suite.Equal(engineio.NewFuel(145), consumed)
	suite.Nil(except)
}
