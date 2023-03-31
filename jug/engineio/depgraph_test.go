package engineio

import (
	"testing"

	mapset "github.com/deckarep/golang-set"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

type node struct {
	ptr  ElementPtr
	deps []ElementPtr
}

func TestDependencyGraph(t *testing.T) {
	tests := []struct {
		inputs []node
		str    string
		size   uint64
		iter   []ElementPtr

		resolves bool
		resolved []ElementPtr
		batches  [][]ElementPtr
	}{
		{
			[]node{
				{0, nil},
				{1, []ElementPtr{0, 2, 4}},
				{2, nil},
				{3, nil},
				{4, nil},
				{5, nil},
				{6, nil},
				{7, nil},
				{8, []ElementPtr{7, 5}},
				{9, []ElementPtr{0, 4, 5}},
				{10, []ElementPtr{0, 6}},
			},
			"DependencyGraph{0, 1:[0 2 4], 2, 3, 4, 5, 6, 7, 8:[5 7], 9:[0 4 5], 10:[0 6]}",
			11,
			[]ElementPtr{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			true,
			[]ElementPtr{0, 2, 3, 4, 5, 6, 7, 1, 8, 9, 10},
			[][]ElementPtr{{0, 2, 3, 4, 5, 6, 7}, {1, 8, 9, 10}},
		},
		{
			[]node{
				{0, nil},
				{1, nil},
				{2, []ElementPtr{3}},
				{3, []ElementPtr{2}},
				{4, nil},
				{5, nil},
			},
			"DependencyGraph{0, 1, 2:[3], 3:[2], 4, 5}",
			6,
			[]ElementPtr{0, 1, 2, 3, 4, 5},
			false,
			nil,
			nil,
		},
		{
			[]node{
				{0, nil},
				{1, []ElementPtr{3}},
				{2, []ElementPtr{1}},
				{3, []ElementPtr{2}},
			},
			"DependencyGraph{0, 1:[3], 2:[1], 3:[2]}",
			4,
			[]ElementPtr{0, 1, 2, 3},
			false,
			nil,
			nil,
		},
	}

	for _, test := range tests {
		dgraph := NewDependencyGraph()
		require.Equal(t, &DependencyGraph{graph: make(map[ElementPtr]mapset.Set)}, dgraph)

		for _, input := range test.inputs {
			dgraph.Insert(input.ptr, input.deps...)
		}

		require.Equal(t, dgraph, dgraph.Copy())
		require.Equal(t, test.size, dgraph.Size())
		require.Equal(t, test.str, dgraph.String())

		ptrs := dgraph.Iter()
		for _, iter := range test.iter {
			require.Equal(t, iter, <-ptrs)
		}

		batches, ok := dgraph.ResolveBatches()
		require.Equal(t, test.resolves, ok)

		resolved, ok := dgraph.Resolve()
		require.Equal(t, test.resolves, ok)

		if !test.resolves {
			continue
		}

		require.Equal(t, test.resolved, resolved)
		require.Equal(t, test.batches, batches)
	}
}

func TestDependencyGraph_AllDependencies(t *testing.T) {
	inputs := []node{
		{0, nil},
		{1, []ElementPtr{0, 2, 4, 8}},
		{2, nil},
		{3, nil},
		{4, nil},
		{5, nil},
		{6, nil},
		{7, nil},
		{8, []ElementPtr{7, 5, 9}},
		{9, []ElementPtr{0, 4, 5}},
		{10, []ElementPtr{0, 6}},
	}

	dgraph := NewDependencyGraph()
	for _, input := range inputs {
		dgraph.Insert(input.ptr, input.deps...)
	}

	deps := dgraph.AllDependencies(1)
	require.Equal(t, []ElementPtr{0, 2, 4, 5, 7, 8, 9}, deps)
}

func TestDependencyGraph_Serialization(t *testing.T) {
	inputs := []node{
		{0, nil},
		{1, []ElementPtr{0, 2, 4, 8}},
		{2, nil},
		{3, nil},
		{4, nil},
		{5, nil},
		{6, nil},
		{7, nil},
		{8, []ElementPtr{7, 5, 9}},
		{9, []ElementPtr{0, 4, 5}},
		{10, []ElementPtr{0, 6}},
	}

	dgraph := NewDependencyGraph()
	for _, input := range inputs {
		dgraph.Insert(input.ptr, input.deps...)
	}

	encoded, err := polo.Polorize(dgraph)
	require.Nil(t, err)

	decoded := new(DependencyGraph)
	err = polo.Depolorize(decoded, encoded)
	require.Nil(t, err)

	require.Equal(t, dgraph, decoded)
}
