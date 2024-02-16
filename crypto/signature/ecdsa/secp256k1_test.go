package ecdsa

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/crypto/common"
)

var (
	privateKeyInHex   = "d4f2f4e558d484d42e06593e72c50af87dbb9b1b2b2128f35e8c0b6c8f3e1582"
	pubKeyInHex       = "5f2c7306be02b16d0f1ae75ae3fdbedf10b970d98c7646ec5e9beaf325a2e004"
	message           = []byte("Hello MOI user, this is the message being signed")
	expectedSignature = "0146304402201b2f03875387cb1964d70d414bae4fc15fc9b244967d973676a" +
		"7087e628e15bc02207ee005106a92fa003e877ee5d773ded75ac6317672fc3b27def321be3b18d7ca03"
)

func TestECDSASignWithSecp256k1(t *testing.T) {
	t.Parallel()

	var s256 EcdsaSecp256k1Signature

	privateKeyBytes, _ := hex.DecodeString(privateKeyInHex)
	kid := kramaid.KramaID("bvby3pBVU5BEL2jBHJrH23GTb9qe8nL4XHqqKzZVbth7gBZ5c3." +
		"16Uiu2HAmGZr9gyQ7fDdmdBsRL29EjxR81Y74TEPbemBkyKuk2Ufj")

	err := s256.Sign(message, privateKeyBytes, kid)
	if err != nil {
		t.Error("signing with secp256k1 key failed")
	}

	rawSig := common.MarshalSignature(common.Signature(s256))
	rawSigInHex := hex.EncodeToString(rawSig)

	require.Equal(t,
		expectedSignature,
		rawSigInHex,
	)
}

func TestECDSAVerifyWithSecp256k1(t *testing.T) {
	t.Parallel()

	pubKeyBytes, _ := hex.DecodeString(pubKeyInHex)
	sigInHexBytes, err := hex.DecodeString(expectedSignature)
	require.NoError(t, err)

	sig, err := common.UnmarshalSignature(sigInHexBytes)
	require.NoError(t, err)

	s256 := EcdsaSecp256k1Signature(sig)

	verificationBool, err := s256.Verify(message, pubKeyBytes)
	require.NoError(t, err)
	require.Equal(t, true, verificationBool)

	privateKeyBytes, _ := hex.DecodeString(privateKeyInHex)
	_, publicKey := btcec.PrivKeyFromBytes(privateKeyBytes)
	verificationBoolFromPk, err := s256.Verify(message, publicKey.SerializeCompressed())
	require.NoError(t, err)
	require.Equal(t, true, verificationBoolFromPk)
}
