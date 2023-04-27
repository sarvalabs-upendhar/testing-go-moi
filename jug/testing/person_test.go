package testing

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
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
	logicID, _ := types.NewLogicIDv0(true, false, false, false, 0, address)

	consumed := suite.Initialize(manifest, logicID, address, 5000)
	suite.Equal(engineio.Fuel(100), consumed)
}

func (suite *PersonTestSuite) TestGetNameOf() {
	consumed, output, except := suite.Call("GetNameOf", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal("Joe", output["name"])
	suite.Equal(engineio.Fuel(20), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetNameOf", map[string]any{
		"person": map[string]any{"name": "Marco"},
	})
	suite.Equal("Marco", output["name"])
	suite.Equal(engineio.Fuel(20), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestDoubleAge() {
	consumed, output, except := suite.Call("DoubleAge", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal(map[string]any{"name": "Joe", "age": uint64(80), "gender": "male"}, output["person"])
	suite.Equal(engineio.Fuel(65), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("DoubleAge", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26},
	})
	suite.Equal(map[string]any{"name": "Marco", "age": uint64(52)}, output["person"])
	suite.Equal(engineio.Fuel(65), consumed)
	suite.Nil(except)
}

func (suite *PersonTestSuite) TestPersonStorage() {
	consumed, _, except := suite.Call("StorePerson!", map[string]any{
		"person": map[string]any{"name": "Joe", "age": 40, "gender": "male"},
	})
	suite.Equal(engineio.Fuel(185), consumed)
	suite.Nil(except)

	consumed, output, except := suite.Call("GetPerson", map[string]any{"name": "Joe"})
	suite.Equal(map[string]any{"name": "Joe", "age": uint64(40), "gender": "male"}, output["person"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetPerson", map[string]any{"name": "Marco"})
	suite.Equal(map[string]any{}, output["person"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)

	consumed, _, except = suite.Call("StorePerson!", map[string]any{
		"person": map[string]any{"name": "Marco", "age": 26, "gender": "male"},
	})
	suite.Equal(engineio.Fuel(185), consumed)
	suite.Nil(except)

	consumed, output, except = suite.Call("GetPerson", map[string]any{"name": "Marco"})
	suite.Equal(map[string]any{"name": "Marco", "age": uint64(26), "gender": "male"}, output["person"])
	suite.Equal(engineio.Fuel(70), consumed)
	suite.Nil(except)
}
