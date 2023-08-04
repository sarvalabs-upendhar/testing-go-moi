package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

func TestManifestCompiler(t *testing.T) {
	// Create a new runtime object
	runtime := NewRuntime()
	// Register the PISA runtime (required for manifest element decoding)
	engineio.RegisterEngineRuntime(runtime)

	t.Run("erc20_manifest", func(t *testing.T) {
		manifest, err := engineio.ReadManifestFile("../manifests/erc20.yaml")
		require.NoError(t, err)

		compiler, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		require.NoError(t, err)

		descriptor, err := compiler.compile()
		require.NoError(t, err)

		assert.Equal(t, engineio.PISA, descriptor.Engine)
		assert.Equal(t, must(manifest.Hash()), descriptor.ManifestHash)
		assert.Equal(t, false, descriptor.Interactive)

		assert.Equal(t, true, descriptor.StateMatrix.Persistent())
		assert.Equal(t, false, descriptor.StateMatrix.Ephemeral())
		assert.Equal(t, engineio.ElementPtr(0), descriptor.StateMatrix[engineio.PersistentState])

		assert.Equal(t, map[string]*engineio.Classdef{}, descriptor.Classdefs)
		assert.Equal(t, map[string]*engineio.Callsite{
			"Seeder!":     {Ptr: 1, Kind: engineio.DeployerCallsite},
			"Name":        {Ptr: 4, Kind: engineio.InvokableCallsite},
			"Symbol":      {Ptr: 5, Kind: engineio.InvokableCallsite},
			"Decimals":    {Ptr: 6, Kind: engineio.InvokableCallsite},
			"TotalSupply": {Ptr: 7, Kind: engineio.InvokableCallsite},
			"BalanceOf":   {Ptr: 8, Kind: engineio.InvokableCallsite},
			"Allowance":   {Ptr: 9, Kind: engineio.InvokableCallsite},
			"Approve!":    {Ptr: 10, Kind: engineio.InvokableCallsite},
			"Transfer!":   {Ptr: 11, Kind: engineio.InvokableCallsite},
			"Mint!":       {Ptr: 12, Kind: engineio.InvokableCallsite},
			"Burn!":       {Ptr: 13, Kind: engineio.InvokableCallsite},
		}, descriptor.Callsites)
	})

	t.Run("person_manifest", func(t *testing.T) {
		manifest, err := engineio.ReadManifestFile("../manifests/person.yaml")
		require.NoError(t, err)

		compiler, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		require.NoError(t, err)

		descriptor, err := compiler.compile()
		require.NoError(t, err)

		assert.Equal(t, engineio.PISA, descriptor.Engine)
		assert.Equal(t, must(manifest.Hash()), descriptor.ManifestHash)

		assert.Equal(t, false, descriptor.Interactive)
		assert.Equal(t, true, descriptor.StateMatrix.Persistent())
		assert.Equal(t, false, descriptor.StateMatrix.Ephemeral())
		assert.Equal(t, engineio.ElementPtr(0), descriptor.StateMatrix[engineio.PersistentState])

		assert.Equal(t,
			map[string]*engineio.Classdef{
				"Person": {Ptr: 1},
			},
			descriptor.Classdefs,
		)
		assert.Equal(t, map[string]*engineio.Callsite{
			"Setup!":         {Ptr: 10, Kind: engineio.DeployerCallsite},
			"StorePerson!":   {Ptr: 2, Kind: engineio.InvokableCallsite},
			"GetPerson":      {Ptr: 3, Kind: engineio.InvokableCallsite},
			"GetNameOf":      {Ptr: 4, Kind: engineio.InvokableCallsite},
			"DoubleAge":      {Ptr: 5, Kind: engineio.InvokableCallsite},
			"RenamePerson":   {Ptr: 7, Kind: engineio.InvokableCallsite},
			"CheckNameAlpha": {Ptr: 9, Kind: engineio.InvokableCallsite},
			"CheckPersonAge": {Ptr: 12, Kind: engineio.InvokableCallsite},
		}, descriptor.Callsites)
	})

	t.Run("duplicate_element_ptrs", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Kind: StateElement, Data: &StateSchema{}},
				{Ptr: 0, Kind: RoutineElement, Data: &RoutineSchema{}},
			},
		}

		_, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		assert.EqualError(t, err, "invalid manifest: duplicate element pointer [0x0]")
	})

	t.Run("gapped_element_ptrs", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Kind: StateElement, Data: &StateSchema{}},
				{Ptr: 3, Kind: RoutineElement, Data: &RoutineSchema{}},
			},
		}

		_, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		assert.EqualError(t, err, "invalid manifest: element pointer gaps detected")
	})

	t.Run("circular_dependency", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Deps: []engineio.ElementPtr{1}, Kind: StateElement, Data: &StateSchema{}},
				{Ptr: 1, Deps: []engineio.ElementPtr{0}, Kind: RoutineElement, Data: &RoutineSchema{}},
			},
		}

		compiler, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		require.NoError(t, err)

		_, err = compiler.compile()
		assert.EqualError(t, err, "invalid manifest: circular/empty dependency detected")
	})

	t.Run("invalid_element_kind", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Kind: StateElement, Data: &StateSchema{}},
				{Ptr: 1, Kind: engineio.ElementKind("uranium"), Data: &RoutineSchema{}},
			},
		}

		compiler, err := newManifestCompiler(engineio.NewFuel(500), manifest, runtime.instructs)
		require.NoError(t, err)

		_, err = compiler.compile()
		assert.EqualError(t, err, "invalid element kind [0x1]: uranium")
	})

	t.Run("fuel_depleted_dependency_resolution", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Kind: StateElement, Data: &StateSchema{}},
				{Ptr: 1, Deps: []engineio.ElementPtr{0}, Kind: RoutineElement, Data: &RoutineSchema{}},
			},
		}

		compiler, err := newManifestCompiler(engineio.NewFuel(20), manifest, runtime.instructs)
		require.NoError(t, err)

		_, err = compiler.compile()
		assert.EqualError(t, err, "insufficient fuel for manifest compile")
	})

	t.Run("fuel_depleted_compilation", func(t *testing.T) {
		manifest := &engineio.Manifest{
			Syntax: "0.1.0",
			Engine: engineio.ManifestEngineSpec{Kind: string(engineio.PISA)},
			Elements: []engineio.ManifestElement{
				{Ptr: 0, Kind: ConstantElement, Data: &ConstantSchema{"u64", "0x030a0b"}},
				{Ptr: 1, Kind: RoutineElement, Data: &RoutineSchema{
					Name:     "Hello",
					Kind:     engineio.InvokableCallsite,
					Executes: InstructionsSchema{Hex: "0x110000100000"},
				}},
			},
		}

		compiler, err := newManifestCompiler(engineio.NewFuel(70), manifest, runtime.instructs)
		require.NoError(t, err)

		_, err = compiler.compile()
		assert.EqualError(t, err, "insufficient fuel for manifest compile")
	})
}
