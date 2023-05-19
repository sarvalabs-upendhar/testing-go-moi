package mudra

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
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
		DataDir:      datadir1,
		NodePassword: "nodepass1",
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

func TestKramaVaultWithoutAnyMode(t *testing.T) {
	datadir1, err := ioutil.TempDir("", "random")
	require.NoError(t, err)

	config := &VaultConfig{
		DataDir:      datadir1,
		NodePassword: "nodepass1",
	}

	_, err = NewVault(config, moinode.MoiFullNode, 1)
	require.ErrorIs(t, common.ErrNoKeystore, err)
}

func TestRegisterModeWithoutMnemomic(t *testing.T) {
	datadir, err := ioutil.TempDir("", "moichain")
	require.NoError(t, err)

	config := &VaultConfig{
		DataDir:      datadir,
		NodePassword: "nodepass1",
		Mode:         1,
	}

	_, err = NewVault(config, moinode.MoiFullNode, 1)
	require.ErrorIs(t, common.ErrMnemonicMandatory, err)
}

func TestKramaVaultRegisterMode(t *testing.T) {
	datadir, err := ioutil.TempDir("", "moichain")
	require.NoError(t, err)

	config := &VaultConfig{
		DataDir:      datadir,
		NodePassword: "nodepass1",
		Mode:         1,
		SeedPhrase:   "unlock element young void mass casino suffer twin earth drill aerobic tooth",
	}

	vault, err := NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	moiIDStringFromSetup, err := vault.MOiID()
	require.NoError(t, err)
	kramaIDFromSetup, err := vault.MOiID()
	require.NoError(t, err)

	config1 := &VaultConfig{
		DataDir:      datadir,
		NodePassword: "nodepass1",
	}

	vault1, err := NewVault(config1, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	moiIDStringFromStart, err := vault1.MOiID()
	require.NoError(t, err)
	kramaIDFromStart, err := vault1.MOiID()
	require.NoError(t, err)

	require.Equal(t, moiIDStringFromSetup, moiIDStringFromStart)
	require.Equal(t, kramaIDFromSetup, kramaIDFromStart)
}
