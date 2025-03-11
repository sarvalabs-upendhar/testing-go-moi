package bls

import (
	"encoding/hex"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/crypto/common"
	"github.com/stretchr/testify/require"
	blst "github.com/supranational/blst/bindings/go"
)

const (
	expectedSig = "0460b776cf0407a74559f4e696fedf5990294794915be00e14e81cab41a2ea49bd4d4d5d0f45" +
		"0aaae872232e38019320f7ad19954814cb53b13b17be262ddf99251af7d4509af52f6f1dcc27b732" +
		"c0d0216e93f4e057c47fc058f4aa201f80d40b6a"
	// TODO: FIX ME
	kid = identifiers.KramaID("1116Uiu2HAmGZr9gyQ7fDdmdBsRL29EjxR81Y74TEPbemBkyKuk2Ufj")
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

// variable for multi-sig
var (
	pk1S256, _ = hexutil.Decode("0x8491a74ba065adb16c36fe05fc88d04ddfe10603a2e7c703064a63ab9b939edc")
	pk1Bls     = blst.KeyGen(pk1S256)
	skA        = new(blst.SecretKey).Deserialize(pk1Bls.Serialize())

	pk2S256, _ = hexutil.Decode("0xe6ebdb53f3782fd08ef97579fde81492bb823b6f53b20adf6ad62e5cdbdd6fd0")
	pk2Bls     = blst.KeyGen(pk2S256)
	skB        = new(blst.SecretKey).Deserialize(pk2Bls.Serialize())

	pk3S256, _ = hexutil.Decode("0x509deefbb0dfe44db6e8e72f17b283f660b23ad7aeaf238159f27a5a38ce6853")
	pk3Bls     = blst.KeyGen(pk3S256)
	skC        = new(blst.SecretKey).Deserialize(pk3Bls.Serialize())

	msgsArr     = [][]byte{[]byte("message_1"), []byte("message_2"), []byte("message_3")}
	wrongMsgArr = [][]byte{[]byte("wrong_message_1"), []byte("wrong_message_2"), []byte("wrong_message_3")}

	// Public keys corresponding to above secret keys
	pubKeyA = new(blst.P1Affine).From(skA)
	pubKeyB = new(blst.P1Affine).From(skB)
	pubKeyC = new(blst.P1Affine).From(skC)
)

func TestBLSSignAndVerify(t *testing.T) {
	tests := []struct {
		name           string
		message        []byte
		privateKey     []byte
		expectedSig    string
		expectingError bool
	}{
		{
			name:        "Valid BLS Sign",
			message:     sampleMessage,
			privateKey:  samplePrivateKey,
			expectedSig: expectedSig,
		},
		{
			name:           "Invalid BLS Sign with wrong private key",
			message:        sampleMessage,
			privateKey:     []byte{1, 2, 3}, // some invalid key
			expectedSig:    "",
			expectingError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bsig BlsWithBlstSignature

			err := bsig.Sign(test.message, test.privateKey, kid)
			if test.expectingError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			sigInHex := hex.EncodeToString(common.MarshalSignature(common.Signature(bsig)))
			require.Equal(t, test.expectedSig, sigInHex)
		})
	}
}

func TestBLSVerify(t *testing.T) {
	tests := []struct {
		name           string
		message        []byte
		publicKey      []byte
		signature      string
		expectingError bool
		expectingValid bool
	}{
		{
			name:           "Valid BLS Verify",
			message:        sampleMessage,
			publicKey:      blsPublicKey,
			signature:      expectedSig,
			expectingError: false,
			expectingValid: true,
		},
		{
			name:           "Invalid BLS Verify with wrong message",
			message:        []byte("invalid message"),
			publicKey:      blsPublicKey,
			signature:      expectedSig,
			expectingError: false,
			expectingValid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sigInHexBytes, err := hex.DecodeString(test.signature)
			require.NoError(t, err)

			bsigGeneral, err := common.UnmarshalSignature(sigInHexBytes)
			require.NoError(t, err)

			bsig := BlsWithBlstSignature(bsigGeneral)

			verificationBool, err := bsig.Verify(test.message, test.publicKey)
			require.NoError(t, err)
			require.Equal(t, test.expectingValid, verificationBool)
		})
	}
}

func TestBlsMultiSig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		messages        [][]byte
		expectedSuccess bool
		publicKeys      [][]byte
	}{
		{
			name:            "Positive case",
			messages:        msgsArr,
			expectedSuccess: true,
			publicKeys:      [][]byte{pubKeyA.Serialize(), pubKeyB.Serialize(), pubKeyC.Serialize()},
		},
		{
			name:            "Wrong messages case",
			messages:        wrongMsgArr,
			expectedSuccess: false,
			publicKeys:      [][]byte{pubKeyA.Serialize(), pubKeyB.Serialize(), pubKeyC.Serialize()},
		},
		{
			name:            "Incorrect order case",
			messages:        msgsArr,
			expectedSuccess: false,
			publicKeys:      [][]byte{pubKeyA.Serialize(), pubKeyC.Serialize(), pubKeyB.Serialize()},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sigA, sigB, sigC BlsWithBlstSignature

			err := sigA.Sign(msgsArr[0], skA.Serialize(), kid)
			require.NoError(t, err)

			err = sigB.Sign(msgsArr[1], skB.Serialize(), kid)
			require.NoError(t, err)

			err = sigC.Sign(msgsArr[2], skC.Serialize(), kid)
			require.NoError(t, err)

			sigBytes, err := AggregateSignatures([]BlsWithBlstSignature{sigA, sigB, sigC})
			require.NoError(t, err)

			validation, err := VerifyMultiSig(sigBytes, tt.messages, tt.publicKeys)
			require.NoError(t, err)
			require.Equal(t, tt.expectedSuccess, validation)
		})
	}
}
