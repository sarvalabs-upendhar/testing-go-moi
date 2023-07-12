package engineio

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	mapset "github.com/deckarep/golang-set"
	"github.com/sarvalabs/go-polo"
)

// DependencyGraph is a graph construct for managing element dependencies. Vertices represent
// an ElementPtr and Edges represent the directional dependency of between these elements
//
// It is thread-safe and can be resolved into a deterministic slice of elements that represent their
// compilation order, this dependency resolution will fail if a circular or empty dependency is detected.
// It also contains methods for thread-safe iteration and checking membership.
type DependencyGraph struct {
	mutex sync.RWMutex
	graph map[ElementPtr]mapset.Set
}

// NewDependencyGraph generates and returns an empty DependencyGraph
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{graph: make(map[ElementPtr]mapset.Set)}
}

// Insert inserts an ElementPtr as a graph vertex to the DependencyGraph.
// It also accepts a variadic number of dependencies for the pointer and inserts them as edges.
// If the vertex (and subsequently its edges) already exists, it is overwritten.
func (dgraph *DependencyGraph) Insert(ptr ElementPtr, deps ...ElementPtr) {
	// Create a new Set and insert the dependencies into it
	set := mapset.NewSet()
	for _, dep := range deps {
		set.Add(dep)
	}

	// Push the vertex and its edges into the graph
	dgraph.push(ptr, set)
}

// Remove removes an ElementPtr as a graph vertex from the DependencyGraph.
// If such a vertex does not exist, this is a no-op.
func (dgraph *DependencyGraph) Remove(ptr ElementPtr) {
	dgraph.pop(ptr)
}

// Dependencies returns the edges of going out of a given vertex pointer.
// The dependencies are returned as a mapset.Set (cardinality is zero if no dependencies for vertex)
func (dgraph *DependencyGraph) Dependencies(ptr ElementPtr) []ElementPtr {
	depSet, _ := dgraph.peek(ptr)

	deps := make([]ElementPtr, 0, depSet.Cardinality())
	for dep := range depSet.Iter() {
		deps = append(deps, dep.(ElementPtr)) //nolint:forcetypeassert
	}

	return deps
}

// AllDependencies returns all the edges (and edges of edges) for a given vertex pointer.
// It recursively collects all dependencies from each dependency layer and returns them (without duplicates).
// Note: This should only be used if the DependencyGraph can be resolved, otherwise, it will result in an infinite loop.
func (dgraph *DependencyGraph) AllDependencies(ptr ElementPtr) []ElementPtr {
	depSet := mapset.NewSet()

	// Collect all the direct deps of the pointer
	for _, dep := range dgraph.Dependencies(ptr) {
		// Add the direct dep to depSet
		depSet.Add(dep)

		// Recursively collect all sub dependencies
		deeper := dgraph.AllDependencies(dep)
		if len(deeper) == 0 {
			continue
		}

		// Add all sub dependencies to the set
		for _, dep := range deeper {
			depSet.Add(dep)
		}
	}

	// Collect all dependencies (free from duplicates)
	deps := make([]ElementPtr, 0, depSet.Cardinality())
	for dep := range depSet.Iter() {
		deps = append(deps, dep.(ElementPtr)) //nolint:forcetypeassert
	}

	// Sort the dependencies
	sort.Slice(deps, func(i, j int) bool {
		return deps[i] < deps[j]
	})

	return deps
}

// Size returns the number of vertices in the DependencyGraph
func (dgraph *DependencyGraph) Size() uint64 {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	return uint64(len(dgraph.graph))
}

// Contains returns whether a vertex for the given ElementPtr exists in the DependencyGraph
func (dgraph *DependencyGraph) Contains(ptr ElementPtr) bool {
	_, ok := dgraph.peek(ptr)

	return ok
}

// Copy creates a clone of the DependencyGraph and returns it
func (dgraph *DependencyGraph) Copy() *DependencyGraph {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	// Create a DependencyGraph with a graph buffer large enough for all elements in the original
	clone := &DependencyGraph{graph: make(map[ElementPtr]mapset.Set, len(dgraph.graph))}
	// For each vertex pointer, copy its edge dependencies and insert
	for ptr, deps := range dgraph.graph {
		clone.push(ptr, deps.Clone())
	}

	return clone
}

