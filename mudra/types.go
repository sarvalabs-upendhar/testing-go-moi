package mudra

import (
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	blst "github.com/supranational/blst/bindings/go"
	"gitlab.com/sarvalabs/btcd-musig/btcec"
)

type KeyType uint8

const (
	RSA KeyType = iota
	SECP256K1
	ED25519
	BLS
)

type PrivateKey interface {
	KeyType() KeyType
	GetPublicKeyInBytes() []byte
	Bytes() []byte
	UnMarshal([]byte)
}

// SECP256K1PrivKey Private key in Secp256k1 Curve
type SECP256K1PrivKey struct {
	p secp256k1.PrivateKey
}

func (secP SECP256K1PrivKey) KeyType() KeyType {
	return SECP256K1
}

func (secP SECP256K1PrivKey) GetPublicKeyInBytes() []byte {
	secpCasted := secP.p
	pub := secpCasted.PubKey()

	return pub.SerializeCompressed()
}

func (secP SECP256K1PrivKey) Bytes() []byte {
	secpCasted := secP.p

	return secpCasted.Serialize()
}

func (secP *SECP256K1PrivKey) UnMarshal(secretBytes []byte) {
	pKey, _ := btcec.PrivKeyFromBytes(secretBytes)
	secP.p = *pKey
}

/* BLS private key */
type BLSPrivKey struct {
	p blst.SecretKey
}

func (blsPk BLSPrivKey) KeyType() KeyType {
	return BLS
}

func (blsPk *BLSPrivKey) UnMarshal(secBytes []byte) {
	pairingFriendlyPrivKey := blst.KeyGen(secBytes)
	blsPk.p = *pairingFriendlyPrivKey
}

func (blsPk BLSPrivKey) Bytes() []byte {
	blsPkCasted := blsPk.p

	return blsPkCasted.Serialize()
}

func (blsPk BLSPrivKey) GetPublicKeyInBytes() []byte {
	blsPkCasted := blsPk.p
	pub := new(blst.P1Affine).From(&blsPkCasted)

	return pub.Compress()
}
