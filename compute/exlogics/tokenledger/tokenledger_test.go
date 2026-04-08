package tokenledger

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio/test"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func TestTokenLedgerTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())
	suite.Run(t, new(TokenLedgerTestSuite))
}

type TokenLedgerTestSuite struct {
	test.SingleLogicSuite
}

var (
	SeederID   = identifiers.RandomParticipantIDv0().AsIdentifier()
	ReceiverID = identifiers.RandomParticipantIDv0().AsIdentifier()

	InitialSeed uint64 = 100000000
)

func (suite *TokenLedgerTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./tokenledger.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	_, err = suite.Initialise(engineio.PISA, nil, manifest, SeederID)
	suite.Require().NoErrorf(err, "could not read initialise test")

	inputs := InputSeed{
		Symbol: "MOI",
		Supply: InitialSeed,
	}

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(inputs, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Deploy("Seed", calldata, nil, nil, nil)

	// Check the balance of the seeder
	keySeederBalance := pisa.GenerateStorageKey(SlotBalances, pisa.MakeMapKey(SeederID))
	suite.CheckLogicStorage(keySeederBalance, InitialSeed)
}

func (suite *TokenLedgerTestSuite) TestTransfer() {
	TransferAmount := uint64(1000)

	// Add incentives for an existing krama ID with valid inputs
	suite.Invoke(
		"Transfer",
		suite.DocGen(map[string]any{
			"amount":   TransferAmount,
			"receiver": ReceiverID,
		}),
		nil, nil, nil,
	)

	// Check the balance of the seeder
	keySeederBalance := pisa.GenerateStorageKey(SlotBalances, pisa.MakeMapKey(SeederID))
	suite.CheckLogicStorage(keySeederBalance, InitialSeed-TransferAmount)

	// Check the balance of the receiver
	keyReceiverBalance := pisa.GenerateStorageKey(SlotBalances, pisa.MakeMapKey(ReceiverID))
	suite.CheckLogicStorage(keyReceiverBalance, TransferAmount)
}
