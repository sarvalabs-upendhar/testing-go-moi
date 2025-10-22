package test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

type TestLogicInstance struct {
	ti      *testing.T
	x       engineio.Engine
	Runtime engineio.Runtime

	logic     map[identifiers.Identifier]*debugStateDriver
	logicSnap map[identifiers.Identifier]*debugStateDriver

	participants map[identifiers.Identifier]*debugStateDriver
	snapshots    map[identifiers.Identifier]*debugStateDriver

	defaultFuel    engineio.FuelGauge
	defaultLogicID identifiers.Identifier
	defaultSender  identifiers.Identifier
	defaultAccess  map[[32]byte]bool

	transition *debugTransition

	cleaners []testSuiteOptionCleaner
}

func (li *TestLogicInstance) initialise(
	kind engineio.EngineKind,
	as engineio.AssetEngine,
	logics ...Logic,
) (*engineio.FuelGauge, error) {
	// Set up the engine runtime
	engine, ok := engineio.FetchEngine(kind)
	require.True(li.ti, ok, "unsupported engine kind")
	li.x = engine
	li.Runtime = engine.Runtime(uint64(defaultTimestamp))
	li.Runtime.BindAssetEngine(as)
	li.defaultAccess = make(map[[32]byte]bool)
	li.defaultFuel = defaultLimit
	li.transition = newDebugTransition()

	li.logic = make(map[identifiers.Identifier]*debugStateDriver)
	li.participants = make(map[identifiers.Identifier]*debugStateDriver)
	li.snapshots = make(map[identifiers.Identifier]*debugStateDriver)
	li.logicSnap = make(map[identifiers.Identifier]*debugStateDriver)

	consumption := new(engineio.FuelGauge)

	for _, logic := range logics {
		if _, ok = li.logic[logic.LogicID]; ok {
			return nil, fmt.Errorf("logic already initialised")
		}

		// Compile a logic descriptor from the manifest
		artifact, compileEffort, err := li.x.CompileManifest(
			engineio.ManifestKindFromIdentifier(logic.LogicID),
			logic.LogicID,
			logic.Manifest,
			defaultLimit,
		)
		consumption.Add(compileEffort)

		if err != nil {
			return consumption, err
		}

		if li.defaultSender.IsNil() {
			if len(logic.Actors) == 0 {
				li.defaultSender = identifiers.RandomParticipantIDv0().AsIdentifier()
				li.defaultAccess[li.defaultSender] = true

				s := newDebugStateDriver(li.ti, li.defaultSender)
				li.participants[li.defaultSender] = s

				// Add to transition
				li.transition.Entry[li.defaultSender] = s
				require.NoError(li.ti, li.Runtime.CreateActor(li.defaultSender, s, nil))
			} else {
				li.defaultSender = logic.Actors[0]
			}

			li.defaultLogicID = logic.LogicID
		}

		li.logic[logic.LogicID] = newDebugStateDriver(li.ti, logic.LogicID)

		li.defaultAccess[logic.LogicID] = true

		if err = li.Runtime.SpawnLogic(
			logic.LogicID,
			artifact,
			li.logic[logic.LogicID],
			nil); err != nil {
			return consumption, err
		}

		for _, id := range logic.Actors {
			li.defaultAccess[id] = true

			s := newDebugStateDriver(li.ti, id)
			li.participants[id] = s

			// Add to transition
			li.transition.Entry[id] = s
			require.NoError(li.ti, li.Runtime.CreateActor(id, s, nil))
		}
	}

	return consumption, nil
}

func (li *TestLogicInstance) setupTest() {
	// If logic state driver is available, take snapshot
	for id, driver := range li.logic {
		li.logicSnap[id] = driver.Copy()
	}

	// For each known participant, take snapshot
	for id, snap := range li.participants {
		li.snapshots[id] = snap.Copy()
	}
}

