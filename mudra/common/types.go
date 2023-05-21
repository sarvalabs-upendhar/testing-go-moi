package common

// SigType represents the Signature algorithm
type SigType byte

const (
	// EcdsaSecp256k1 is an enum for the ECDSA Signature scheme using secp256k1 private key
	EcdsaSecp256k1 SigType = 0x01
	// SchnorrSecp256k1 is an enum for the Schnorr Signature scheme using secp256k1 private key
	SchnorrSecp256k1 SigType = 0x02
	// SchnorrSR25519 is an enum for the Schnorr signature scheme using sr25519 private key
	SchnorrSR25519 SigType = 0x03
	// BlsBLST is an enum for the BLS signature scheme using BLST compression
	BlsBLST SigType = 0x04
)

// Byte returns the constant byte associated with Signature type
func (st SigType) Byte() byte {
	return byte(st)
}

// Signature represents the multihash construction of signature (<SIG_ALGO>,<SIG_LENGTH>,<SIG_DIGEST>,<SIG_EXTRA>)
type Signature struct {
	// SigPrefix have bytes of length 2, where [0]: signature algorithm and [1]: signature length
	SigPrefix [2]byte
	// Digest represents digest generated after signing the actual payload
	Digest []byte
	// Extra represents additional information that will be needed at the time of verification
	Extra []byte
}
