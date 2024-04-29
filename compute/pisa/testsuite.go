package pisa

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
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa/exception"
	"github.com/sarvalabs/go-polo"
)

type TestSuite struct {
	suite.Suite
	X engineio.EngineInstance

	logic   engineio.LogicDriver
	logicID identifiers.LogicID

	snapLogic  engineio.StateDriver
	snapSender engineio.StateDriver

	fuelDefault uint64
	fuelCounter uint64

	cleaners []TestSuiteOptCleaner
}

const (
	defaultFuelLimit = 50000
	defaultTimestamp = 915492840
	defaultClusterID = "test"
)

type (
	TestSuiteOptCleaner func(suite *TestSuite)
	TestSuiteCallOption func(suite *TestSuite) error
)

func (suite *TestSuite) Initialise(manifest engineio.Manifest, sender identifiers.Address) (uint64, error) {
	// Create a manifest compiler
	compiler := NewManifestCompiler(defaultFuelLimit, manifest)
	// Compile a logic descriptor from the manifest
	descriptor, consumed, err := compiler.CompileDescriptor()
	if err != nil {
		return consumed, err
	}

	// Create an address for the logic
	address := identifiers.NewRandomAddress()
	// Create a logic object from the
	// descriptor and assign the logic ID
	suite.logic = engineio.NewDebugLogicDriver(suite.T(), address, descriptor)
	suite.logicID = suite.logic.LogicID()
	suite.fuelDefault = defaultFuelLimit

	// Create a debug state driver for the logic's local state
	suite.snapLogic = engineio.NewDebugStateDriver(suite.T(), address, suite.logicID)

	// If a sender is not given, create a new address
	if sender == identifiers.NilAddress {
		sender = identifiers.NewRandomAddress()
	}

	// Create a state driver for the sender address
	suite.snapSender = engineio.NewDebugStateDriver(suite.T(), sender, suite.logicID)

	// Create a PISA instance and assign
	suite.X, err = NewEngine().SpawnInstance(
		suite.logic, defaultFuelLimit, suite.snapLogic,
		engineio.NewDebugEnvDriver(suite.T(), defaultTimestamp, defaultClusterID),
	)

	return consumed, err
}

func (suite *TestSuite) SetupTest() {
	//nolint:forcetypeassert
	if driver := suite.X.(*Instance).internal.GetPersistentDriver(); driver != nil {
		// If logic state driver is available, take snapshot
		suite.snapLogic = driver.(*State).driver.(*engineio.DebugStateDriver).Copy()
	}

	//nolint:forcetypeassert
	if driver := suite.X.(*Instance).internal.GetSenderEphemeralDriver(); driver != nil {
		// If sender state driver is available, take snapshot
		suite.snapSender = driver.(*State).driver.(*engineio.DebugStateDriver).Copy()
	}

	// Reset fuel counter and default fuel limits
	suite.fuelCounter = 0
	suite.X.(*Instance).internal.FuelReset() //nolint:forcetypeassert
}

func (suite *TestSuite) TearDownTest() {
	// Restore state drivers from snapshot
	// This will erase any changes made to the drivers during the test
	suite.X.(*Instance).internal.SetPersistentDriver(newState(suite.snapLogic))       //nolint:forcetypeassert
	suite.X.(*Instance).internal.SetSenderEphemeralDriver(newState(suite.snapSender)) //nolint:forcetypeassert
}

func (suite *TestSuite) Deploy(
	site string, input polo.Document, output polo.Document,
	expectedCost uint64, expectedErr *exception.Exception,
) {
	// Create a new LogicDeploy ixn
	ixn := engineio.NewDebugIxnDriver(suite.T(),
		common.IxLogicDeploy,
		site, input.Bytes(),
		defaultFuelLimit,
		big.NewInt(1),
	)

	result, err := suite.X.Call(context.Background(), ixn, suite.snapSender)
	if err != nil {
		suite.T().Fatalf("deploy call failed: %v", err)
	}

	suite.check(result, output, expectedCost, expectedErr)
}

