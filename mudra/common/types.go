package common

// SigType represents the Signature algorithm
type SigType uint

const (
	// EcdsaSecp256k1 is an enum for the ECDSA Signature scheme using secp256k1 private key
	EcdsaSecp256k1 SigType = iota
	// SchnorrSecp256k1 is an enum for the Schnorr Signature scheme using secp256k1 private key
	SchnorrSecp256k1
	// SchnorrSR25519 is an enum for the Schnorr signature scheme using sr25519 private key
	SchnorrSR25519
	// BlsBLST is an enum for the BLS signature scheme using BLST compression
	BlsBLST
)

// Byte returns the constant byte associated with Signature type
func (st SigType) Byte() byte {
	switch st {
	case EcdsaSecp256k1:
		return 0x01
	case SchnorrSecp256k1:
		return 0x02
	case SchnorrSR25519:
		return 0x03
	case BlsBLST:
		return 0x04
	default:
		return 0x00
	}
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
