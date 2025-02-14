package toggler

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func TestTogglerTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())
	suite.Run(t, new(TogglerTestSuite))
}

type TogglerTestSuite struct {
	engineio.TestSuite
}

var SeederID = identifiers.RandomParticipantIDv0().AsIdentifier()

func (suite *TogglerTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./toggler.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	_, err = suite.Initialise(engineio.PISA, manifest, SeederID)
	suite.Require().NoErrorf(err, "could not read initialise test")

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(InputSeed{Initial: false}, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Enlist("Seed", calldata, nil, nil)

	// Check the balance of the seeder
	keyValue := pisa.GenerateStorageKey(SlotValue)
	suite.CheckEphemeralStorage(SeederID, keyValue, false)
}

func (suite *TogglerTestSuite) TestToggle() {
	suite.Invoke("Toggle", nil, nil, nil)

	// Check the balance of the seeder
	keyValue := pisa.GenerateStorageKey(SlotValue)
	suite.CheckEphemeralStorage(SeederID, keyValue, true)
}
