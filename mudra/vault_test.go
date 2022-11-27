package mudra

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	"github.com/stretchr/testify/require"
)

func TestBLSSignAgg(t *testing.T) {
	// Validator 1 DataDir
	datadir1, err := ioutil.TempDir("", "testDataDir")
	require.NoError(t, err)

	// Validator 2 DataDir
	datadir2, err := ioutil.TempDir("", "testDataDir")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(datadir1)
		os.RemoveAll(datadir2)
	})

	// Validator 1 Init
	_, _, err = poi.RandGenKeystore(datadir1, "nodepass1")
	require.NoError(t, err)

	config := &VaultConfig{
		DataDir:       datadir1,
		NodePassword:  "nodepass1",
		MoiIDUsername: "raman",
		MoiIDPassword: "Test@123",
		MoiIDURL:      "dev",
	}

	vault1, err := NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	// Public Key of Validator 1
	pub1 := vault1.GetConsensusPrivateKey().GetPublicKeyInBytes()

	// Validator 2 Init
	_, _, err = poi.RandGenKeystore(datadir2, "nodepass2")
	require.NoError(t, err)

	config.DataDir = datadir2
	config.NodePassword = "nodepass2"

	vault2, err := NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	// Public Key of Validator 2
	pub2 := vault2.GetConsensusPrivateKey().GetPublicKeyInBytes()

	msg := []byte("I'm getting signed")
	mulSigs := make([][]byte, 2)

	// Signing at Validator 1
	sigBytes1, err := vault1.Sign(msg, common.BlsBLST)
	require.NoError(t, err)

	mulSigs[0] = sigBytes1

	// Signing at Validator 2
	sigBytes2, err := vault2.Sign(msg, common.BlsBLST)
	require.NoError(t, err)

	mulSigs[1] = sigBytes2

	aggSig, err := AggregateSignatures(mulSigs)
	require.NoError(t, err)

	validationStatus, err := VerifyAggregateSignature(msg, aggSig, [][]byte{pub1, pub2})
	require.NoError(t, err)

	require.Equal(t, true, validationStatus)
}
