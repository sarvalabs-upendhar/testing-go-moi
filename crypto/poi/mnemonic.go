package poi

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/scrypt"

	"github.com/sarvalabs/moichain/crypto/common"
)

type authParams struct {
	Cipher map[string]string `json:"cipher"`
	KDF    struct {
		Algo      string    `json:"algorithm"`
		KDFParams kdfParams `json:"dfparams"`
	} `json:"kdf"`
}

type kdfParams struct {
	Salt string `json:"salt"`
	N    int    `json:"n"`
	Len  int    `json:"dklen"`
	P    int    `json:"p"`
	R    int    `json:"r"`
}

type MnemonicKeystore struct {
	Version            int        `json:"version"`
	Auth               authParams `json:"authenticity"`
	Mac                string     `json:"mac"`
	MnenomicCipherText string     `json:"mnemonicCiphertext"`
}

func (seed *MnemonicKeystore) Unmarshall(mkeystoreBytes []byte) error {
	err := json.Unmarshal(mkeystoreBytes, seed)
	if err != nil {
		return err
	}

	return nil
}

func GetMnemonicKeystore(dataDir string) ([]byte, error) {
	mksFilePath := strings.Join([]string{dataDir, MnemonicKeystorePath}, "/")

	mksContent, err := os.ReadFile(mksFilePath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil, common.ErrNoMnemonicKeystore
		}

		return nil, err
	}

	return mksContent, nil
}

// checkAuthenticity is to verify the authenticity of the keystore being altered
func checkAuthenticity(cipherString, macString string, macCipher []byte) error {
	cipherText, err := hex.DecodeString(cipherString)
	if err != nil {
		return err
	}

	mac, err := hex.DecodeString(macString)
	if err != nil {
		return err
	}

	calculatedMAC := common.GetKeccak256Hash(macCipher, cipherText)

	if !bytes.Equal(calculatedMAC, mac) {
		return errors.New("authenticity failed! Make sure you pass correct password")
	}

	return nil
}

// getDerivedKey used to derive the Key used while constructing the keystore
func getDerivedKey(cryptoJSON authParams, passPhrase string) ([]byte, error) {
	var key []byte

	authArray := []byte(passPhrase)

	salt, err := hex.DecodeString(cryptoJSON.KDF.KDFParams.Salt)
	if err != nil {
		return key, err
	}

	if cryptoJSON.KDF.Algo == keyHeaderKDF {
		n := cryptoJSON.KDF.KDFParams.N
		r := cryptoJSON.KDF.KDFParams.R
		p := cryptoJSON.KDF.KDFParams.P

		key, err = scrypt.Key(authArray, salt, n, r, p, 64)
		if err != nil {
			return nil, errors.New("error deriving the key from given crypto params")
		}

		return key, nil
	}

	return key, fmt.Errorf("unsupported KDF: %s", cryptoJSON.KDF.Algo)
}

func (seed *Mnemonic) FromString(seedPhrase string) error {
	twelvePhrases := strings.Split(seedPhrase, " ")

	if len(twelvePhrases) == 12 {
		copy(seed[:], twelvePhrases)
	} else {
		return errors.New("invalid length for mnemonic")
	}

	return nil
}

func (seed *Mnemonic) FromKeystore(keystoreBytes []byte, passPhrase string) error {
	mks := MnemonicKeystore{}
	if err := mks.Unmarshall(keystoreBytes); err != nil {
		return err
	}

	dKey, err := getDerivedKey(mks.Auth, passPhrase)
	if err != nil {
		return err
	}

	if err := checkAuthenticity(mks.Auth.Cipher["ciphertext"], mks.Mac, dKey[16:32]); err != nil {
		return err
	}

	mnemonicCipherText, _ := hex.DecodeString(mks.MnenomicCipherText)
	mnemonicCounter := mks.Auth.Cipher["iv"]

	mnemonicIV, err := hex.DecodeString(mnemonicCounter)
	if err != nil {
		return err
	}

	mnemomicEncKey := dKey[32:]

	mnemonicEntropy, err := aesCTRXOR(mnemomicEncKey, mnemonicCipherText, mnemonicIV)
	if err != nil {
		return err
	}

	seedPhrase, err := bip39.NewMnemonic(mnemonicEntropy)
	if err != nil {
		return err
	}

	if err := seed.FromString(seedPhrase); err != nil {
		return err
	}

	return nil
}

func (seed Mnemonic) String() string {
	return strings.Join(seed[:], " ")
}
