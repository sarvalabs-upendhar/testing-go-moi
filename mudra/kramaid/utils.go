package kramaid

import (
	hexutil "encoding/hex"
	"errors"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/moichain/mudra/common"
)

// GeneratePeerID takes privateKey in bytes and generate peerID for communication
func GeneratePeerID(prvKey []byte) (peer.ID, error) {
	var genPeerID peer.ID

	// Casting secp256k1 private key to signature.PrivKey
	key, err := crypto.UnmarshalSecp256k1PrivateKey(prvKey)
	if err != nil {
		return genPeerID, errors.New("error while un marshalling secp256k1 key:" + err.Error())
	}

	// Generating id from PublicKey
	genPeerID, err = peer.IDFromPublicKey(key.GetPublic())
	if err != nil {
		return genPeerID, err
	}

	return genPeerID, nil
}

func GetAddressFromPublicBytes(pubBytes []byte) string {
	addrBytes := common.GetKeccak256Hash(pubBytes[1:])
	addr := addrBytes[12:]

	return hexutil.EncodeToString(addr)
}

func GetPublicKeyFromPrivateBytes(privKeyBytesOfValidator []byte, compressed bool) []byte {
	var publicBytes []byte

	_, publicKeyInEC := btcec.PrivKeyFromBytes(privKeyBytesOfValidator)

	if compressed {
		publicBytes = publicKeyInEC.SerializeCompressed()
	} else {
		publicBytes = publicKeyInEC.SerializeUncompressed()
	}

	return publicBytes
}

func itob(val uint32) []byte {
	r := make([]byte, 4)
	for i := uint32(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}

	return r
}

func btoi(val []byte) uint32 {
	r := uint32(0)
	for i := uint32(0); i < 4; i++ {
		r |= uint32(val[i]) << (8 * i)
	}

	return r
}

// // getPrivateKeyInBytes is a private function that returns the raw privateKey in bytes
// func serializePrivateKey(prvKey *ecdsa.PrivateKey) []byte {
// 	bigint := prvKey.D
// 	n := prvKey.Params().BitSize / 8

// 	if bigint.BitLen()/8 >= n {
// 		return bigint.Bytes()
// 	}

// 	ret := make([]byte, n)
// 	ReadBits(bigint, ret)

// 	return ret
// }

// ReadBits encodes the absolute value of bigint as big-endian bytes. Callers must ensure
// that buf has enough space. If buf is too short the result will be incomplete.
func ReadBits(bigint *big.Int, buf []byte) {
	i := len(buf)

	for _, d := range bigint.Bits() {
		for j := 0; j < wordBytes && i > 0; j++ {
			i--

			buf[i] = byte(d)
			d >>= 8
		}
	}
}