func (suite *TestSuite) Invoke(
	site string, input polo.Document, output polo.Document,
	expectedCost uint64, expectedErr *exception.Exception,
	opts ...TestSuiteCallOption,
) {
	// Apply any given call options
	suite.applyOpts(opts...)

	// Create a new LogicInvoke ixn
	ixn := engineio.NewDebugIxnDriver(suite.T(),
		common.IxLogicInvoke,
		site, input.Bytes(),
		suite.fuelDefault,
		big.NewInt(1),
	)

	result, err := suite.X.Call(context.Background(), ixn, suite.snapSender)
	if err != nil {
		suite.T().Fatalf("deploy call failed: %v", err)
	}

	// Check the results
	suite.check(result, output, expectedCost, expectedErr)
	// Cleanup all call option changes
	suite.cleanOpts()
}

// TODO: refactor UseSender option to accept a state driver instead of an address
// This will be necessary for ephemeral state checks when that becomes possible

//nolint:forcetypeassert
func UseSender(address identifiers.Address) TestSuiteCallOption {
	return func(suite *TestSuite) error {
		var original engineio.StateDriver
		// Capture the original state driver for the sender
		if driver := suite.X.(*Instance).internal.GetSenderEphemeralDriver(); driver != nil {
			original = driver.(*State).driver.(*engineio.DebugStateDriver).Copy()
		}

		// Create a replacement state driver for the given address
		suite.snapSender = engineio.NewDebugStateDriver(suite.T(), address, suite.logicID)

		// Set up a cleaner that reverts the state inside the instance back to the original sender
		suite.cleaners = append(suite.cleaners, func(dirty *TestSuite) {
			dirty.snapSender = original
		})

		return nil
	}
}

func UseFuel(fuel uint64) TestSuiteCallOption {
	return func(suite *TestSuite) error {
		suite.fuelDefault = fuel

		suite.cleaners = append(suite.cleaners, func(dirty *TestSuite) {
			dirty.fuelDefault = defaultFuelLimit
		})

		return nil
	}
}

func (suite *TestSuite) CheckStorage(key []byte, val any) {
	content, ok := suite.X.(*Instance).internal.GetPersistentDriver().(*State).driver.GetStorageEntry(key)
	if !ok {
		suite.T().Fatalf("no entry in storage for key")
	}

	// Decode the content from storage
	encoded, err := polo.Polorize(val)
	require.NoError(suite.T(), err)

	require.Equal(suite.T(), encoded, content)
}

func (suite *TestSuite) ShowStorage() {
	var str strings.Builder

	for k, v := range suite.SnapStorage() {
		str.WriteString(fmt.Sprintf(">> %v : %v\n", k, v))
	}

	fmt.Println(str.String())
}

//nolint:forcetypeassert
func (suite *TestSuite) SnapStorage() map[string][]byte {
	driver := suite.X.(*Instance).internal.GetPersistentDriver().(*State).driver.(*engineio.DebugStateDriver).Copy()

	return driver.(*engineio.DebugStateDriver).LogicState()
}

func (suite *TestSuite) DocGen(values map[string]any) polo.Document {
	doc, err := polo.PolorizeDocument(values, polo.DocStructs(), polo.DocStringMaps())
	if err != nil {
		panic(err)
	}

	return doc
}

func (suite *TestSuite) applyOpts(opts ...TestSuiteCallOption) {
	for _, opt := range opts {
		if err := opt(suite); err != nil {
			suite.T().Fatalf("failed to apply call option: %v", err)
		}
	}
}

func (suite *TestSuite) cleanOpts() {
	// Apply all cleaners
	for _, cleaner := range suite.cleaners {
		cleaner(suite)
	}

	// Reset the cleaners buffer
	suite.cleaners = make([]TestSuiteOptCleaner, 0)
}

func (suite *TestSuite) check(
	result engineio.CallResult,
	output polo.Document,
	fuelup uint64,
	err *exception.Exception,
) {
	var decoded *exception.Exception

	if result.Error() != nil {
		decoded = new(exception.Exception)
		require.NoError(suite.T(), polo.Depolorize(decoded, result.Error()))
	}

	if err == nil {
		require.Nil(suite.T(), decoded)
	} else {
		require.Equal(suite.T(), err, decoded)
	}

	if output != nil {
		doc := make(polo.Document)
		require.NoError(suite.T(), polo.Depolorize(&doc, result.Outputs()))
		require.Equal(suite.T(), output, doc)
	}

	suite.fuelCounter += fuelup
	require.Equal(suite.T(), suite.fuelCounter, result.Fuel())
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
