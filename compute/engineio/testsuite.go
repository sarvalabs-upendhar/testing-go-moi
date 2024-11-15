package engineio

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/drivers"
	"github.com/sarvalabs/go-polo"
)

type TestSuite struct {
	suite.Suite

	x Engine
	X DebugEngineInstance

	logic     debugLogicDriver
	logicSnap *debugStateDriver
	logicID   identifiers.LogicID

	participants map[identifiers.Address]*debugStateDriver
	snapshots    map[identifiers.Address]*debugStateDriver

	defaultFuel   uint64
	defaultSender identifiers.Address

	cleaners []testSuiteOptionCleaner
}

const (
	defaultFuelLimit = 500_000
	defaultTimestamp = 915492840
	defaultClusterID = "test"
)

type (
	testSuiteOptionCleaner func(suite *TestSuite)
	TestSuiteOption        func(suite *TestSuite) error
)

func (suite *TestSuite) Initialise(kind EngineKind, manifest Manifest, sender identifiers.Address) (uint64, error) {
	// Set up the engine runtime
	engine, ok := FetchEngine(kind)
	require.True(suite.T(), ok, "unsupported engine kind")
	suite.x = engine

	// Compile a logic descriptor from the manifest
	descriptor, consumed, err := suite.x.CompileManifest(manifest, defaultFuelLimit)
	if err != nil {
		return consumed, err
	}

	// Create an address for the logic
	address := identifiers.NewRandomAddress()
	// Create a logic object from the
	// descriptor and assign the logic ID
	suite.logic = newDebugLogicDriver(suite.T(), address, descriptor)
	suite.logicID = suite.logic.LogicID()

	// If a sender is not given, create a new address
	if sender == identifiers.NilAddress {
		sender = identifiers.NewRandomAddress()
	}

	// Set defaults
	suite.defaultSender = sender
	suite.defaultFuel = defaultFuelLimit

	// Initialise state maps
	suite.participants = make(map[identifiers.Address]*debugStateDriver)
	suite.snapshots = make(map[identifiers.Address]*debugStateDriver)

	// Create a driver for the logic's local state
	suite.logicSnap = newDebugStateDriver(suite.T(), address, suite.logicID)
	// Create a driver for the sender's state
	suite.participants[sender] = newDebugStateDriver(suite.T(), sender, suite.logicID)

	// Create a PISA instance and assign
	suite.X, err = suite.x.SpawnDebugInstance(
		suite.logic,
		defaultFuelLimit,
		suite.logicSnap,
		newDebugEnvDriver(suite.T(), defaultTimestamp, defaultClusterID),
		newDebugEventDriver(suite.T(), suite.logicID),
	)

	return consumed, err
}

func (suite *TestSuite) SetupTest() {
	suite.X.FuelReset()

	// If logic state driver is available, take snapshot
	if driver := suite.X.GetLocalDriver(); driver != nil {
		suite.logicSnap = driver.(*debugStateDriver).Copy() //nolint:forcetypeassert
	}

	// For each known participant, take snapshot
	for addr, snap := range suite.participants {
		suite.snapshots[addr] = snap.Copy()
	}
}

func (suite *TestSuite) TearDownTest() {
	// Restore state drivers from snapshot
	// This will erase any changes made to the drivers during the test.
	// Restore local state driver from snapshot
	suite.X.SetLocalDriver(suite.logicSnap)

	// For each known participant, restore state from snapshot
	for addr, snap := range suite.snapshots {
		suite.participants[addr] = snap
	}
}

func (suite *TestSuite) Deploy(
	callsite string, input polo.Document,
	output polo.Document, failure ErrorResult,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(common.IxLogicDeploy, callsite, input, output, failure, opts...)
}

func (suite *TestSuite) Enlist(
	callsite string, input polo.Document,
	output polo.Document, failure ErrorResult,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(common.IxLogicEnlist, callsite, input, output, failure, opts...)
}

func (suite *TestSuite) Invoke(
	callsite string, input polo.Document,
	output polo.Document, failure ErrorResult,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(common.IxLogicInvoke, callsite, input, output, failure, opts...)
}

func (suite *TestSuite) CheckEphemeralStorage(addr identifiers.Address, key []byte, val any) {
	party, ok := suite.participants[addr]
	require.True(suite.T(), ok, "participant not found for address")

	suite.checkStorage(party, key, val)
}

func (suite *TestSuite) CheckPersistentStorage(key []byte, val any) {
	local := suite.X.GetLocalDriver()
	suite.checkStorage(local, key, val)
}

func (suite *TestSuite) CheckEventStream(index uint64, event drivers.Event) {
	log, ok := suite.X.GetEventDriver().Fetch(index)

	require.True(suite.T(), ok, "event not found")
	require.Equal(suite.T(), event, log, "event mismatch")
}

func (suite *TestSuite) DocGen(values map[string]any) polo.Document {
	doc, err := polo.PolorizeDocument(values, polo.DocStructs(), polo.DocStringMaps())
	require.NoError(suite.T(), err, "cannot generate document")

	return doc
}

