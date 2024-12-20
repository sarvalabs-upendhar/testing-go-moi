package cid

import (
	"encoding/hex"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/storage"
)

var nilCID CID

type CID [33]byte

func ContentID(contentType byte, id [common.HashLength]byte) CID {
	var cid CID

	if id == common.NilHash {
		return cid
	}

	cid[0] = contentType

	copy(cid[1:], id[:])

	return cid
}

func (cid CID) IsNil() bool {
	return cid == nilCID
}

func (cid CID) ContentType() byte {
	return cid[0]
}

func (cid CID) Key() []byte {
	return cid[1:]
}

func (cid CID) String() string {
	return hex.EncodeToString(cid[:])
}

func (cid CID) Bytes() []byte {
	return cid[:]
}

type CIDSet struct {
	set map[CID]struct{}
}

// NewHashSet initializes and returns a new HashSet.
func NewHashSet() *CIDSet {
	return &CIDSet{set: make(map[CID]struct{})}
}

// Add puts a Cid in the HashSet.
func (s *CIDSet) Add(c CID) {
	s.set[c] = struct{}{}
}

// Has returns if the HashSet contains a given Cid.
func (s *CIDSet) Has(c CID) bool {
	_, ok := s.set[c]

	return ok
}

// Remove deletes a hash from the HashSet.
func (s *CIDSet) Remove(c CID) {
	delete(s.set, c)
}

// Len returns how many elements the HashSet has.
func (s *CIDSet) Len() int {
	return len(s.set)
}

// Keys returns the Hashes in the set.
func (s *CIDSet) Keys() []CID {
	out := make([]CID, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}

	return out
}

// Visit adds a Hash to the set only if it is
// not in it already.
func (s *CIDSet) Visit(c CID) bool {
	if !s.Has(c) {
		s.Add(c)

		return true
	}

	return false
}

// ForEach allows to run a custom function on each
// Cid in the set.
func (s *CIDSet) ForEach(f func(c CID) error) error {
	for c := range s.set {
		err := f(c)
		if err != nil {
			return err
		}
	}

	return nil
}

func AccountCID(hash common.Hash) CID {
	return ContentID(storage.Account.Byte(), hash)
}

func ContextCID(hash common.Hash) CID {
	return ContentID(storage.Context.Byte(), hash)
}

func StorageCID(hash common.Hash) CID {
	return ContentID(storage.Storage.Byte(), hash)
}

func AssetCID(hash common.Hash) CID {
	return ContentID(storage.Asset.Byte(), hash)
}

func LogicCID(hash common.Hash) CID {
	return ContentID(storage.Logic.Byte(), hash)
}

func ManifestCID(hash common.Hash) CID {
	return ContentID(storage.LogicManifest.Byte(), hash)
}

func DeedsCID(hash common.Hash) CID {
	return ContentID(storage.Deeds.Byte(), hash)
}

func InteractionsCID(hash common.Hash) CID {
	return ContentID(storage.Interaction.Byte(), hash)
}

func ReceiptsCID(hash common.Hash) CID {
	return ContentID(storage.Receipt.Byte(), hash)
}
