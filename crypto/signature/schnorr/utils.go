package schnorr

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"

	sr25519In "github.com/oasisprotocol/curve25519-voi/primitives/sr25519"

	"github.com/sarvalabs/moichain/crypto/common"
)

type Sr25519PrivKey struct {
	msk sr25519In.MiniSecretKey
	kp  *sr25519In.KeyPair
}

// GenerateFromSecret hashes the secret with SHA2, and uses
// that 32 byte output to create the private key.
func (privKey *Sr25519PrivKey) GenerateFromSecret(secret []byte) error {
	hasher := sha256.New()
	hasher.Write(secret)
	seed := hasher.Sum(nil)

	tempPrivKey := new(Sr25519PrivKey)
	if err := tempPrivKey.msk.UnmarshalBinary(seed); err != nil {
		return errors.New("sr25519: failed to deserialize MiniSecretKey: " + err.Error())
	}

	sk := tempPrivKey.msk.ExpandEd25519()
	privKey.kp = sk.KeyPair()

	return nil
}

// Sign produces a signature on the provided message.
func (privKey *Sr25519PrivKey) Sign(msg []byte) ([]byte, error) {
	if privKey.kp == nil {
		return nil, common.ErrUnIntialized
	}

	signingCtx := sr25519In.NewSigningContext([]byte{})
	st := signingCtx.NewTranscriptBytes(msg)

	sig, err := privKey.kp.Sign(rand.Reader, st)
	if err != nil {
		return nil, errors.New("failed to sign message: " + err.Error())
	}

	sigBytes, err := sig.MarshalBinary()
	if err != nil {
		return nil, errors.New("failed to serialize signature: " + err.Error())
	}

	return sigBytes, nil
}

// PubKeyInBytes gets the corresponding public key from the private key.
func (privKey *Sr25519PrivKey) PubKeyInBytes() ([]byte, error) {
	if privKey.kp == nil {
		return nil, common.ErrUnIntialized
	}

	pubBytes, err := privKey.kp.PublicKey().MarshalBinary()
	if err != nil {
		return nil, errors.New("failed to serialize public key: " + err.Error())
	}

	return pubBytes, nil
}

func VerifySignatureForSchnorrSr25519(msg, sigBytes, pubKey []byte) (bool, error) {
	var srpk sr25519In.PublicKey
	if err := srpk.UnmarshalBinary(pubKey); err != nil {
		return false, err
	}

	var sig sr25519In.Signature
	if err := sig.UnmarshalBinary(sigBytes); err != nil {
		return false, err
	}

	signingCtx := sr25519In.NewSigningContext([]byte{})
	st := signingCtx.NewTranscriptBytes(msg)

	return srpk.Verify(st, &sig), nil
}
