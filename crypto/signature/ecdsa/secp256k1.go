package ecdsa

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/crypto/common"
)

// EcdsaSecp256k1Signature implements Mudra
// which is a custom type of Signature that uses Secp256k1 Curve
// and ECDSA signing algorithm for signing and verification
type EcdsaSecp256k1Signature common.Signature

// Type returns ECDSA_SECP256K1 type
func (s256Sig *EcdsaSecp256k1Signature) Type() common.SigType {
	return common.EcdsaSecp256k1
}

// Sign the data using Secp256k1 curve
func (s256Sig *EcdsaSecp256k1Signature) Sign(rawMessage []byte, signingKey []byte, kid identifiers.KramaID) error {
	privateKey, publicKey := btcec.PrivKeyFromBytes(signingKey)

	// Generating blake2b hash for the message
	messageHash := blake2b.Sum256(rawMessage)

	signature := ecdsa.Sign(privateKey, messageHash[:])
	sigBytes := signature.Serialize()

	var sigPrefix [2]byte
	sigPrefix[0] = common.EcdsaSecp256k1.Byte()
	sigPrefix[1] = byte(len(sigBytes))
	s256Sig.SigPrefix = sigPrefix
	s256Sig.Digest = sigBytes

	// TODO: FIX ME
	tag, err := kid.Tag()
	if err != nil {
		return err
	}

	pubBytes := publicKey.SerializeCompressed()

	if tag.Version() == 0 {
		s256Sig.Extra = pubBytes[:1] // Adding public key's parity prefix
	} else {
		s256Sig.Extra = pubBytes
	}

	return nil
}

// Verify used to verify ECDSA signature against the publicKey
func (s256Sig *EcdsaSecp256k1Signature) Verify(rawMessage []byte, publicKeyBytes []byte) (bool, error) {
	if len(publicKeyBytes) == 32 {
		yAxis := s256Sig.Extra
		// Adding 0x03/0x02 and making pubkey length as 33 to signify that it is compressed
		publicKeyBytes = append(yAxis, publicKeyBytes...)
	}

	pubKey, err := btcec.ParsePubKey(publicKeyBytes)
	if err != nil {
		return false, err
	}

	signature, err := ecdsa.ParseSignature(s256Sig.Digest)
	if err != nil {
		return false, err
	}

	messageHash := blake2b.Sum256(rawMessage)

	return signature.Verify(messageHash[:], pubKey), nil
}
