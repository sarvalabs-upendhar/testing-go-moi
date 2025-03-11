package schnorr

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/crypto/common"
)

// SchnorrSignature is a custom type of Signature that uses Secp256k1 Curve and Schnorr algorithm
// for signing and also implements Mudra interface
type SchnorrSignature common.Signature

// Type returns SCHNORR_SECP256K1 type
func (sch *SchnorrSignature) Type() common.SigType {
	return common.SchnorrSecp256k1
}

// Sign the data using Secp256k1 curve and schnorr signature
func (sch *SchnorrSignature) Sign(data []byte, signingKey []byte, kid identifiers.KramaID) error {
	privKey, pubKey := btcec.PrivKeyFromBytes(signingKey)
	keccakOfMessage := common.GetKeccak256Hash(data)

	sigInSchnorr, err := schnorr.Sign(privKey, keccakOfMessage)
	if err != nil {
		return err
	}

	sigBytes := sigInSchnorr.Serialize()

	var sigPrefix [2]byte
	sigPrefix[0] = common.SchnorrSecp256k1.Byte()
	sigPrefix[1] = byte(len(sigBytes))
	sch.SigPrefix = sigPrefix
	sch.Digest = sigBytes

	// TODO: FIX ME
	tag, err := kid.Tag()
	if err != nil {
		return err
	}

	if tag.Version() == 0 {
		sch.Extra = nil // SchnorrSignature does not need any extra bytes if kramaid version is 1
	} else {
		sch.Extra = pubKey.SerializeCompressed()
	}

	return nil
}

// Verify used to verify signature against the publicKey
func (sch *SchnorrSignature) Verify(message []byte, nodePublicKey []byte) (bool, error) {
	// Parsing publicKey as btcec.PublicKey
	pubKey, err := btcec.ParsePubKey(nodePublicKey)
	if err != nil {
		return false, err
	}

	signInSchnorr, err := schnorr.ParseSignature(sch.Digest)
	if err != nil {
		return false, err
	}

	keccakOfMessage := common.GetKeccak256Hash(message)

	return signInSchnorr.Verify(keccakOfMessage, pubKey), nil
}
