package poi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	"gitlab.com/sarvalabs/moichain/mudra/common"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/scrypt"
)

// checkAuthenticity is to verify the authenticity of the keystore being altered
func checkAuthenticity(cipherString, macString string, macCipher []byte) (bool, error) {
	cipherText, err := hex.DecodeString(cipherString)
	if err != nil {
		return false, err
	}

	mac, err := hex.DecodeString(macString)
	if err != nil {
		return false, err
	}

	calculatedMAC := common.GetKeccak256Hash(macCipher, cipherText)
	if !bytes.Equal(calculatedMAC, mac) {
		return false, common.ErrInDecryption
	}

	return true, nil
}

// getKDFKey used to derive the Key used while constructing the wallet
func getKDFKey(cryptoJSON cryptoParams, auth string) (SRPPrivateBytes, error) {
	var srb SRPPrivateBytes

	authArray := []byte(auth)

	salt, err := hex.DecodeString(cryptoJSON.KDFParams["salt"].(string))
	if err != nil {
		return srb, err
	}

	if cryptoJSON.KDF == keyHeaderKDF {
		n := mParseInt(cryptoJSON.KDFParams["n"])
		r := mParseInt(cryptoJSON.KDFParams["r"])
		p := mParseInt(cryptoJSON.KDFParams["p"])

		derivedKey, err := scrypt.Key(authArray, salt, n, r, p, SrpdkLen)
		if err != nil {
			log.Fatal("error deriving the private bytes of SRP")
		}

		if err = srb.FromBytes(derivedKey); err != nil {
			return srb, err
		}

		return srb, nil
	}

	return srb, fmt.Errorf("unsupported KDF: %s", cryptoJSON.KDF)
}

// getSRPFromEncryptedJSON used to derive the Mnemonic from wallet keystore
//
// NOTE: There is NO go implementation(Upto my research) providing functionality to derive mnemonic
// from wallet keystore. This is our own implementation.
func getSRPFromEncryptedJSON(walletKeystore WalletKeystoreJSON, auth string) (*string, error) {
	srpPrivateBytes, err := getKDFKey(walletKeystore.Crypto, auth)
	if err != nil {
		return nil, err
	}

	pathKey := srpPrivateBytes.getPathKey()
	_, err = checkAuthenticity(walletKeystore.Crypto.CipherText, walletKeystore.Crypto.MAC, pathKey[16:32])

	if err != nil {
		return nil, err
	}

	srpCipherText, _ := hex.DecodeString(walletKeystore.SRP.CipherText)

	srpIV, err := hex.DecodeString(walletKeystore.SRP.Counter)
	if err != nil {
		return nil, err
	}

	mnemomicPrivateBytes := srpPrivateBytes.getMnemonicKey()

	srpEntropy, err := aesCTRXOR(mnemomicPrivateBytes, srpCipherText, srpIV)
	if err != nil {
		return nil, err
	}

	seed, err := bip39.NewMnemonic(srpEntropy)
	if err != nil {
		return nil, err
	}

	return &seed, nil
}
