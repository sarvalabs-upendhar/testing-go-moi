package poi

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	mrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	blst "github.com/supranational/blst/bindings/go"
	"gitlab.com/sarvalabs/btcd-musig/btcutil/hdkeychain"
	"gitlab.com/sarvalabs/btcd-musig/chaincfg"
	"gitlab.com/sarvalabs/moichain/mudra/common"
	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/mudra/poi/moinode"
)

// GetKeystore returns keystore of node that persists private key required for consensus and p2p network communication
func GetKeystore(dataDir string) ([]byte, error) {
	ksFilePath := strings.Join([]string{dataDir, "keystore.json"}, "/")
	ksContent, err := ioutil.ReadFile(ksFilePath)

	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil, common.ErrNoKeystore
		}

		return nil, err
	}

	return ksContent, nil
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

func GenerateKeysForVault(userName, pAsSw0rd, environment string) ([]byte, string, uint32, error) {
	if environment == "" {
		return nil, "", 0, errors.New("environment cannot be empty")
	}

	moiIDBaseURL := getMoiIDBaseURL(environment)

	// Get Base-X Encoded username
	encodedUserName, err := BaseXEncode(userName)
	if err != nil {
		return nil, "", 0, err
	}

	// Get MOI Wallet Default Address
	defAddrInBytes, err := sendRequest(moiIDBaseURL, "getdefaultaddress", *encodedUserName)
	if err != nil {
		return nil, "", 0, err
	}

	defautlAddress := string(defAddrInBytes)

	if defautlAddress == ZeroAddress {
		return nil, "", 0, common.ErrInvalidUsername
	}
	// authenticate if user's moi id is correct or not
	// authStatus is a bool variable which convey if user is a valid moi-id or not
	authStatus, zkProof, err := Authenticate(defautlAddress, pAsSw0rd, moiIDBaseURL)
	if err != nil {
		return nil, "", 0, err
	}

	if authStatus {
		// Get Wallet Keystore from MOI id to derive Secret Recovery Phrase
		ksPayload := map[string]interface{}{
			"defAddr":     defautlAddress,
			"typeOfProof": "keystore",
			"authToken":   zkProof,
		}
		ksPayloadInJSON, err := json.Marshal(ksPayload)

		if err != nil {
			return nil, "", 0, err
		}

		keystoreResponse, err := http.Post(moiIDBaseURL+"/moi-id/auth/getmks",
			"application/json", bytes.NewBuffer(ksPayloadInJSON))

		if err != nil {
			return nil, "", 0, err
		}

		keystoreResponseInBytes, err := ioutil.ReadAll(keystoreResponse.Body)
		if err != nil {
			return nil, "", 0, err
		}

		type MKS struct {
			ZKP             interface{}        `json:"_proof"`
			KeyStore        WalletKeystoreJSON `json:"_keystore"`
			MoiCipherParams interface{}        `json:"_moiCipherParams"`
		}

		var userMks MKS
		err = json.Unmarshal(keystoreResponseInBytes, &userMks)

		if err != nil {
			return nil, "", 0, err
		}

		srp, err := getSRPFromEncryptedJSON(userMks.KeyStore, pAsSw0rd)
		if err != nil {
			return nil, "", 0, err
		}

		_, err = CheckForKYC(defautlAddress, moiIDBaseURL)
		if err != nil {
			return nil, "", 0, err
		}

		/* Initializing the MoiNode Registry */
		mnReg := moinode.Init(moiIDBaseURL)

		// Get number of nodes the user have
		_, addressIndex, err := mnReg.GetNodes(map[string]interface{}{
			"userID":    defautlAddress,
			"countOnly": true,
		})
		if err != nil {
			return nil, "", 0, err
		}

		bothSignAndCommPrivBytes, err := kramaid.GetPrivateKeysForSigningAndNetwork(*srp, addressIndex)
		if err != nil {
			return nil, "", 0, err
		}

		moiIDAddressInLC := trimHexString(defautlAddress)

		return bothSignAndCommPrivBytes, moiIDAddressInLC, addressIndex, nil
	}

	return nil, "", 0, common.ErrAuthFailed
}

