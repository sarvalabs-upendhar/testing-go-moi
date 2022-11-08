package mudra

import (
	"fmt"
	"log"
	"testing"

	"gitlab.com/sarvalabs/moichain/mudra/common"
	"gitlab.com/sarvalabs/moichain/mudra/poi"
	"gitlab.com/sarvalabs/moichain/mudra/poi/moinode"
)

/*
func TestKramaVault_New(t *testing.T) {
	vault := KramaVault{}
	datadir := "/Users/sarvatechdeveloper1/.moi/moinode-new-4"
	err := vault.New(datadir, "vmvm1234", "dev-050122", "Test@123", "dev", moinode.MOI_FULL_NODE, 1)
	if err != nil {
		t.Fatalf("%v", err)
	}

	t.Log("Private Key for consensus: ", vault.consensusPriv.Bytes())
	t.Log("Private Key for network: ", vault.networkPriv.Bytes())
	t.Log("KramaID: ", vault.KramaID())
}

func TestBLSSignAndVerify(t *testing.T) {
	vault := KramaVault{}
	datadir := "/Users/sarvatechdeveloper1/.moi/moinode-new-4"
	err := vault.New(datadir, "vmvm1234", "dev-050122", "Test@123", "dev", moinode.MOI_FULL_NODE, 1)
	if err != nil {
		t.Fatalf("%v", err)
	}

	msg := []byte("I'm getting signed")
	sigBytes, err := vault.Sign(msg, common.BLS_BLST)
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Log("Signature: ", sigBytes)

	pubKey := vault.consensusPriv.GetPublicKeyInBytes()
	verificationSuccess, err := vault.Verify(msg, sigBytes, pubKey)
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Log("Verification Successful? : ", verificationSuccess)
}
*/

func TestBLSSignAgg(t *testing.T) {
	// Validator 1
	_, _, err := poi.RandGenKeystore(fmt.Sprintf("hoola_%d", 1), "vmvm1234")
	if err != nil {
		log.Panicln(err)
	}

	datadir1 := "./hoola_1"

	config := &VaultConfig{
		DataDir:       datadir1,
		NodePassword:  "vmvm1234",
		MoiIDUsername: "dev-050122z",
		MoiIDPassword: "Test@123",
		MoiIDURL:      "dev",
	}

	vault1, err := NewVault(config, moinode.MoiFullNode, 1)
	if err != nil {
		log.Panicln(err)
	}
	// Public Key of Validator 1
	pub1 := vault1.GetConsensusPrivateKey().GetPublicKeyInBytes()

	// Validator 2
	_, _, err = poi.RandGenKeystore(fmt.Sprintf("hoola_%d", 2), "yiyi")
	if err != nil {
		log.Panicln(err)
	}

	datadir2 := "./hoola_2"
	config.DataDir = datadir2
	config.NodePassword = "yiyi"

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

	fmt.Println(VerifyAggregateSignature(msg, aggSig, [][]byte{pub1, pub2}))
}

//func TestKramaVault_New(t *testing.T) {
//	vault := KramaVault{}
//	datadir := "/Users/sarvatechdeveloper1/.moi/moinode-new-4"
//	err := vault.New(datadir, "vmvm1234", "dev-050122", "Test@123", "dev", moinode.MOI_FULL_NODE, 1)
//	if err != nil {
//		t.Fatalf("%v", err)
//	}
//
//	t.Log("Private Key for consensus: ", vault.consensusPriv.Bytes())
//	t.Log("Private Key for network: ", vault.networkPriv.Bytes())
//	t.Log("KramaID: ", vault.KramaID())
//}
//
//func TestBLSSignAndVerify(t *testing.T) {
//	vault := KramaVault{}
//	datadir := "/Users/sarvatechdeveloper1/.moi/moinode-new-4"
//	err := vault.New(datadir, "vmvm1234", "dev-050122", "Test@123", "dev", moinode.MOI_FULL_NODE, 1)
//	if err != nil {
//		t.Fatalf("%v", err)
//	}
//
//	msg := []byte("I'm getting signed")
//	sigBytes, err := vault.Sign(msg, common.BLS_BLST)
//	if err != nil {
//		t.Fatalf("%v", err)
//	}
//	t.Log("Signature: ", sigBytes)
//
//	pubKey := vault.consensusPriv.GetPublicKeyInBytes()
//	verificationSuccess, err := vault.Verify(msg, sigBytes, pubKey)
//	if err != nil {
//		t.Fatalf("%v", err)
//	}
//	t.Log("Verification Successful? : ", verificationSuccess)
//}
