package kutils

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

type Set struct {
	set map[ktypes.Hash]struct{}
}

// NewSet initializes and returns a new Set.
func NewSet() *Set {
	return &Set{set: make(map[ktypes.Hash]struct{})}
}

// Add puts a Cid in the Set.
func (s *Set) Add(c ktypes.Hash) {
	s.set[c] = struct{}{}
}

// Has returns if the Set contains a given Cid.
func (s *Set) Has(c ktypes.Hash) bool {
	_, ok := s.set[c]

	return ok
}

// Remove deletes a hash from the Set.
func (s *Set) Remove(c ktypes.Hash) {
	delete(s.set, c)
}

// Len returns how many elements the Set has.
func (s *Set) Len() int {
	return len(s.set)
}

// Keys returns the Hashes in the set.
func (s *Set) Keys() []ktypes.Hash {
	out := make([]ktypes.Hash, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}

	return out
}

// Visit adds a Hash to the set only if it is
// not in it already.
func (s *Set) Visit(c ktypes.Hash) bool {
	if !s.Has(c) {
		s.Add(c)

		return true
	}

	return false
}

// ForEach allows to run a custom function on each
// Cid in the set.
func (s *Set) ForEach(f func(c ktypes.Hash) error) error {
	for c := range s.set {
		err := f(c)
		if err != nil {
			return err
		}
	}

	return nil
}

type Queue struct {
	elems []ktypes.Hash
	set   *Set
}

func NewCidQueue() *Queue {
	return &Queue{set: NewSet()}
}

func (cq *Queue) Pop() ktypes.Hash {
	for {
		if len(cq.elems) == 0 {
			return ktypes.NilHash
		}

		out := cq.elems[0]
		cq.elems = cq.elems[1:]

		if cq.set.Has(out) {
			cq.set.Remove(out)

			return out
		}
	}
}

func (cq *Queue) Cids() []ktypes.Hash {
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
	return append([]ktypes.Hash{}, cq.elems...)
}

func (cq *Queue) Push(cid ktypes.Hash) {
	if cq.set.Visit(cid) {
		cq.elems = append(cq.elems, cid)
	}
}

func (cq *Queue) Remove(cid ktypes.Hash) {
	cq.set.Remove(cid)
}

func (cq *Queue) Has(cid ktypes.Hash) bool {
	return cq.set.Has(cid)
}

func (cq *Queue) Len() int {
	return cq.set.Len()
}
