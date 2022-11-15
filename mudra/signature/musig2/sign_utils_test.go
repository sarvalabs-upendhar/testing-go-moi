package musig2

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Equal(
		t,
		"02ddb5a9d04f95b446b1c64a25d94d64aa572ba457bd970fb2f591e6d295c973d1",
		aggPubKeyInHex,
	)
}

func BenchmarkGetAggregatedPublicKey(b *testing.B) {
	_, pubKKeys := getSetOfKeys(108)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = GetAggregatedPublicKey(pubKKeys)
	}
}
