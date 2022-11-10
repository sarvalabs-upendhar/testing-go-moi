package poi

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/eknkc/basex"
	"gitlab.com/sarvalabs/moichain/mudra/common"
	"golang.org/x/crypto/scrypt"
)

const (
	MaxTimeForConnect = 2 * time.Second
	MaxTimeForRequest = 2 * time.Second
)

// BaseXEncode function converts string to hex and do encode using bitcoin style leading zero compression
// Know more about this basex encoding here: https://awesomeopensource.com/project/cryptocoinjs/base-x
func BaseXEncode(username string) (*string, error) {
	convertedHexstring := fmt.Sprintf("%x", username)

	decodedHexString, err := hex.DecodeString(convertedHexstring)
	if err != nil {
		return nil, errors.New("error in decoding the string to hex")
	}

	base58EncodingScheme, err := basex.NewEncoding(ALPHABET)
	if err != nil {
		return nil, errors.New("error in constructing MOI Encoding scheme")
	}

	moiEncodedString := base58EncodingScheme.Encode(decodedHexString)

	return &moiEncodedString, nil
}

func getMoiIDBaseURL(env string) string {
	switch env {
	case "dev":
		return "https://devapi.moinet.io"
	case "qa":
		return "https://qaapi.moinet.io"
	case "prod":
		return "https://api.moinet.io"
	default:
		return "https://api.moinet.io"
	}
}

func sendRequest(moiIDBaseURL string, reqPath string, encodedUserName string) ([]byte, error) {
	var (
		err  error
		resp *http.Response
	)

	switch reqPath {
	case "getdefaultaddress":
		{
			resp, err = http.Get(moiIDBaseURL + "/moi-id/identity/getdefaultaddress?username=" + encodedUserName)
		}
	default:
		{
			return nil, errors.New("invalid request path")
		}
	}

	if err != nil {
		return nil, err
	}

	respInBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respInBytes, nil
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

	// This is to handle alpha version's poi keystore
	// in new version IV param is all caps
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

// mParseInt is to parse the int/float64 to int, helps in KDF
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

// trimHexString remove 0x prefix to hex string
func trimHexString(hexString string) string {
	strInBytes := []byte(hexString)
	if string(strInBytes[:2]) == "0x" {
		return string(strInBytes[2:])
	} else {
		return string(strInBytes)
	}
}

// encryptAndGetKeystore encrypts the secret with the given password 'auth'.
func encryptAndGetKeystore(data, auth []byte) (cryptoParams, error) {
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

// CheckForKYC helps in checking the KYC information
func CheckForKYC(userDefAddr, moiIDBaseURL string) (bool, error) {
	// preparing request payload for checkForKYC
	checkForKYCPayload, err := json.Marshal(map[string]string{
		"defAddr":   userDefAddr,
		"nameSpace": "validator",
	})
	if err != nil {
		return false, err
	}

	requestBody := bytes.NewBuffer(checkForKYCPayload)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: MaxTimeForConnect,
			}).DialContext,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), MaxTimeForRequest)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		moiIDBaseURL+"/moi-id/digitalme/checkForKYC",
		requestBody)
	req.Header.Set("Content-Type", "application/json")

	if err != nil {
		return false, err
	}

	checkForKYCResponse, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer checkForKYCResponse.Body.Close()

	checkForKYCResponseInBytes, err := ioutil.ReadAll(checkForKYCResponse.Body)
	if err != nil {
		return false, err
	}

	// Response from above end-point
	type responseFromCheckForKYC struct {
		Status    string `json:"status"`
		Message   string `json:"message"`
		OtherData string `json:"otherData"`
	}

	var parseResponse responseFromCheckForKYC
	err = json.Unmarshal(checkForKYCResponseInBytes, &parseResponse)

	if err != nil {
		return false, err
	}

	if parseResponse.Status == "failed" {
		return false, errors.New("\n" + parseResponse.Message + "\n\n" + parseResponse.OtherData)
	}

	return true, nil
}
