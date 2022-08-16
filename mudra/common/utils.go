package common

import (
	"crypto/subtle"
	"golang.org/x/crypto/sha3"
	"hash"
)

// GetKeccak256Hash calculates and returns the Keccak256 hash of the input data.
func GetKeccak256Hash(data ...[]byte) []byte {
	b := make([]byte, 32)
	d, ok := sha3.NewLegacyKeccak256().(interface {
		hash.Hash
		Read([]byte) (int, error)
	})

	if !ok {
		return b
	}

	for _, b := range data {
		d.Write(b)
	}

	if _, err := d.Read(b); err != nil {
		return nil
	}

	return b
}

// IsZeroBytes checks if the key is a zero key.
func IsZeroBytes(key []byte) bool {
	b := byte(0)
	for _, s := range key {
		b |= s
	}

	return subtle.ConstantTimeByteEq(b, 0) == 1
}

// UnmarshalSignature unmarshalls the hex bytes into signature which further can be type cast to different SignatureType
func UnmarshalSignature(hexBytes []byte) (Signature, error) {
	var unParsedSignature Signature

	if len(hexBytes) == 0 {
		return unParsedSignature, ErrEmpty
	}

	unParsedSignature.SigPrefix = [2]byte{hexBytes[0], hexBytes[1]}
	unParsedSignature.Digest = hexBytes[2 : hexBytes[1]+2]

	extraBytes := hexBytes[2+int(hexBytes[1]):]
	if len(extraBytes) == 0 {
		unParsedSignature.Extra = nil
	} else {
		unParsedSignature.Extra = hexBytes[2+int(hexBytes[1]):]
	}

	return unParsedSignature, nil
}

// MarshalSignature returns the signature bytes in this order Signature prefix + Signature Digest + Extra bytes
func MarshalSignature(sig Signature) []byte {
	var finalSigBytes []byte
	finalSigBytes = append(finalSigBytes, sig.SigPrefix[:]...)
	finalSigBytes = append(finalSigBytes, sig.Digest...)

	if sig.Extra != nil {
		finalSigBytes = append(finalSigBytes, sig.Extra...)
	}

	return finalSigBytes
}
