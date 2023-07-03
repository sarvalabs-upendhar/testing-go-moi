package mudra

import (
	hexutil "encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
)

var msg = []byte("I'm getting signed")

const (
	blsSignSample = "0460a4612b9f516ad866663d2c7e4a9580aecf24bb6c10f6c1a06838" +
		"5cf0c03fe8f181617fffc340b9c0f5da438ff374c9b40a2cde5fa14280cde318cf7164e669c1" +
		"28a4874cc19a5b5f67b1488f2b8c63911079add069f387b9361cbf87d0f5cefc"

	ecdsaSignSample = "01473045022100e6823cc24ea8ab0dff424efc35c1a58fa7a5d7f744dca0848ecfcabd11b43" +
		"c550220115ec0005a878b5c2de44e5e90458ee89e18720f33636929d89b238131179d0503"

	testMnemonic = "unlock element young void mass casino suffer twin earth drill aerobic tooth"
	testMoiID    = "03e0c762f9f5e47395559346f4f780329c49eebd0a53cbb69c3cb3117ff4e0e24f"
)

var testKramaID = kramaid.KramaID("3WzFwwvSz7ZwiU3cDwk7uZtc9gX4d5h18MhsmXaT1XVqa3Bv16pP" +
	"." + "16Uiu2HAkzBJTbFo1FXxvLy9qBWeP3z5zP6bkkPheXzk7HVgdW4xR")

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

	t.Cleanup(func() {
		os.RemoveAll(datadir)
	})

	config := &VaultConfig{
		DataDir:      datadir,
		NodePassword: "nodepass1",
		Mode:         1,
		SeedPhrase:   testMnemonic,
	}

	vault, err := NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	testECDSASignWithOptions(t, vault)

	moiIDStringFromSetup, err := vault.MoiID()
	require.NoError(t, err)
	require.Equal(t, moiIDStringFromSetup, testMoiID)

	moiIDPubKey, err := vault.MoiIDPublicKey()
	require.NoError(t, err)

	_, derivedMoiIDPubKey, err := poi.GetPrivateKeyAtPath(testMnemonic, DefaultMOIIDPath)
	require.NoError(t, err)

	require.Equal(t, moiIDPubKey, derivedMoiIDPubKey)

	kramaIDFromSetup := vault.KramaID()
	require.Equal(t, kramaIDFromSetup, testKramaID)

	config1 := &VaultConfig{
		DataDir:      datadir,
		NodePassword: "nodepass1",
	}

	vault1, err := NewVault(config1, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	moiIDStringFromStart, err := vault1.MoiID()
	require.NoError(t, err)

	kramaIDFromStart := vault1.KramaID()

	require.Equal(t, moiIDStringFromSetup, moiIDStringFromStart)
	require.Equal(t, kramaIDFromSetup, kramaIDFromStart)

	testBLSSign(t, vault1)
	testECDSASign(t, vault1)
}

func testBLSSign(t *testing.T, vault *KramaVault) {
	t.Helper()
	fmt.Print("Testing BLS Signing")

	sigBytes1, err := vault.Sign(msg, common.BlsBLST)
	require.NoError(t, err)
	require.Equal(t, hexutil.EncodeToString(sigBytes1), blsSignSample)
	fmt.Println(": ✓")
}

func testECDSASign(t *testing.T, vault *KramaVault) {
	t.Helper()
	fmt.Print("Testing ECDSA Signing")

	_, err := vault.Sign(msg, common.EcdsaSecp256k1)
	require.ErrorIs(t, common.ErrSignOptionsNotPassed, err)
	fmt.Println(": ✓")
}

func testECDSASignWithOptions(t *testing.T, vault *KramaVault) {
	t.Helper()
	fmt.Print("Testing ECDSA Signing with SignOptions")

	sigBytes, err := vault.Sign(msg, common.EcdsaSecp256k1, UsingIgcPath("m/44'/6174'/9023'/0/0"))
	require.NoError(t, err)
	require.Equal(t, hexutil.EncodeToString(sigBytes), ecdsaSignSample)
	fmt.Println(": ✓")
}

func TestSignWithNetworkKey(t *testing.T) {
	datadir1, err := ioutil.TempDir("", "testDataDir")
	require.NoError(t, err)

	_, _, err = poi.RandGenKeystore(datadir1, "nodepass1")
	require.NoError(t, err)

	vConfig := &VaultConfig{
		DataDir:      datadir1,
		NodePassword: "nodepass1",
	}

	vault, err := NewVault(vConfig, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	signOptions := UsingNetworkKey()
	sigBytes, err := vault.Sign(msg, common.EcdsaSecp256k1, signOptions)
	require.NoError(t, err)

	pubKey := vault.GetNetworkPrivateKey().GetPublicKeyInBytes()

	verificationStatus, err := Verify(msg, sigBytes, pubKey)
	require.NoError(t, err)

	require.Equal(t, true, verificationStatus)
}
