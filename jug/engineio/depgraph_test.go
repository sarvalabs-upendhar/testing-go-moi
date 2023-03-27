package engineio

import (
	"testing"

	mapset "github.com/deckarep/golang-set"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

type node struct {
	ptr  uint64
	deps []uint64
}

func TestDependencyGraph(t *testing.T) {
	tests := []struct {
		inputs []node
		str    string
		size   uint64
		iter   []uint64

		resolves bool
		resolved []uint64
		batches  [][]uint64
	}{
		{
			[]node{
				{0, nil},
				{1, []uint64{0, 2, 4}},
				{2, nil},
				{3, nil},
				{4, nil},
				{5, nil},
				{6, nil},
				{7, nil},
				{8, []uint64{7, 5}},
				{9, []uint64{0, 4, 5}},
				{10, []uint64{0, 6}},
			},
			"DependencyGraph{0, 1:[0 2 4], 2, 3, 4, 5, 6, 7, 8:[5 7], 9:[0 4 5], 10:[0 6]}",
			11,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			true,
			[]uint64{0, 2, 3, 4, 5, 6, 7, 1, 8, 9, 10},
			[][]uint64{{0, 2, 3, 4, 5, 6, 7}, {1, 8, 9, 10}},
		},
		{
			[]node{
				{0, nil},
				{1, nil},
				{2, []uint64{3}},
				{3, []uint64{2}},
				{4, nil},
				{5, nil},
			},
			"DependencyGraph{0, 1, 2:[3], 3:[2], 4, 5}",
			6,
			[]uint64{0, 1, 2, 3, 4, 5},
			false,
			nil,
			nil,
		},
		{
			[]node{
				{0, nil},
				{1, []uint64{3}},
				{2, []uint64{1}},
				{3, []uint64{2}},
			},
			"DependencyGraph{0, 1:[3], 2:[1], 3:[2]}",
			4,
			[]uint64{0, 1, 2, 3},
			false,
			nil,
			nil,
		},
	}

	for _, test := range tests {
		dgraph := NewDependencyGraph()
		require.Equal(t, &DependencyGraph{graph: make(map[uint64]mapset.Set)}, dgraph)

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
		{1, []uint64{0, 2, 4, 8}},
		{2, nil},
		{3, nil},
		{4, nil},
		{5, nil},
		{6, nil},
		{7, nil},
		{8, []uint64{7, 5, 9}},
		{9, []uint64{0, 4, 5}},
		{10, []uint64{0, 6}},
	}

	dgraph := NewDependencyGraph()
	for _, input := range inputs {
		dgraph.Insert(input.ptr, input.deps...)
	}

	deps := dgraph.AllDependencies(1)
	require.Equal(t, []uint64{0, 2, 4, 5, 7, 8, 9}, deps)
}

func TestDependencyGraph_Serialization(t *testing.T) {
	inputs := []node{
		{0, nil},
		{1, []uint64{0, 2, 4, 8}},
		{2, nil},
		{3, nil},
		{4, nil},
		{5, nil},
		{6, nil},
		{7, nil},
		{8, []uint64{7, 5, 9}},
		{9, []uint64{0, 4, 5}},
		{10, []uint64{0, 6}},
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
