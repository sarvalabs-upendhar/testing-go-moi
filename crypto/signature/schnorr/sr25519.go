package schnorr

import (
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/crypto/common"
)

// SchnorrSR25519Sig is a custom type of Signature that uses sr25519 Curve and Schnorr signing algorithm for signing
// also implements Mudra interface
type SchnorrSR25519Sig common.Signature

// Type returns ECDSA_SECP256K1 type
func (snrSR25519 *SchnorrSR25519Sig) Type() common.SigType {
	return common.SchnorrSR25519
}

// Sign the data using Secp256k1 curve
func (snrSR25519 *SchnorrSR25519Sig) Sign(data []byte, signingKey []byte, kid identifiers.KramaID) error {
	prvKey := new(Sr25519PrivKey)

	err := prvKey.GenerateFromSecret(signingKey)
	if err != nil {
		return err
	}

	sigBytes, err := prvKey.Sign(data)
	if err != nil {
		return err
	}

	var sigPrefix [2]byte
	sigPrefix[0] = common.SchnorrSR25519.Byte()
	sigPrefix[1] = byte(len(sigBytes))
	snrSR25519.SigPrefix = sigPrefix
	snrSR25519.Digest = sigBytes

	tag, err := kid.Tag()
	if err != nil {
		return err
	}

	if tag.Version() == 0 {
		snrSR25519.Extra = nil // SchnorrSR25519Signature does not need any extra bytes if kramaid version is 1
	} else {
		snrSR25519.Extra, err = prvKey.PubKeyInBytes()
		if err != nil {
			return err
		}
	}

	return nil
}

// Verify used to verify signature against the publicKey
func (snrSR25519 *SchnorrSR25519Sig) Verify(message []byte, nodePublicKey []byte) (bool, error) {
	return VerifySignatureForSchnorrSr25519(message, snrSR25519.Digest, nodePublicKey)
}
