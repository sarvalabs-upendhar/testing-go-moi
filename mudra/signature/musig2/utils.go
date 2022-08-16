package musig2

import (
	"bytes"
	"gitlab.com/sarvalabs/btcd-musig/btcec"
	"gitlab.com/sarvalabs/btcd-musig/btcec/schnorr"
	"sort"
)

// sortableKeys defines a type of slice of public keys that implements the sort
// interface for BIP 340 keys.
type sortableKeys []*btcec.PublicKey

// Less reports whether the element with index i must sort before the element
// with index j.
func (s sortableKeys) Less(i, j int) bool {
	keyIBytes := schnorr.SerializePubKey(s[i])
	keyJBytes := schnorr.SerializePubKey(s[j])

	return bytes.Compare(keyIBytes, keyJBytes) == -1
}

// Swap swaps the elements with indexes i and j.
func (s sortableKeys) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Len is the number of elements in the collection.
func (s sortableKeys) Len() int {
	return len(s)
}

// sortPublicKeys takes a set of schnorr public keys and returns a new slice that is
// a copy of the keys sorted in lexicographical order bytes
func sortPublicKeys(keys []*btcec.PublicKey) []*btcec.PublicKey {
	keySet := sortableKeys(keys)
	if sort.IsSorted(keySet) {
		return keys
	}

	sort.Sort(keySet)

	return keySet
}
