package engineio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterEngineRuntime(t *testing.T) {
	// register a mock engine runtime
	mock := &mockEngineRuntime{kind: PISA}
	RegisterEngineRuntime(mock)

	// verify that the engine runtime was registered correctly
	r, ok := FetchEngineRuntime(PISA)
	require.True(t, ok, "Expected engine runtime to be registered")
	require.Equal(t, r.Kind(), PISA, "Expected registered engine runtime to have kind %s, but got %s", PISA, r.Kind())

	// overwrite the mock engine runtime with a new one
	mock2 := &mockEngineRuntime{kind: PISA}
	RegisterEngineRuntime(mock2)

	// verify that the new engine runtime was registered correctly
	r, ok = FetchEngineRuntime(PISA)
	require.True(t, ok, "Expected engine runtime to be registered")
	require.Equal(t, r.Kind(), PISA, "Expected registered engine runtime to have kind %s, but got %s", PISA, r.Kind())

	require.False(t, r.(*mockEngineRuntime) != mock2, "Expected engine runtime to be overwritten") //nolint:forcetypeassert
}

func TestFetchEngineRuntime(t *testing.T) {
	// fetch an unregistered engine runtime
	_, ok := FetchEngineRuntime(MERU)
	require.False(t, ok, "Expected engine runtime not to be registered")

	// register a mock engine runtime
	mock := &mockEngineRuntime{kind: MERU}
	RegisterEngineRuntime(mock)

	// fetch the registered engine runtime
	r, ok := FetchEngineRuntime(MERU)
	require.True(t, ok, "Expected engine runtime to be registered")
	require.Equal(t, r.Kind(), MERU, "Expected registered engine runtime to have kind %s, but got %s", MERU, r.Kind())
}

// mock EngineRuntime implementation for testing
type mockEngineRuntime struct {
	kind EngineKind
}

func (m *mockEngineRuntime) Kind() EngineKind {
	return m.kind
}

func (m *mockEngineRuntime) SpawnEngine(_ Fuel, _ LogicDriver, _ CtxDriver, _ EnvDriver) (Engine, error) {
	return nil, nil
}

func (m *mockEngineRuntime) CompileManifest(_ Fuel, _ *Manifest) (*LogicDescriptor, Fuel, error) {
	return nil, NewFuel(0), nil
}

func (m *mockEngineRuntime) ValidateCalldata(_ LogicDriver, _ *IxnObject) error {
	return nil
}

func (m *mockEngineRuntime) GetElementGenerator(_ ElementKind) (ManifestElementGenerator, bool) {
	return nil, false
}

func (m *mockEngineRuntime) GetCallEncoder(_ *Callsite, _ LogicDriver) (CallEncoder, error) {
	return nil, nil
}
