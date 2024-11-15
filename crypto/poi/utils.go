package poi

import (
	"crypto/ecdsa"
	hexutil "encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/tyler-smith/go-bip39"
)

const (
	MaxTimeForConnect  = 2 * time.Second
	MaxTimeForRequest  = 2 * time.Second
	HardenedStartIndex = 2147483648 // 2^31
	// number of bits in a big.Word
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	// number of bytes in a big.Word
	wordBytes = wordBits / 8
)

// trimHexString remove 0x prefix to hex string
func trimHexString(hexString string) string {
	strInBytes := []byte(hexString)
	if string(strInBytes[:2]) == "0x" {
		return string(strInBytes[2:])
	} else {
		return string(strInBytes)
	}
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

func GetPrivateKeyAtPath(mnemonic, hdPath string) ([]byte, []byte, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, nil, err
	}

	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, nil, err
	}

	_hdPath := strings.Split(hdPath, "/")
	if len(_hdPath) != 6 {
		return nil, nil, errors.New("invalid Igc path")
	}

	nodeNumbers := make([]uint32, 5)
	counter := 0

	for _, node := range _hdPath {
		if node != "m" {
			if node[len(node)-1:] == "'" {
				num, err := strconv.ParseUint(node[:len(node)-1], 10, 32)
				if err != nil {
					return nil, nil, err
				}

				nodeNumbers[counter] = HardenedStartIndex + uint32(num)
			} else {
				num, err := strconv.ParseUint(node, 10, 32)
				if err != nil {
					return nil, nil, err
				}

				nodeNumbers[counter] = uint32(num)
			}

			counter++
		}
	}

	tempKey := masterKey
	for _, n := range nodeNumbers {
		tempKey, err = tempKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}

	privKeyInEC, err := tempKey.ECPrivKey()
	if err != nil {
		return nil, nil, err
	}

	privKeyInECDSA := privKeyInEC.ToECDSA()
	signingPrivKeyInBytes := serializePrivateKey(privKeyInECDSA)
	_, publicKey := btcec.PrivKeyFromBytes(signingPrivKeyInBytes)

	compressedPubKey := publicKey.SerializeCompressed()

	return signingPrivKeyInBytes, compressedPubKey[1:], nil
}

// GetPrivateKeysForSigningAndNetwork used to return concatenated privateKeys
// one at path m/44'/6174'/5020'/0/0 for signing
// one at path m/44'/6174'/6020'/0/0 for network communication
// in bytes
func GetPrivateKeysForSigningAndNetwork(mnemonic string, nthValidator uint32) ([]byte, string, error) {
	// Extract seed from mnemonic
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, "", err
	}
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, "", err
	}
	// Now tempKey points to extended private key at path: m/44'/6174'

	// Hardened keys index starts from 2147483648 (2^31)
	// So.,
	// 44' = 2147483648 + 44 = 2147483692
	// 6174' = 2147483648 + 6174 = 2147489822
	igcParams := [2]uint32{2147483692, 2147489822}

	tempKey := masterKey
	for _, n := range igcParams {
		tempKey, err = tempKey.Derive(n)
		if err != nil {
			return nil, "", err
		}
	}
	// Now tempKey points to extended private key at path: m/44'/6174'

	var aggPrivKey []byte // concatenatation of network and consensus private keys

	// Derive PrivateKey for signing, so load keyPair at path: m/44'/6174'/5020'/0/n
	validatorPrivKey := tempKey

	var validatorPath [3]uint32
	validatorPath[0] = HardenedStartIndex + 5020 // hardened
	validatorPath[1] = 0
	validatorPath[2] = nthValidator

	for _, n := range validatorPath {
		validatorPrivKey, err = validatorPrivKey.Derive(n)
		if err != nil {
			return nil, "", err
		}
	}

	// Casting to Elliptic curve Private key
	privKeyInEC, err := validatorPrivKey.ECPrivKey()
	if err != nil {
		return nil, "", err
	}

	privKeyInECDSA := privKeyInEC.ToECDSA()
	signingPrivKeyInBytes := serializePrivateKey(privKeyInECDSA)
	aggPrivKey = append(aggPrivKey, signingPrivKeyInBytes...)

	// Derive PrivateKey for network, so load keyPair at path: m/44'/6174'/6020'/0/n
	ntwPrivKey := tempKey

	var networkPath [3]uint32
	networkPath[0] = HardenedStartIndex + 6020
	networkPath[1] = 0
	networkPath[2] = nthValidator

	for _, n := range networkPath {
		ntwPrivKey, err = ntwPrivKey.Derive(n)
		if err != nil {
			return nil, "", err
		}
	}

	// Casting to Elliptic curve Private key
	nPrivKeyInEC, err := ntwPrivKey.ECPrivKey()
	if err != nil {
		return nil, "", err
	}

	nPrivKeyInECDSA := nPrivKeyInEC.ToECDSA()
	ntwPrivKeyInBytes := serializePrivateKey(nPrivKeyInECDSA)

	aggPrivKey = append(aggPrivKey, ntwPrivKeyInBytes...)

	// Derive MOI ID public Key which is at path m/44'/6174'/0'/0/0
	moiIDPrivateKey := tempKey

	var moiIDPath [3]uint32
	moiIDPath[0] = HardenedStartIndex + 0
	moiIDPath[1] = 0
	moiIDPath[2] = 0

	for _, n := range moiIDPath {
		moiIDPrivateKey, err = moiIDPrivateKey.Derive(n)
		if err != nil {
			return nil, "", err
		}
	}

	moiIDPubKey, err := moiIDPrivateKey.Neuter()
	if err != nil {
		return nil, "", err
	}

	moiIDPubInSecp256k1, err := moiIDPubKey.ECPubKey()
	if err != nil {
		return nil, "", err
	}

	moiIDPubBytes := moiIDPubInSecp256k1.SerializeCompressed()
	// fmt.Println("Pub: ", moiIDPubBytes)
	// moiID := getAddressFromPublicBytes(moiIDPubBytes)
	// fmt.Println("Addr: ", moiID)
	return aggPrivKey, trimHexString(hexutil.EncodeToString(moiIDPubBytes)), nil
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
