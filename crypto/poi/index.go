package poi

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/sarvalabs/go-legacy-kramaid"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/tyler-smith/go-bip39"

	"github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
)

// GetKeystore returns keystore of node that persists private key required for consensus and p2p network communication
func GetKeystore(dataDir string) ([]byte, error) {
	ksFilePath := strings.Join([]string{dataDir, "keystore.json"}, "/")

	ksContent, err := os.ReadFile(ksFilePath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil, common.ErrNoKeystore
		}

		return nil, err
	}

	return ksContent, nil
}

func SetupKeystore(
	currentKramaID kramaid.KramaID,
	bothSignAndCommPrivBytes []byte,
	validatorType moinode.MoiNodeType,
	dataDir string,
	passPhrase string,
) error {
	nodeSpecificPublicBytes := kramaid.GetPublicKeyFromPrivateBytes(bothSignAndCommPrivBytes[0:32], true)
	nodeSpecificPublicAddr := kramaid.GetAddressFromPublicBytes(nodeSpecificPublicBytes)

	moiID, err := currentKramaID.MoiID()
	if err != nil {
		return err
	}

	nodeIndex, err := currentKramaID.NodeIndex()
	if err != nil {
		return err
	}

	var nodeKS nodeKeystore
	nodeKS.MoiID = moiID
	nodeKS.IGCPath = [4]uint32{NodeIGCPath[0], NodeIGCPath[1], NodeIGCPath[2], nodeIndex}
	nodeKS.KramaID = string(currentKramaID)
	nodeKS.NodeType = validatorType.ByteString()
	nodeKS.NodeAddress = nodeSpecificPublicAddr

	if err = StoreKeystore(bothSignAndCommPrivBytes, passPhrase, dataDir, nodeKS); err != nil {
		return err
	}

	return nil
}

func DecryptKeystore(ksInBytes []byte, nodePassPhrase string) ([]byte, string, uint32, error) {
	var nKs nodeKeystore

	err := json.Unmarshal(ksInBytes, &nKs)
	if err != nil {
		return nil, "", 0, err
	}

	prvKeys, err := decryptKeystore(nKs.Crypto, nodePassPhrase)
	if err != nil {
		return nil, "", 0, err
	}

	return prvKeys, nKs.MoiID, nKs.IGCPath[3], nil
}

func getPrivKeysForTest(seed []byte) ([]byte, []byte, error) {
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams) // here key is master key
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
		}
	}
	// Now tempKey points to extended private key at path: m/44'/6174'

	// Deriving MOI ID at m/44'/6174'/0'/0/0
	moiIDPrivKey := tempKey

	moiIDPath := new([3]uint32)
	moiIDPath[0] = kramaid.HardenedStartIndex + 0 // m/44'/6174'/0'
	moiIDPath[1] = 0                              // m/44'/6174'/0'/0 ie., external
	moiIDPath[2] = 0                              // m/44'/6174'/0'/0/0

	for _, n := range moiIDPath {
		moiIDPrivKey, err = moiIDPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}

	moiPubKeyPoint, err := moiIDPrivKey.Neuter()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubInSecp256k1, err := moiPubKeyPoint.ECPubKey()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubBytes := moiIDPubInSecp256k1.SerializeCompressed()

	var aggPrivKey []byte // to concat both private keys

	// Let's derive PrivateKey for signing, so load keyPair at path: m/44'/6174'/5020'/0/n
	validatorPrivKey := tempKey

	var validatorPath [3]uint32
	validatorPath[0] = kramaid.HardenedStartIndex + 5020 // hardened
	validatorPath[1] = 0
	validatorPath[2] = 0

	for _, n := range validatorPath {
		validatorPrivKey, err = validatorPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}
	// Now validatorPrivKey points to extended private key at path: m/44'/6174'/5020'/0/n

	// Casting to Elliptic curve Private key
	privKeyInEC, err := validatorPrivKey.ECPrivKey()
	if err != nil {
		return nil, nil, err
	}

	signingPrivKeyInBytes := privKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, signingPrivKeyInBytes...)

	// Let's derive PrivateKey for communication, so load keyPair at path: m/44'/6174'/6020'/0/n
	ntwPrivKey := tempKey

	var networkPath [3]uint32
	networkPath[0] = kramaid.HardenedStartIndex + 6020 // hardened
	networkPath[1] = 0
	networkPath[2] = 0

	for _, n := range networkPath {
		ntwPrivKey, err = ntwPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}
	// Now ntwPrivKey points to extended private key at path: m/44'/6174'/6020'/0/n

	// Casting to Elliptic curve Private key
	nPrivKeyInEC, err := ntwPrivKey.ECPrivKey()
	if err != nil {
		return nil, nil, err
	}

	ntwPrivKeyInBytes := nPrivKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, ntwPrivKeyInBytes...)

	return aggPrivKey, moiIDPubBytes, nil
}

func RandGenKeystore(dataDir, localNodePass string) ([]byte, kramaid.KramaID, error) {
	mnemonic := GenerateRandMnemonic()

	seed, err := bip39.NewSeedWithErrorChecking(mnemonic.String(), "")
	if err != nil {
		return nil, "", err
	}

	bothSignAndCommPrivBytes, moiPubBytes, err := getPrivKeysForTest(seed)
	if err != nil {
		return nil, "", err
	}

	pairingFriendlyPrivKey := blst.KeyGen(bothSignAndCommPrivBytes[0:32])
	blsPkCasted := *pairingFriendlyPrivKey
	pub := new(blst.P1Affine).From(&blsPkCasted)
	pubBytes := pub.Compress()

	moiIDAddressInString := hex.EncodeToString(moiPubBytes)

	/* Creating KramaID */
	currentKID, err := kramaid.NewKramaID(
		1,
		bothSignAndCommPrivBytes[32:],
		0,
		moiIDAddressInString,
		true,
	)
	if err != nil {
		return nil, "", err
	}

	var nodeKS nodeKeystore
	nodeKS.MoiID = hex.EncodeToString(moiPubBytes)
	nodeKS.IGCPath = [4]uint32{6174, 5020, 0, 0}
	nodeKS.KramaID = string(currentKID)
	nodeKS.NodeType = "0x07"

	if err = StoreKeystore(bothSignAndCommPrivBytes, localNodePass, dataDir, nodeKS); err != nil {
		return nil, "", err
	}

	return pubBytes, currentKID, nil
}

func GenerateRandMnemonic() Mnemonic {
	seedPhrase := Mnemonic{}

	// not checking for error, since below function will not return error
	// as long as we pass bit-size = 128
	randEntropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(randEntropy)
	seedPhrase.FromString(mnemonic)

	return seedPhrase
}
