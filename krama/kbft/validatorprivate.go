package kbft

import (
	"crypto"
	"crypto/ed25519"
	crand "crypto/rand"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gitlab.com/sarvalabs/moichain/types"
)

type PrivateValidator interface {
	GetPubKey() PublicKey
	SignVote(v *types.Vote) error
}

type TestPrivateValidator struct {
	PrivateKey crypto.PrivateKey
	// PublicKey  signature.PublicKey
}

func NewTestPrivateValidator(path string) (*TestPrivateValidator, error) {
	v := new(TestPrivateValidator)
	keyFile := filepath.Join(path, "/seed.key")

	if err := v.acquireSeed(keyFile); err != nil {
		return nil, err
	}

	return v, nil
}

func (pv *TestPrivateValidator) GetPubKey() PublicKey {
	if privKey, ok := pv.PrivateKey.(ed25519.PrivateKey); ok {
		pb := &MOIPublicKey{privKey.Public()}

		return pb
	}

	return nil
}

func (pv *TestPrivateValidator) SignVote(v *types.Vote) error {
	if privKey, ok := pv.PrivateKey.(ed25519.PrivateKey); ok {
		rawBytes := v.Bytes()
		v.Signature = ed25519.Sign(privKey, rawBytes)

		return nil
	}

	return errors.New("private key not of type ed25519.PrivateKey")
}

func (pv *TestPrivateValidator) acquireSeed(keyfile string) error {
	// Check if the key file exists at the given path
	if _, err := os.Stat(keyfile); !os.IsNotExist(err) {
		// Keyfile already exists
		log.Println("Found an existing key")

		// Read the keyfile into bytes data and check for errors
		data, err := ioutil.ReadFile(keyfile)
		if err != nil {
			// Return the error
			return err
		}

		// Generate private key from seed
		pv.PrivateKey = ed25519.NewKeyFromSeed(data)
	} else {
		// Keyfile does not exist
		log.Println("Generating new key")

		// Generate a new RSA keypair and check for errors
		_, key, err := ed25519.GenerateKey(crand.Reader)
		if err != nil {
			// Return the error
			return err
		}

		// Write the private key data to keyfile and check for errors
		if err := ioutil.WriteFile(keyfile, key.Seed(), 0o600); err != nil {
			// Return the error
			return err
		}
		pv.PrivateKey = key
	}

	// Return the key and a nil error
	return nil
}
