package poi

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"

	"github.com/sarvalabs/go-moi/crypto/common"
)

// aesCTRXOR Standard CTR Mode of AES
func aesCTRXOR(key, inText, iv []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCTR(aesBlock, iv)
	outText := make([]byte, len(inText))
	stream.XORKeyStream(outText, inText)

	return outText, err
}

// generateCryptoParams encrypts the secret with the given password and generate the crypto params of digest
func generateCryptoParams(data, auth []byte) (cryptoParams, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic("reading from signature/rand failed: " + err.Error())
	}

	derivedKey, err := scrypt.Key(auth, salt, 4096, 8, 1, 32)
	if err != nil {
		return cryptoParams{}, err
	}

	encryptKey := derivedKey[:16]

	iv := make([]byte, aes.BlockSize) // 16
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic("reading from signature/rand failed: " + err.Error())
	}

	cipherText, err := aesCTRXOR(encryptKey, data, iv)
	if err != nil {
		return cryptoParams{}, err
	}

	mac := common.GetKeccak256Hash(derivedKey[16:32], cipherText)

	scryptParamsJSON := make(map[string]interface{}, 5)
	scryptParamsJSON["n"] = 4096
	scryptParamsJSON["r"] = 8
	scryptParamsJSON["p"] = 1
	scryptParamsJSON["dklen"] = 32
	scryptParamsJSON["salt"] = hex.EncodeToString(salt)
	cipherParamsJSON := map[string]string{
		"IV": hex.EncodeToString(iv),
	}

	cryptoStruct := cryptoParams{
		Cipher:       "aes-128-ctr",
		CipherText:   hex.EncodeToString(cipherText),
		CipherParams: cipherParamsJSON,
		KDF:          "scrypt",
		KDFParams:    scryptParamsJSON,
		MAC:          hex.EncodeToString(mac),
	}

	return cryptoStruct, nil
}

// StoreKeystore stores the keystore bytes in nodedir keystore path
func StoreKeystore(privKeyBytesOfValidator []byte, nodePassPhrase, dataDir string, nodeKS nodeKeystore) error {
	cryptoStruct, err := generateCryptoParams(privKeyBytesOfValidator, []byte(nodePassPhrase))
	if err != nil {
		return err
	}

	nodeKS.Crypto = cryptoStruct

	ksPayloadInBytes, err := json.Marshal(nodeKS)
	if err != nil {
		return err
	}

	path := filepath.Join(dataDir, "/keystore.json")

	return os.WriteFile(path, ksPayloadInBytes, 0o600)
}

// mParseInt helper for `getKDFKeyForKeystore` to parse the int/float64 to int
func mParseInt(m interface{}) int {
	assertedVal, ok := m.(float64)
	if !ok {
		return 0
	}

	return int(assertedVal)
}

func getKDFKeyForKeystore(cryptoJSON cryptoParams, auth string) ([]byte, error) {
	authArray := []byte(auth)

	salt, err := hex.DecodeString(cryptoJSON.KDFParams["salt"].(string))
	if err != nil {
		return nil, err
	}

	dkLen := mParseInt(cryptoJSON.KDFParams["dklen"])

	if cryptoJSON.KDF == keyHeaderKDF {
		n := mParseInt(cryptoJSON.KDFParams["n"])
		r := mParseInt(cryptoJSON.KDFParams["r"])
		p := mParseInt(cryptoJSON.KDFParams["p"])

		return scrypt.Key(authArray, salt, n, r, p, dkLen)
	}

	return nil, fmt.Errorf("unsupported KDF: %s", cryptoJSON.KDF)
}

// decryptKeystore decrypts the keystore and return bytes of length 32
// [0:32] = Consensus Key and [32:64] = Network Key
func decryptKeystore(cryptoJSON cryptoParams, auth string) ([]byte, error) {
	if cryptoJSON.Cipher != "aes-128-ctr" {
		return nil, fmt.Errorf("cipher not supported: %v", cryptoJSON.Cipher)
	}

	mac, err := hex.DecodeString(cryptoJSON.MAC)
	if err != nil {
		return nil, err
	}

	var targetedInitVector string

	// This is to handle Indus version's POI keystore, in new version the key `IV param`` is all caps
	if cryptoJSON.CipherParams["IV"] != "" {
		targetedInitVector = cryptoJSON.CipherParams["IV"]
	} else {
		targetedInitVector = cryptoJSON.CipherParams["iv"]
	}

	iv, err := hex.DecodeString(targetedInitVector)
	if err != nil {
		return nil, err
	}

	cipherText, err := hex.DecodeString(cryptoJSON.CipherText)
	if err != nil {
		return nil, err
	}

	derivedKey, err := getKDFKeyForKeystore(cryptoJSON, auth)
	if err != nil {
		return nil, err
	}

	calculatedMAC := common.GetKeccak256Hash(derivedKey[16:32], cipherText)
	if !bytes.Equal(calculatedMAC, mac) {
		return nil, errors.New("could not decrypt key with given password")
	}

	plainText, err := aesCTRXOR(derivedKey[:16], cipherText, iv)
	if err != nil {
		return nil, err
	}

	return plainText, err
}
