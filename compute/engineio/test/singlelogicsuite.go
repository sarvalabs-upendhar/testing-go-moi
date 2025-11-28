package test

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"

	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-polo"
)

type Logic struct {
	LogicID  identifiers.Identifier
	Manifest engineio.Manifest
	Actors   []identifiers.Identifier
}

type SingleLogicSuite struct {
	suite.Suite
	TestLogicInstance
}

var defaultLimit = engineio.FuelGauge{
	Compute: 500_000,
	Storage: 1_000_000,
}

const (
	defaultTimestamp = 915492840
	defaultClusterID = "test"
)

type (
	testSuiteOptionCleaner func(suite *TestLogicInstance)
	TestSuiteOption        func(suite *TestLogicInstance) error
)

func (suite *SingleLogicSuite) Initialise(
	kind engineio.EngineKind,
	as engineio.AssetEngine,
	manifest engineio.Manifest,
	actors ...identifiers.Identifier,
) (*engineio.FuelGauge, error) {
	suite.ti = suite.T()

	return suite.initialise(kind, as, Logic{
		LogicID:  identifiers.RandomLogicIDv0().AsIdentifier(),
		Manifest: manifest,
		Actors:   actors,
	})
}

func (suite *SingleLogicSuite) SetupTest() {
	suite.setupTest()
}

func (suite *SingleLogicSuite) TearDownTest() {
	suite.tearDownTest()
}

func (suite *SingleLogicSuite) Deploy(
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]int,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(suite.defaultLogicID, common.IxLogicDeploy, callsite, input, access, output, failure, opts...)
}

func (suite *SingleLogicSuite) Enlist(
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]int,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(suite.defaultLogicID, common.IxLogicEnlist, callsite, input, access, output, failure, opts...)
}

func (suite *SingleLogicSuite) Invoke(
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]int,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(suite.defaultLogicID, common.IxLogicInvoke, callsite, input, access, output, failure, opts...)
}

func (suite *SingleLogicSuite) CheckActorStorage(id identifiers.Identifier, key [32]byte, val any) {
	suite.checkActorStorage(id, suite.defaultLogicID, key, val)
}

func (suite *SingleLogicSuite) CheckLogicStorage(key [32]byte, val any) {
	suite.checkStorage(suite.logic[suite.defaultLogicID], suite.defaultLogicID, key, val)
}

func (suite *SingleLogicSuite) DocGen(values map[string]any) polo.Document {
	return DocGen(suite.T(), values)
}

func UseSender(id identifiers.Identifier) TestSuiteOption {
	return func(li *TestLogicInstance) error {
		// If the address is already the current default sender, skip and return
		if id == li.defaultSender {
			return nil
		}

		// Check if the address already has an active state driver
		if _, ok := li.participants[id]; !ok {
			// Create a new state driver
			driver := newDebugStateDriver(li.ti, id)
			// Attach it to the active and snapped states
			li.participants[id] = driver
			li.snapshots[id] = driver
		}

		// Capture the original sender address
		original := li.defaultSender
		// Replace the default sender with the given address
		li.defaultSender = id

		// Set up a cleaner that returns the sender address to the original
		li.cleaners = append(li.cleaners, func(dirty *TestLogicInstance) {
			dirty.defaultSender = original
		})

		return nil
	}
}

func UseFuel(fuel engineio.FuelGauge) TestSuiteOption {
	return func(li *TestLogicInstance) error {
		// set new default fuel
		li.defaultFuel = fuel
		// add a cleaner to reset the default fuel to its previous value
		li.cleaners = append(li.cleaners, func(dirty *TestLogicInstance) {
			dirty.defaultFuel = defaultLimit
		})

		return nil
	}
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