func UseSender(addr identifiers.Address) TestSuiteOption {
	return func(suite *TestSuite) error {
		// If the address is already the current default sender, skip and return
		if addr == suite.defaultSender {
			return nil
		}

		// Check if the address already has an active state driver
		if _, ok := suite.participants[addr]; !ok {
			// Create a new state driver
			driver := newDebugStateDriver(suite.T(), addr, suite.logicID)
			// Attach it to the active and snapped states
			suite.participants[addr] = driver
			suite.snapshots[addr] = driver
		}

		// Capture the original sender address
		original := suite.defaultSender
		// Replace the default sender with the given address
		suite.defaultSender = addr

		// Set up a cleaner that returns the sender address to the original
		suite.cleaners = append(suite.cleaners, func(dirty *TestSuite) {
			dirty.defaultSender = original
		})

		return nil
	}
}

func UseFuel(fuel uint64) TestSuiteOption {
	return func(suite *TestSuite) error {
		// set new default fuel
		suite.defaultFuel = fuel
		// add a cleaner to reset the default fuel to its previous value
		suite.cleaners = append(suite.cleaners, func(dirty *TestSuite) {
			dirty.defaultFuel = defaultFuelLimit
		})

		return nil
	}
}

func (suite *TestSuite) callAndCheck(
	kind common.IxOpType, site string, input polo.Document,
	expectedOut polo.Document, expectedErr ErrorResult,
	opts ...TestSuiteOption,
) {
	// Apply Test Options
	suite.applyOpts(opts...)

	// Perform an IxLogicEnlist call
	result := suite.call(kind, site, input)
	// Check the result to match the expected output/error
	suite.check(result, expectedOut, expectedErr)

	// Cleanup test options
	suite.cleanOpts()
}

func (suite *TestSuite) sender() StateDriver {
	return suite.participants[suite.defaultSender]
}

func (suite *TestSuite) call(kind common.IxOpType, site string, input polo.Document) CallResult {
	// Create a new LogicDeploy ixn
	ixn := newDebugTxnDriver(suite.T(),
		kind, common.Hash{},
		site, input.Bytes(),
		suite.defaultFuel,
		big.NewInt(1), // default fuel price
	)

	result, err := suite.X.Call(context.Background(), ixn, suite.sender())
	require.NoErrorf(suite.T(), err, "call failed: %v", err)

	return result
}

func (suite *TestSuite) check(result CallResult, expectedOut polo.Document, expectedErr ErrorResult) {
	if result.Error() != nil {
		require.NotNilf(suite.T(), expectedErr, "unexpected error: %v", expectedErr)

		decoded := new(pisa.ErrorResult)
		require.NoError(suite.T(), polo.Depolorize(decoded, result.Error()))
		require.Equal(suite.T(), expectedErr.String(), decoded.String())
	}

	if expectedOut != nil {
		doc := make(polo.Document)
		require.NoError(suite.T(), polo.Depolorize(&doc, result.Outputs()))
		require.Equal(suite.T(), expectedOut, doc)
	}
}

func (suite *TestSuite) checkStorage(reader StorageReader, key []byte, val any) {
	// Get the raw storage content for the key
	content, err := reader.GetStorageEntry(key)
	require.NoError(suite.T(), err)

	// Encode the value into its polo bytes
	encoded, err := polo.Polorize(val)
	require.NoError(suite.T(), err)

	// Check that the raw content matches the encoded value
	require.Equal(suite.T(), encoded, content, "storage mismatch")
}

func (suite *TestSuite) applyOpts(opts ...TestSuiteOption) {
	// Apply all test options
	for _, opt := range opts {
		err := opt(suite)
		require.NoErrorf(suite.T(), err, "failed to apply test option: %v", err)
	}
}

func (suite *TestSuite) cleanOpts() {
	// Apply all cleaners
	for _, cleaner := range suite.cleaners {
		cleaner(suite)
	}

	// Reset the cleaners
	suite.cleaners = make([]testSuiteOptionCleaner, 0)
}

func AnalyseStorageDiff(o, n map[string][]byte) {
	// Declare a map to collect all known keys
	// A map allows us to remove duplicates intrinsically
	allkeys := make(map[string]struct{})
	// Collect keys from old
	for k := range o {
		allkeys[k] = struct{}{}
	}
	// Collect keys from new
	for k := range n {
		allkeys[k] = struct{}{}
	}

	var result strings.Builder

	for key := range allkeys {
		ov, oe := o[key]
		nv, ne := n[key]

		switch {
		case oe && !ne:
			// key existed but has been removed
			result.WriteString(fmt.Sprintf(">> %v : Removed\n", key))

		case !oe && ne:
			// key has been inserted
			result.WriteString(fmt.Sprintf(">> %v : Inserted\n", key))

		case oe && ne:
			// key existed and continues to exist
			if bytes.Equal(ov, nv) {
				result.WriteString(fmt.Sprintf(">> %v : Untouched\n", key))
			} else {
				result.WriteString(fmt.Sprintf(">> %v : Modified\n", key))
				result.WriteString(fmt.Sprintf(">>> %v -> %v\n", ov, nv))
			}

		default:
			panic("error")
		}
	}

	fmt.Println(result.String())
}
