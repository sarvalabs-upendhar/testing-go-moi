package bls

import (
	"encoding/hex"
	"testing"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/crypto/common"
)

const (
	expectedSig = "0460b776cf0407a74559f4e696fedf5990294794915be00e14e81cab41a2ea49bd4d4d5d0f45" +
		"0aaae872232e38019320f7ad19954814cb53b13b17be262ddf99251af7d4509af52f6f1dcc27b732" +
		"c0d0216e93f4e057c47fc058f4aa201f80d40b6a"
)

var (
	samplePrivateKey = []byte{
		74, 29, 73, 110, 113, 40, 77, 200, 240, 44, 78, 5, 215, 168, 91, 122,
		196, 105, 7, 34, 164, 40, 206, 94, 38, 170, 172, 254, 241, 46, 229, 78,
	}

	sampleMessage = []byte("Hello MOI user, this is test string being signed")

	blsPublicKey = []byte{
		176, 19, 70, 163, 32, 1, 85, 50, 125, 112, 18, 148, 16, 147, 111, 212,
		99, 190, 29, 171, 191, 60, 170, 30, 13, 211, 180, 43, 185, 16, 118, 95, 164, 26,
		243, 196, 224, 45, 103, 74, 183, 34, 109, 233, 182, 191, 42, 37,
	}
)

func TestBLSSign(t *testing.T) {
	t.Parallel()

	var bsig BlsWithBlstSignature

	kid := kramaid.KramaID("bvby3pBVU5BEL2jBHJrH23GTb9qe8nL4XHqqKzZVbth7gBZ5c3.16Uiu2HAmGZr9gyQ7fD" +
		"dmdBsRL29EjxR81Y74TEPbemBkyKuk2Ufj")

	err := bsig.Sign(sampleMessage, samplePrivateKey, kid)
	require.NoError(t, err)

	sigInHex := hex.EncodeToString(common.MarshalSignature(common.Signature(bsig)))
	require.Equal(t, expectedSig, sigInHex)
}

func TestBLSVerify(t *testing.T) {
	t.Parallel()

	sigInHexBytes, err := hex.DecodeString(expectedSig)
	require.NoError(t, err)

	bsigGeneral, err := common.UnmarshalSignature(sigInHexBytes)
	require.NoError(t, err)

	bsig := BlsWithBlstSignature(bsigGeneral)

	verificationBool, err := bsig.Verify(sampleMessage, blsPublicKey)
	require.NoError(t, err)

	require.Equal(t, true, verificationBool)
}
