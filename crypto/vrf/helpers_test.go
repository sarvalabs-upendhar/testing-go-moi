package vrf

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	blst "github.com/supranational/blst/bindings/go"
)

func generateRandomSecretKey(t *testing.T) *blst.SecretKey {
	t.Helper()

	var ikm [32]byte

	_, err := rand.Read(ikm[:])
	require.NoError(t, err)

	return blst.KeyGen(ikm[:])
}

// Helper function to generate VRFs and proofs for given inputs
func generateVrfsAndProofs(t *testing.T, vrfPrivKey *PrivateKey, vrfInputs [][]byte) ([][32]byte, [][]byte) {
	t.Helper()

	vrfs := make([][32]byte, 0)
	proofs := make([][]byte, 0)

	for _, vrfInput := range vrfInputs {
		vrf, proof, err := vrfPrivKey.Evaluate(vrfInput)
		require.NoError(t, err)

		vrfs = append(vrfs, vrf)
		proofs = append(proofs, proof)
	}

	return vrfs, proofs
}
