package common

import mapset "github.com/deckarep/golang-set"

// HashRegistry is a cache for known hashes.
type HashRegistry struct {
	hashes mapset.Set
	max    int
}

// NewHashRegistry creates a new HashRegistry with a max capacity.
func NewHashRegistry(capacity int) *HashRegistry {
	return &HashRegistry{
		max:    capacity,
		hashes: mapset.NewSet(),
	}
}

// Add adds a list of elements to the set.
func (k *HashRegistry) Add(data ...interface{}) {
	for k.hashes.Cardinality() > Max(0, k.max-len(data)) {
		k.hashes.Pop()
	}

	for _, hash := range data {
		k.hashes.Add(hash)
	}
}

// Contains returns whether the given item is in the set.
func (k *HashRegistry) Contains(data interface{}) bool {
	return k.hashes.Contains(data)
}

// Cardinality returns the number of elements in the set.
func (k *HashRegistry) Cardinality() int {
	return k.hashes.Cardinality()
}

func Max(a, b int) int {
	if a > b {
		return a
	}

	return b
}
