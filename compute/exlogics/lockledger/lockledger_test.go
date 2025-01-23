package lockledger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func TestLockLedgerTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())
	suite.Run(t, new(LockLedgerTestSuite))
}

type LockLedgerTestSuite struct {
	engineio.TestSuite
}

var (
	SeederID   = identifiers.RandomParticipantIDv0().AsIdentifier()
	ReceiverID = identifiers.RandomParticipantIDv0().AsIdentifier()

	InitialSeed uint64 = 100000000
)

func (suite *LockLedgerTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./lockledger.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	_, err = suite.Initialise(engineio.PISA, manifest, SeederID)
	suite.Require().NoErrorf(err, "could not read initialise test")

	inputs := InputSeed{
		Symbol: "MOI",
		Supply: InitialSeed,
	}

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(inputs, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Deploy("Seed", calldata, nil, nil)

	// Check the balance of the seeder
	keySeederSpendable := pisa.GenerateStorageKey(EphemeralSlotSpendable)
	suite.CheckEphemeralStorage(SeederID, keySeederSpendable, InitialSeed)
}

func (suite *LockLedgerTestSuite) TestLockup() {
	LockupAmount := uint64(1000)

	// Add incentives for an existing krama ID with valid inputs
	suite.Invoke(
		"Lockup",
		must(polo.PolorizeDocument(InputLockup{
			Amount: LockupAmount,
		})),
		nil, nil,
	)

	// Check the spendable balance of the seeder
	keySeederSpendable := pisa.GenerateStorageKey(EphemeralSlotSpendable)
	suite.CheckEphemeralStorage(SeederID, keySeederSpendable, InitialSeed-LockupAmount)

	// Check the lockedup balance of the seeder
	keySeederLockedup := pisa.GenerateStorageKey(EphemeralSlotLockedup)
	suite.CheckEphemeralStorage(SeederID, keySeederLockedup, LockupAmount)
}

func (suite *LockLedgerTestSuite) TestMint() {
	MintAmount := uint64(1000)

	// Add incentives for an existing krama ID with valid inputs
	suite.Invoke(
		"Mint",
		must(polo.PolorizeDocument(InputMint{
			Amount: MintAmount,
		})),
		nil, nil,
	)

	// Check the total supply
	keySupply := pisa.GenerateStorageKey(PersistentSlotSupply)
	suite.CheckPersistentStorage(keySupply, InitialSeed+MintAmount)

	// Check the spendable balance of the seeder
	keySeederSpendable := pisa.GenerateStorageKey(EphemeralSlotSpendable)
	suite.CheckEphemeralStorage(SeederID, keySeederSpendable, InitialSeed+MintAmount)
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}
