package vrf

import (
	"bytes"
	"crypto"
	"errors"

	"golang.org/x/crypto/blake2b"

	"github.com/supranational/blst/bindings/go"
)

var (
	Digest = []byte("BLS_SIG_BLS12381G2_XMD:BLAKE2B_SSWU_RO_NUL_")
	// ErrInvalidVRF occurs when the VRF does not validate.
	ErrInvalidVRF = errors.New("invalid VRF proof")
	// ErrSignCreation occurs when the signing fails.
	ErrSignCreation = errors.New("failed to create signature")
)

// PublicKey holds a public VRF key.
type PublicKey struct {
	*blst.P1Affine
}

// PrivateKey holds a private VRF key.
type PrivateKey struct {
	*blst.SecretKey
}

// Public returns the corresponding public key as bytes.
func (k *PrivateKey) Public() crypto.PublicKey {
	pk := new(PublicKey)
	pk.From(k.SecretKey)

	return pk
}

// NewVRFSigner creates a signer object from a private key.
func NewVRFSigner(seck *blst.SecretKey) *PrivateKey {
	return &PrivateKey{seck}
}

// NewVRFVerifier creates a verifier object from a public key.
func NewVRFVerifier(pubkey *blst.P1Affine) *PublicKey {
	return &PublicKey{pubkey}
}

// Evaluate returns the verifiable unpredictable function evaluated using alpha
func (k *PrivateKey) Evaluate(alpha []byte) ([32]byte, []byte, error) {
	msgHash := blake2b.Sum256(alpha)

	sig := new(blst.P2Affine).Sign(k.SecretKey, msgHash[:], Digest)
	if sig == nil {
		return [32]byte{}, nil, ErrSignCreation
	}

	beta := blake2b.Sum256(sig.Serialize())

	return beta, sig.Serialize(), nil
}

// ProofToHash returns the vrf output hash from the given input alpha and signature
func (pk *PublicKey) ProofToHash(alpha, pi []byte) ([32]byte, error) {
	if len(pi) == 0 {
		return [32]byte{}, ErrInvalidVRF
	}

	sig := new(blst.P2Affine).Deserialize(pi)

	msgHash := blake2b.Sum256(alpha)

	if !sig.Verify(true, pk.P1Affine, true, msgHash[:], Digest) {
		return [32]byte{}, ErrInvalidVRF
	}

	return blake2b.Sum256(pi), nil
}

// Verify asserts that proof is correct for input alpha and output VRF hash
func (pk *PublicKey) Verify(vrfOutput [32]byte, alpha, signature []byte) (bool, error) {
	hashOfProof, err := pk.ProofToHash(alpha, signature)
	if err != nil {
		return false, err
	}

	return bytes.Equal(vrfOutput[:], hashOfProof[:]), nil
}
