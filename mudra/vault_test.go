package mudra

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/mudra/common"
	"gitlab.com/sarvalabs/moichain/mudra/poi"
	"gitlab.com/sarvalabs/moichain/mudra/poi/moinode"
)

func TestBLSSignAgg(t *testing.T) {
	// Validator 1 DataDir
	datadir1, err := ioutil.TempDir("", "testDataDir")
	if err != nil {
		log.Fatal(err)
	}

	// Validator 2 DataDir
	datadir2, err := ioutil.TempDir("", "testDataDir")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("DataDir 1: ", datadir1)
	fmt.Println("DataDir 2: ", datadir2)

	defer os.RemoveAll(datadir1)
	defer os.RemoveAll(datadir2)

	// Validator 1 Init
	_, _, err = poi.RandGenKeystore(datadir1, "nodepass1")
	if err != nil {
		log.Panicln(err)
	}

	config := &VaultConfig{
		DataDir:       datadir1,
		NodePassword:  "nodepass1",
		MoiIDUsername: "raman",
		MoiIDPassword: "Test@123",
		MoiIDURL:      "dev",
	}

	vault1, err := NewVault(config, moinode.MoiFullNode, 1)
	if err != nil {
		log.Panicln(err)
	}
	// Public Key of Validator 1
	pub1 := vault1.GetConsensusPrivateKey().GetPublicKeyInBytes()

	// Validator 2 Init
	_, _, err = poi.RandGenKeystore(datadir2, "nodepass2")
	if err != nil {
		log.Panicln(err)
	}

	config.DataDir = datadir2
	config.NodePassword = "nodepass2"

	vault2, err := NewVault(config, moinode.MoiFullNode, 1)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Public Key of Validator 2
	pub2 := vault2.GetConsensusPrivateKey().GetPublicKeyInBytes()

	msg := []byte("I'm getting signed")
	mulSigs := make([][]byte, 2)

	// Signing at Validator 1
	sigBytes1, err := vault1.Sign(msg, common.BlsBLST)
	if err != nil {
		t.Fatalf("%v", err)
	}

	mulSigs[0] = sigBytes1

	// Signing at Validator 2
	sigBytes2, err := vault2.Sign(msg, common.BlsBLST)
	if err != nil {
		t.Fatalf("%v", err)
	}

	mulSigs[1] = sigBytes2

	aggSig, err := AggregateSignatures(mulSigs)
	if err != nil {
		t.Fatalf("%v", err)
	}

	validationStatus, err := VerifyAggregateSignature(msg, aggSig, [][]byte{pub1, pub2})
	if err != nil {
		t.Fatalf("%v", err)
	}

	require.Equal(t, true, validationStatus)
}
