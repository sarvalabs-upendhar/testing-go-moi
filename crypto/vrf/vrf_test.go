package vrf

import (
	"testing"

	_ "github.com/sarvalabs/go-moi/common"
	"github.com/stretchr/testify/require"
	blst "github.com/supranational/blst/bindings/go"
)

func TestPublicKey_ProofToHash(t *testing.T) {
	sk := generateRandomSecretKey(t)
	pk := new(blst.P1Affine).From(sk)

	vrfPrivKey := NewVRFSigner(sk)
	vrfPubKey := NewVRFVerifier(pk)

	vrfOutput, proof, _ := vrfPrivKey.Evaluate([]byte("data"))

	tests := []struct {
		name     string
		vrfProof []byte
		valid    bool
	}{
		{
			name:     "valid proof",
			vrfProof: proof,
			valid:    true,
		},
		{
			name:     "left truncated proof",
			vrfProof: proof[1:],
			valid:    false,
		},
		{
			name:     "right truncated proof",
			vrfProof: proof[:len(proof)-1],
			valid:    false,
		},
		{
			name:     "invalid proof",
			vrfProof: []byte("invalid_proof"),
			valid:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := vrfPubKey.ProofToHash([]byte("data"), test.vrfProof)

			if !test.valid {
				require.Error(t, err)
				require.Equal(t, err, ErrInvalidVRF)

				return
			}

			require.NoError(t, err)
			require.Equal(t, vrfOutput, output)
		})
	}
}

func TestPublicKey_Verify(t *testing.T) {
	sk := generateRandomSecretKey(t)
	pk := new(blst.P1Affine).From(sk)

	vrfPrivKey := NewVRFSigner(sk)
	vrfPubKey := NewVRFVerifier(pk)

	vrfInputs := [][]byte{[]byte("data1"), []byte("data2"), []byte("data2")}
	vrfs, proofs := generateVrfsAndProofs(t, vrfPrivKey, vrfInputs)

	tests := []struct {
		name      string
		vrfInput  []byte
		vrfOutput [32]byte
		vrfProof  []byte
		valid     bool
	}{
		{
			name:      "Validating proof of data1",
			vrfInput:  vrfInputs[0],
			vrfOutput: vrfs[0],
			vrfProof:  proofs[0],
			valid:     true,
		},
		{
			name:      "Validating proof of data2",
			vrfInput:  vrfInputs[1],
			vrfOutput: vrfs[1],
			vrfProof:  proofs[1],
			valid:     true,
		},
		{
			name:      "Validating proof of data3",
			vrfInput:  vrfInputs[2],
			vrfOutput: vrfs[2],
			vrfProof:  proofs[2],
			valid:     true,
		},
		{
			name:      "Validating proof of data1 with data2 input",
			vrfInput:  vrfInputs[1],
			vrfOutput: vrfs[1],
			vrfProof:  proofs[0],
			valid:     false,
		},
		{
			name:      "Validation proof of data3 with data1 input",
			vrfInput:  vrfInputs[0],
			vrfOutput: vrfs[0],
			vrfProof:  proofs[2],
			valid:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isValid, err := vrfPubKey.Verify(test.vrfOutput, test.vrfInput, test.vrfProof)

			if !test.valid {
				require.Error(t, err)
				require.Equal(t, err, ErrInvalidVRF)

				return
			}

			require.NoError(t, err)
			require.True(t, isValid)
		})
	}
}