func RegisterNode(bothSignAndCommPrivBytes []byte,
	moiIDAddress, dataDir, localNodePass string,
	myKID kramaid.KramaID,
	nodeType moinode.MoiNodeType,
	env string) error {
	defautlAddress := moiIDAddress
	nodeSpecificPublicBytes := kramaid.GetPublicKeyFromPrivateBytes(bothSignAndCommPrivBytes[0:32], true)
	nodeSpecificPublicAddr := kramaid.GetAddressFromPublicBytes(nodeSpecificPublicBytes)

	kramaIDInString := string(myKID)

	addressIndex, err := myKID.NodeIndex()
	if err != nil {
		return err
	}

	/* Storing keystore locally */
	var nodeKS nodeKeystore
	nodeKS.MoiID = trimHexString(defautlAddress)
	nodeKS.IGCPath = [4]uint32{NodeIGCPath[0], NodeIGCPath[1], NodeIGCPath[2], addressIndex}
	nodeKS.KramaID = kramaIDInString
	nodeKS.NodeType = nodeType.ByteString()
	nodeKS.NodeAddress = nodeSpecificPublicAddr

	moiIDBaseURL := getMoiIDBaseURL(env)

	mnReg := moinode.Init(moiIDBaseURL)

	_, err = mnReg.UpdateNode(
		false,
		nodeSpecificPublicBytes,
		defautlAddress,
		nodeType.ByteString(),
		kramaIDInString,
	)
	if err != nil {
		return err
	}

	if err = storeKeystore(bothSignAndCommPrivBytes, localNodePass, dataDir, nodeKS); err != nil {
		return err
	}

	return nil
}

// storeKeystore stores the keystore bytes in nodedir keystore path
func storeKeystore(privKeyBytesOfValidator []byte, nodePassPhrase, dataDir string, nodeKS nodeKeystore) error {
	cryptoStruct, err := encryptAndGetKeystore(privKeyBytesOfValidator, []byte(nodePassPhrase))
	if err != nil {
		return err
	}

	nodeKS.Crypto = cryptoStruct

	ksPayloadInBytes, err := json.Marshal(nodeKS)
	if err != nil {
		return err
	}

	path := filepath.Join(dataDir + "/keystore.json")
	f, err := os.Create(path)

	if err != nil {
		return err
	}

	bytesWritten, err := f.Write(ksPayloadInBytes)
	if err != nil {
		err := f.Close()
		if err != nil {
			return errors.New("error closing the file " + err.Error())
		}

		return err
	}

	if bytesWritten > 0 {
		err = f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func getPrivKeysForTest(seed []byte) ([]byte, []byte, error) {
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams) // here key is master key
	if err != nil {
		return nil, nil, err
	}

	/* Deriving MOI id address */
	masPubKey, err := masterKey.Neuter()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubInSecp256k1, err := masPubKey.ECPubKey()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubBytes := moiIDPubInSecp256k1.SerializeCompressed()

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

	aggPrivKey = append(aggPrivKey[:], signingPrivKeyInBytes[:]...)

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

	aggPrivKey = append(aggPrivKey[:], ntwPrivKeyInBytes[:]...)

	return aggPrivKey, moiIDPubBytes, nil
}

func RandGenKeystore(dataDir, localNodePass string) ([]byte, kramaid.KramaID, error) {
	randInt64 := time.Now().UnixNano()
	source := mrand.NewSource(randInt64)

	var signKey [32]byte
	_, err := mrand.New(source).Read(signKey[:]) //nolint

	if err != nil {
		return nil, "", err
	}

	bothSignAndCommPrivBytes, moiPubBytes, err := getPrivKeysForTest(signKey[:])
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
		bothSignAndCommPrivBytes[32:],
		0,
		moiIDAddressInString,
		1,
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

	if err = storeKeystore(bothSignAndCommPrivBytes, localNodePass, dataDir, nodeKS); err != nil {
		return nil, "", err
	}

	return pubBytes, currentKID, nil
}