func (li *TestLogicInstance) tearDownTest() {
	// Restore state drivers from snapshot
	// This will erase any changes made to the drivers during the test.
	// Restore local state driver from snapshot
	for id, driver := range li.logic {
		driver.Reset(li.logicSnap[id])
	}

	// For each known participant, restore state from snapshot
	for id, snap := range li.snapshots {
		li.participants[id].Reset(snap)
	}
}

func (li *TestLogicInstance) call(
	logicID identifiers.Identifier,
	kind common.IxOpType,
	site string,
	input polo.Document,
	access map[[32]byte]bool,
) engineio.CallResult {
	if len(access) == 0 {
		access = li.defaultAccess
	}

	// Create a new LogicDeploy ixn
	ixn := newDebugTxnDriver(li.ti,
		kind, li.defaultSender, common.Hash{},
		site, input.Bytes(),
		li.defaultFuel.Compute,
		big.NewInt(1),
		access, // default fuel price
	)

	result := li.Runtime.Call(logicID, ixn, li.transition, &li.defaultFuel)

	return *result
}

func (li *TestLogicInstance) applyOpts(opts ...TestSuiteOption) {
	// Apply all test options
	for _, opt := range opts {
		err := opt(li)
		require.NoErrorf(li.ti, err, "failed to apply test option: %v", err)
	}
}

func (li *TestLogicInstance) cleanOpts() {
	// Apply all cleaners
	for _, cleaner := range li.cleaners {
		cleaner(li)
	}

	// Reset the cleaners
	li.cleaners = make([]testSuiteOptionCleaner, 0)
}

func (li *TestLogicInstance) callAndCheck(
	logicID identifiers.Identifier,
	kind common.IxOpType, site string, input polo.Document, access map[[32]byte]bool,
	expectedOut polo.Document, expectedErr *engineio.ErrorResult,
	opts ...TestSuiteOption,
) {
	// Apply Test Options
	li.applyOpts(opts...)

	if len(access) == 0 {
		access = make(map[[32]byte]bool)
	}

	// Perform an IxLogicEnlist call
	result := li.call(logicID, kind, site, input, access)
	// Check the result to match the expected output/error
	li.check(result, expectedOut, expectedErr)

	// Cleanup test options
	li.cleanOpts()
}

func (li *TestLogicInstance) checkActorStorage(
	actorID identifiers.Identifier,
	logicID identifiers.Identifier,
	key [32]byte, val any,
) {
	party := li.participants[actorID]
	if party == nil {
		party = li.logic[actorID]
	}

	require.NotNil(li.ti, party, "participant not found for address")

	li.checkStorage(party, logicID, key, val)
}

func (li *TestLogicInstance) checkStorage(reader *debugStateDriver, logicID [32]byte, key [32]byte, val any) {
	// Get the raw storage content for the key
	content, err := reader.ReadPersistentStorage(logicID, key)
	require.NoError(li.ti, err)

	// Encode the value into its polo bytes
	encoded, err := polo.Polorize(val)
	require.NoError(li.ti, err)

	// Check that the raw content matches the encoded value
	require.Equal(li.ti, encoded, content, "storage mismatch")
}

func (li *TestLogicInstance) check(
	result engineio.CallResult,
	expectedOut polo.Document,
	expectedErr *engineio.ErrorResult,
) {
	if result.IsError() {
		decoded := new(engineio.ErrorResult)

		require.NoError(li.ti, polo.Depolorize(decoded, result.Err))

		require.NotNilf(li.ti, expectedErr, "unexpected error: %v", decoded.Error)
		require.Contains(li.ti, decoded.Error, expectedErr.Error)
	}

	if expectedOut != nil {
		require.Equal(li.ti, expectedOut, result.Out)
	}
}

func DocGen(t *testing.T, values map[string]any) polo.Document {
	t.Helper()

	doc, err := polo.PolorizeDocument(values, polo.DocStructs(), polo.DocStringMaps())
	require.NoError(t, err, "cannot generate document")

	return doc
}
