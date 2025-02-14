package flipper

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func TestFlipperTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())
	suite.Run(t, new(FlipperTestSuite))
}

type FlipperTestSuite struct {
	engineio.TestSuite
}

var SeederID = identifiers.RandomParticipantIDv0().AsIdentifier()

func (suite *FlipperTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./flipper.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	_, err = suite.Initialise(engineio.PISA, manifest, SeederID)
	suite.Require().NoErrorf(err, "could not read initialise test")

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(InputSeed{Initial: false}, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Deploy("Seed", calldata, nil, nil)

	// Check the value of the state
	keyState := pisa.GenerateStorageKey(SlotValue)
	suite.CheckPersistentStorage(keyState, false)
}

func (suite *FlipperTestSuite) TestToggle() {
	suite.Invoke("Toggle", nil, nil, nil)

	// Check the value of the state
	keyValue := pisa.GenerateStorageKey(SlotValue)
	suite.CheckPersistentStorage(keyValue, true)
}
