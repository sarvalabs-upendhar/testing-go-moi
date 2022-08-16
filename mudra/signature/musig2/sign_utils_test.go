package musig2

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/sarvalabs/btcd-musig/btcec"
	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

// getSetOfKeys returns set of private key bytes and public keys
func getSetOfKeys(setLength int) ([][]byte, []*btcec.PublicKey) {
	var btcecPubKeySet []*btcec.PublicKey

	privKeys := make([][]byte, setLength)
	mnemonic := "act chronic spatial infant kidney endless relief demise raise drama mountain skirt"

	for i := 0; i < setLength; i++ {
		tempPrivKey, _ := kramaid.GetPrivateKeysForSigningAndNetwork(mnemonic, uint32(i))
		_, tempPubKey := btcec.PrivKeyFromBytes(tempPrivKey[0:32])

		btcecPubKeySet = append(btcecPubKeySet, tempPubKey)
		privKeys[i] = tempPrivKey[0:32]
	}

	return privKeys, btcecPubKeySet
}

func TestGetAggregatedPublicKey(t *testing.T) {
	_, pubKeys := getSetOfKeys(108)
	sortedPubKeys, _ := GetSortedKeySet(pubKeys)
	aggPubKey := GetAggregatedPublicKey(sortedPubKeys)

	aggPubKeyInHex := hex.EncodeToString(aggPubKey.SerializeCompressed())
	assert.EqualValues(
		t,
		"025ac08078751ff2b3dbb33cd3d36ece442c59d559be09e63188ff3a7fba504009",
		aggPubKeyInHex,
		fmt.Sprintf("> Expected 025ac08078751ff2b3dbb33cd3d36ece442c59d559be09e63188ff3a7fba504009 "+
			"for Aggregated public key, but got: %v", aggPubKeyInHex))
}

func BenchmarkGetAggregatedPublicKey(b *testing.B) {
	_, pubKKeys := getSetOfKeys(108)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = GetAggregatedPublicKey(pubKKeys)
	}
}
