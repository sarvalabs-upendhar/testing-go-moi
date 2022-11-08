package utils

import (
	"gitlab.com/sarvalabs/moichain/types"
)

type HashSet struct {
	set map[types.Hash]struct{}
}

// NewHashSet initializes and returns a new HashSet.
func NewHashSet() *HashSet {
	return &HashSet{set: make(map[types.Hash]struct{})}
}

// Add puts a Cid in the HashSet.
func (s *HashSet) Add(c types.Hash) {
	s.set[c] = struct{}{}
}

// Has returns if the HashSet contains a given Cid.
func (s *HashSet) Has(c types.Hash) bool {
	_, ok := s.set[c]

	return ok
}

// Remove deletes a hash from the HashSet.
func (s *HashSet) Remove(c types.Hash) {
	delete(s.set, c)
}

// Len returns how many elements the HashSet has.
func (s *HashSet) Len() int {
	return len(s.set)
}

// Keys returns the Hashes in the set.
func (s *HashSet) Keys() []types.Hash {
	out := make([]types.Hash, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}

	return out
}

// Visit adds a Hash to the set only if it is
// not in it already.
func (s *HashSet) Visit(c types.Hash) bool {
	if !s.Has(c) {
		s.Add(c)

		return true
	}

	return false
}

// ForEach allows to run a custom function on each
// Cid in the set.
func (s *HashSet) ForEach(f func(c types.Hash) error) error {
	for c := range s.set {
		err := f(c)
		if err != nil {
			return err
		}
	}

	return nil
}

type Queue struct {
	elems []types.Hash
	set   *HashSet
}

func NewCidQueue() *Queue {
	return &Queue{set: NewHashSet()}
}

func (cq *Queue) Pop() types.Hash {
	for {
		if len(cq.elems) == 0 {
			return types.NilHash
		}

		out := cq.elems[0]
		cq.elems = cq.elems[1:]

		if cq.set.Has(out) {
			cq.set.Remove(out)

			return out
		}
	}
}

func (cq *Queue) Cids() []types.Hash {
	// Lazily delete from the list any cids that were removed from the set
	if len(cq.elems) > cq.set.Len() {
		i := 0

		for _, c := range cq.elems {
			if cq.set.Has(c) {
				cq.elems[i] = c
				i++
			}
		}

		cq.elems = cq.elems[:i]
	}

	// Make a copy of the cids
	return append([]types.Hash{}, cq.elems...)
}

func (cq *Queue) Push(cid types.Hash) {
	if cq.set.Visit(cid) {
		cq.elems = append(cq.elems, cid)
	}
}

func (cq *Queue) Remove(cid types.Hash) {
	cq.set.Remove(cid)
}

func (cq *Queue) Has(cid types.Hash) bool {
	return cq.set.Has(cid)
}

func (cq *Queue) Len() int {
	return cq.set.Len()
}
