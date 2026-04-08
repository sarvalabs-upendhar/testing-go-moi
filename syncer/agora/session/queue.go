package session

import (
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type Queue struct {
	elems []cid.CID
	set   *cid.CIDSet
}

func NewCidQueue() *Queue {
	return &Queue{set: cid.NewHashSet()}
}

func (cq *Queue) Pop() cid.CID {
	for {
		if len(cq.elems) == 0 {
			return cid.CID{}
		}

		out := cq.elems[0]
		cq.elems = cq.elems[1:]

		if cq.set.Has(out) {
			cq.set.Remove(out)

			return out
		}
	}
}

func (cq *Queue) Cids() []cid.CID {
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
	return append([]cid.CID{}, cq.elems...)
}

func (cq *Queue) Push(cid cid.CID) {
	if cq.set.Visit(cid) {
		cq.elems = append(cq.elems, cid)
	}
}

func (cq *Queue) Remove(cid cid.CID) {
	cq.set.Remove(cid)
}

func (cq *Queue) Has(cid cid.CID) bool {
	return cq.set.Has(cid)
}

func (cq *Queue) Len() int {
	return cq.set.Len()
}
