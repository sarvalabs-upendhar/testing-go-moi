package musig2

import (
	"bytes"
	"errors"

	"gitlab.com/sarvalabs/btcd-musig/btcec"
	"gitlab.com/sarvalabs/btcd-musig/btcec/schnorr"
	"gitlab.com/sarvalabs/btcd-musig/btcec/schnorr/musig2"
)

// GetSortedKeySet returns sorted public keys where each publicKey is compressed on even co-ordinate
func GetSortedKeySet(pubKeys []*btcec.PublicKey) ([]*btcec.PublicKey, error) {
	parsedPubkeys := make([]*btcec.PublicKey, len(pubKeys))

	for i, pub := range pubKeys {
		pBytes := pub.SerializeCompressed()
		pBytes[0] = 0x02 // always even

		currentParsedPubkey, err := btcec.ParsePubKey(pBytes)
		if err != nil {
			return nil, err
		}

		parsedPubkeys[i] = currentParsedPubkey
	}

	sortedPublicKeys := sortPublicKeys(parsedPubkeys)

	return sortedPublicKeys, nil
}

// GetAggregatedPublicKey used to return sorted public keys and aggregated Public Key
func GetAggregatedPublicKey(sortedPublicKeys []*btcec.PublicKey) *btcec.PublicKey {
	var secondUniqueKeyIndex int

	// Find the first key that isn't the same as the very first key (second unique key).
	firstElementInPublicKeySet := schnorr.SerializePubKey(sortedPublicKeys[0])
	for i := range sortedPublicKeys {
		if !bytes.Equal(schnorr.SerializePubKey(sortedPublicKeys[i]), firstElementInPublicKeySet) {
			secondUniqueKeyIndex = i

			break
		}
	}

	return musig2.AggregateKeys(sortedPublicKeys, false, musig2.WithUniqueKeyIndex(secondUniqueKeyIndex))
}

// InitMusigSession used to generate session from Node Private key and sorted Public keys
func InitMusigSession(signerKey []byte, sortedPubKeySet []*btcec.PublicKey) (*musig2.Session, error) {
	privKey, _ := btcec.PrivKeyFromBytes(signerKey)

	signCtx, err := musig2.NewContext(privKey, sortedPubKeySet, false)
	if err != nil {
		return nil, err
	}

	currentSession, err := signCtx.NewSession()
	if err != nil {
		return nil, err
	}

	return currentSession, nil
}

// UnmarshalPartialSig Unmarshalls partial signature into bytes to sent in wire
func UnmarshalPartialSig(partialSignature *musig2.PartialSignature) []byte {
	partialSignatureInBytes := make([]byte, 65)

	sComponentInBytes := partialSignature.S.Bytes()
	// fmt.Println("S Component:", sComponentInBytes)
	for i := 0; i < 32; i++ {
		partialSignatureInBytes[i] = sComponentInBytes[i]
	}

	rComponentInBytes := partialSignature.R.SerializeCompressed()
	// fmt.Println("R Component: ", rComponentInBytes)

	copy(partialSignatureInBytes[32:], rComponentInBytes)
	// fmt.Println("Partial Sig in Bytes: ", partialSignatureInBytes)
	return partialSignatureInBytes
}

// MarshalToPartialSig Marshals the bytes of S and R component in signature into Partial Signature
func MarshalToPartialSig(partSigBytes []byte) (*musig2.PartialSignature, error) {
	if len(partSigBytes) != 65 {
		return nil, errors.New("invalid partial signature length")
	}

	sComponent := new(btcec.ModNScalar)
	sComponent.SetByteSlice(partSigBytes[:32])

	rComponent, err := btcec.ParsePubKey(partSigBytes[32:])
	if err != nil {
		return nil, err
	}

	musigPrtSig := musig2.NewPartialSignature(sComponent, rComponent)

	return &musigPrtSig, nil
}