// String implements the Stringer interface for DependencyGraph.
func (dgraph *DependencyGraph) String() string {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	elements := make([]string, 0)

	// Iterate over the graph vertices
	for ptr := range dgraph.Iter() {
		// Get the edge dependencies
		deps, _ := dgraph.peek(ptr)
		// If no edges, just add the pointer value
		if deps.Cardinality() == 0 {
			elements = append(elements, fmt.Sprintf("%v", ptr))

			continue
		}

		// Sort the deps and format element as ptr:[deps]
		depSlice := deps.ToSlice()
		sort.Slice(depSlice, func(i, j int) bool {
			return depSlice[i].(ElementPtr) < depSlice[j].(ElementPtr) //nolint:forcetypeassert
		})

		elements = append(elements, fmt.Sprintf("%v:%v", ptr, depSlice))
	}

	return fmt.Sprintf("DependencyGraph{%v}", strings.Join(elements, ", "))
}

// ResolveBatches attempts to resolve the DependencyGraph into batched element pointers.
// Each batch represents elements that need to compiled before the next batch but are independent of
// each other. The output of graph resolution is deterministic as each batch of pointers is sorted.
//
// Returns a boolean along with the batches indicating if the graph could be resolved.
// Graph resolution fails if there are circular or nil (non-existent) dependencies.
func (dgraph *DependencyGraph) ResolveBatches() ([][]ElementPtr, bool) {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	// Create a working copy of the graph
	working := dgraph.Copy()
	// Initialize the slice of element batches
	batches := make([][]ElementPtr, 0)

	// Iterate until, the working graph has been emptied
	for working.Size() != 0 {
		ready := mapset.NewSet()

		// Accumulate all elements from the working
		// set that have zero unresolved dependencies
		for ptr := range working.Iter() {
			if deps, _ := working.peek(ptr); deps.Cardinality() == 0 {
				ready.Add(ptr)
			}
		}

		// If there are no ready elements, we have an issue
		// Either a circular or nil dependency exists in the graph
		if ready.Cardinality() == 0 {
			return nil, false
		}

		// Remove all the elements that are ready from the working graph
		for item := range ready.Iter() {
			working.pop(item.(ElementPtr)) //nolint:forcetypeassert
		}

		// Remove the dependencies for each element in the working graph that have now been resolved.
		// We calculate the difference for each remaining set of dependency edges compared to the ready set.
		for ptr := range working.Iter() {
			deps, _ := working.peek(ptr)
			working.graph[ptr] = deps.Difference(ready)
		}

		// Accumulate all pointers in the ready set as an element batch
		batch := make([]ElementPtr, 0, ready.Cardinality())
		for item := range ready.Iter() {
			batch = append(batch, item.(ElementPtr)) //nolint:forcetypeassert
		}

		// Sort the element batch (the order within a batch is not
		// important, but we do this to get a deterministic output)
		sort.Slice(batch, func(i, j int) bool {
			return batch[i] < batch[j]
		})

		batches = append(batches, batch)
	}

	return batches, true
}

// Resolve attempts to resolve the DependencyGraph into an ordered slice of element pointers.
// This slice represents the order of element compilation and is always deterministic.
// The output our Resolve is essentially a flattened output of ResolveBatches.
//
// Returns a boolean along with the resolved elements indicating if the graph could be resolved.
// Graph resolution fails if there are circular or nil (non-existent) dependencies.
func (dgraph *DependencyGraph) Resolve() ([]ElementPtr, bool) {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	// Resolve the graph into batched elements
	batches, ok := dgraph.ResolveBatches()
	if !ok {
		return nil, false
	}

	// Flatten the batches into a single slice
	// The output inherits its determinism from ResolveBatches
	resolved := make([]ElementPtr, 0, dgraph.Size())
	for _, batch := range batches {
		resolved = append(resolved, batch...)
	}

	return resolved, true
}

// Iter returns a channel iterator that iterates over the vertices of the DependencyGraph is sorted order.
// This iteration is thread-safe, the graph being immutable during the iteration.
func (dgraph *DependencyGraph) Iter() <-chan ElementPtr {
	ch := make(chan ElementPtr)

	go func() {
		dgraph.mutex.RLock()
		defer dgraph.mutex.RUnlock()

		for _, ptr := range dgraph.sorted() {
			ch <- ptr
		}

		close(ch)
	}()

	return ch
}

