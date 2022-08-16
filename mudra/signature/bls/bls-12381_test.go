package bls

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/sarvalabs/moichain/mudra/common"
	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

const (
	expectedSig1 = "0460a78a813eefc41edd71e794b2576e40dfa05c1cf24f4dde70ebc07493ee1984ae227ff4348" +
		"f94b033dff5144b0756207d109a27972892d384f56a16a4c20f322234813c35cf3e22a48c6e4f6edb85ae1" +
		"ee5c1db697c81bf109e6ecdddf52e14fd"
)

var (
	samplePrivateKey = []byte{223, 251, 80, 42, 148, 23, 80, 3, 41, 56, 67, 29, 6, 222, 100, 235,
		198, 118, 247, 22, 36, 154, 171, 94, 29, 237, 207, 78, 250, 162, 67, 14}
	sampleMessage = []byte("Hello MOI user, this is test string being signed")
	blsPublicKey1 = []byte{135, 190, 156, 133, 44, 143, 38, 33, 213, 138, 22, 115, 131, 203, 65,
		129, 205, 190, 223, 234, 85, 40, 136, 133, 77, 193, 165, 213, 155, 170, 237, 41, 169, 255, 56, 50,
		118, 178, 5, 118, 176, 243, 174, 159, 95, 151, 60, 19}
)

func TestBLSActual(t *testing.T) {
	var bsig BlsWithBlstSignature

	kid := kramaid.KramaID("bvby3pBVU5BEL2jBHJrH23GTb9qe8nL4XHqqKzZVbth7gBZ5c3.16Uiu2HAmGZr9gyQ7fD" +
		"dmdBsRL29EjxR81Y74TEPbemBkyKuk2Ufj")

	err := bsig.Sign(sampleMessage, samplePrivateKey, kid)
	if err != nil {
		t.Fatalf("%v", err)
	}

	sigInHex := hex.EncodeToString(common.MarshalSignature(common.Signature(bsig)))
	assert.EqualValues(t,
		expectedSig1,
		sigInHex,
		fmt.Sprintf("> Expected %vfor BLS Signature but got %v", expectedSig1, sigInHex))
}

func TestBLSVerify(t *testing.T) {
	sigInHexBytes, err := hex.DecodeString(expectedSig1)
	if err != nil {
		t.Fatalf("%v", err)
	}

	bsigGeneral, err := common.UnmarshalSignature(sigInHexBytes)
	if err != nil {
		t.Fatalf("%v", err)
	}

	bsig := BlsWithBlstSignature(bsigGeneral)

	boolean, err := bsig.Verify(sampleMessage, blsPublicKey1)
	if err != nil {
		t.Fatalf("%v", err)
	}

	t.Log(boolean)
}
