package kramaid

import (
	"crypto/ecdsa"
	hexutil "encoding/hex"
	"errors"
	"math/big"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/sarvalabs/btcd-musig/btcec"
	"gitlab.com/sarvalabs/btcd-musig/btcutil/hdkeychain"
	"gitlab.com/sarvalabs/btcd-musig/chaincfg"
	"gitlab.com/sarvalabs/moichain/mudra/common"
)

// GetPrivateKeysForSigningAndNetwork used to return concatenated privateKeys
// one at path m/44'/6174'/5020'/0/0 for signing
// one at path m/44'/6174'/6020'/0/0 for network communication
// in bytes
func GetPrivateKeysForSigningAndNetwork(mnemonic string, nthValidator uint32) ([]byte, error) {
	// Extract seed from mnemonic
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}

	// 'xMoiIDPubKey' in KramaID is the extended public key at path m/44'/6174'
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams) // here key is master key
	if err != nil {
		return nil, err
	}

	// Hardened keys index starts from 2147483648 (2^31)
	// So.,
	// 44 = 2147483648 + 44 = 2147483692
	// 6174 = 2147483648 + 6174 = 2147489822
	igcParams := [2]uint32{2147483692, 2147489822}

	tempKey := masterKey
	for _, n := range igcParams {
		tempKey, err = tempKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}
	// Now tempKey points to extended private key at path: m/44'/6174'

	var aggPrivKey []byte // to concat both private keys

	// Let's derive PrivateKey for signing, so load keyPair at path: m/44'/6174'/5020'/0/n
	validatorPrivKey := tempKey

	var validatorPath [3]uint32
	validatorPath[0] = HardenedStartIndex + 5020 // hardened
	validatorPath[1] = 0
	validatorPath[2] = nthValidator

	for _, n := range validatorPath {
		validatorPrivKey, err = validatorPrivKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}
	// Now validatorPrivKey points to extended private key at path: m/44'/6174'/5020'/0/n

	// Casting to Elliptic curve Private key
	privKeyInEC, err := validatorPrivKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	privKeyInECDSA := privKeyInEC.ToECDSA()
	signingPrivKeyInBytes := serializePrivateKey(privKeyInECDSA)

	aggPrivKey = append(aggPrivKey[:], signingPrivKeyInBytes[:]...)

	// Let's derive PrivateKey for communication, so load keyPair at path: m/44'/6174'/6020'/0/n
	ntwPrivKey := tempKey

	var networkPath [3]uint32
	networkPath[0] = HardenedStartIndex + 6020 // hardened
	networkPath[1] = 0
	networkPath[2] = nthValidator

	for _, n := range networkPath {
		ntwPrivKey, err = ntwPrivKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}
	// Now ntwPrivKey points to extended private key at path: m/44'/6174'/6020'/0/n

	// Casting to Elliptic curve Private key
	nPrivKeyInEC, err := ntwPrivKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	nPrivKeyInECDSA := nPrivKeyInEC.ToECDSA()
	ntwPrivKeyInBytes := serializePrivateKey(nPrivKeyInECDSA)

	aggPrivKey = append(aggPrivKey[:], ntwPrivKeyInBytes[:]...)

	return aggPrivKey, nil
}

//GeneratePeerID takes privateKey in bytes and generate peerID for communication
func GeneratePeerID(prvKey []byte) (peer.ID, error) {
	var genPeerID peer.ID

	//Casting secp256k1 private key to signature.PrivKey
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

// getPrivateKeyInBytes is a private function that returns the raw privateKey in bytes
func serializePrivateKey(prvKey *ecdsa.PrivateKey) []byte {
	bigint := prvKey.D
	n := prvKey.Params().BitSize / 8

	if bigint.BitLen()/8 >= n {
		return bigint.Bytes()
	}

	ret := make([]byte, n)
	ReadBits(bigint, ret)

	return ret
}

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