// sorted returns the vertices of the DependencyGraph as a sorted slice of ElementPtr
func (dgraph *DependencyGraph) sorted() []ElementPtr {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	sorted := make([]ElementPtr, 0, len(dgraph.graph))
	for ptr := range dgraph.graph {
		sorted = append(sorted, ptr)
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	return sorted
}

// push accepts a vertex as an ElementPtr and its dependency
// edges as a mapset.Set and inserts them into the DependencyGraph
func (dgraph *DependencyGraph) push(ptr ElementPtr, set mapset.Set) {
	dgraph.mutex.Lock()
	defer dgraph.mutex.Unlock()

	dgraph.graph[ptr] = set
}

// pop accepts a vertex as an ElementPtr and removes it from the graph
func (dgraph *DependencyGraph) pop(ptr ElementPtr) {
	dgraph.mutex.Lock()
	defer dgraph.mutex.Unlock()

	delete(dgraph.graph, ptr)
}

// peek returns the edge dependencies for a given vertex as a
// mapset.Set along with a boolean indicating if the vertex existed.
func (dgraph *DependencyGraph) peek(ptr ElementPtr) (mapset.Set, bool) {
	dgraph.mutex.RLock()
	defer dgraph.mutex.RUnlock()

	set, ok := dgraph.graph[ptr]

	return set, ok
}

// encode converts the DependencyGraph into a map of pointers to their dependencies.
// The generated map is safe to encode with any encoding scheme.
func (dgraph *DependencyGraph) encode() map[ElementPtr][]ElementPtr {
	// Declare a map to collect all graph nodes
	encodable := make(map[ElementPtr][]ElementPtr, dgraph.Size())
	// Iterate over the graph vertices
	for ptr := range dgraph.Iter() {
		// Get the edge dependencies
		depSet, _ := dgraph.peek(ptr)

		// Collect the edges into a []ElementPtr
		deps := make([]ElementPtr, 0, depSet.Cardinality())
		for dep := range depSet.Iter() {
			deps = append(deps, dep.(ElementPtr)) //nolint:forcetypeassert
		}

		// Sort the dependencies
		sort.Slice(deps, func(i, j int) bool {
			return deps[i] < deps[j]
		})

		encodable[ptr] = deps
	}

	return encodable
}

// decode converts a given map of pointers to their dependencies into a DependencyGraph and absorbs it.
func (dgraph *DependencyGraph) decode(data map[ElementPtr][]ElementPtr) {
	// Insert each node into the graph
	*dgraph = *NewDependencyGraph()
	for ptr, deps := range data {
		dgraph.Insert(ptr, deps...)
	}
}

// MarshalJSON implements the json.Marshaller interface for DependencyGraph
func (dgraph *DependencyGraph) MarshalJSON() ([]byte, error) {
	// Get encodable version of dgraph
	encodable := dgraph.encode()

	return json.Marshal(encodable)
}

// UnmarshalJSON implements the json.Unmarshaller interface for DependencyGraph
func (dgraph *DependencyGraph) UnmarshalJSON(data []byte) error {
	// Decode the data into a map of graph nodes
	decodable := make(map[ElementPtr][]ElementPtr)
	if err := json.Unmarshal(data, &decodable); err != nil {
		return err
	}

	// Decode data into dgraph
	dgraph.decode(decodable)

	return nil
}

// Polorize implements the polo.Polorizable interface for DependencyGraph
func (dgraph *DependencyGraph) Polorize() (*polo.Polorizer, error) {
	// Get encodable version of dgraph
	encodable := dgraph.encode()

	// Serialize the encodable map
	polorizer := polo.NewPolorizer()
	if err := polorizer.Polorize(encodable); err != nil {
		return nil, err
	}

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for DependencyGraph
func (dgraph *DependencyGraph) Depolorize(depolorizer *polo.Depolorizer) error {
	// Decode the data into a map of graph nodes
	decodable := make(map[ElementPtr][]ElementPtr)
	if err := depolorizer.Depolorize(&decodable); err != nil {
		return err
	}

	// Decode data into dgraph
	dgraph.decode(decodable)

	return nil
}
