package ecdsa

import (
	hexutil "encoding/hex"
	"fmt"
	"testing"

	"github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/stretchr/testify/assert"
)

var (
	samplePrivateKey = []byte{
		223, 251, 80, 42, 148, 23, 80, 3, 41, 56, 67, 29, 6, 222, 100,
		235, 198, 118, 247, 22, 36, 154, 171, 94, 29, 237, 207, 78, 250, 162, 67, 14,
	}
	sampleMessage = []byte("Hello MOI user, this is test string being signed")
)

func TestECDSASignWithSecp256k1(t *testing.T) {
	var s256 EcdsaSecp256k1Signature

	kid := kramaid.KramaID("bvby3pBVU5BEL2jBHJrH23GTb9qe8nL4XHqqKzZVbth7gBZ5c3." +
		"16Uiu2HAmGZr9gyQ7fDdmdBsRL29EjxR81Y74TEPbemBkyKuk2Ufj")

	err := s256.Sign(sampleMessage, samplePrivateKey, kid)
	if err != nil {
		t.Error("signing with secp256k1 key failed")
	}

	rawSig := common.MarshalSignature(common.Signature(s256))
	rawSigInHex := hexutil.EncodeToString(rawSig)
	assert.EqualValues(t,
		"0146304402207bb29ab5609dd826c114aed914f72b6f5d4860637671dafe7"+
			"47d519a190b7528022011a02462f8f66ff1645e7df7466e2323d50a224eaee35104c5082aa90c9a027e",
		rawSigInHex,
		fmt.Sprintf("> Expected 0146304402207bb29ab5609dd826c114aed914f72b"+
			"6f5d4860637671dafe747d519a190b7528022011a02462f8f66ff1645e7df7466e2323d50a224eaee35104c5082aa90c9a027e "+
			"for ECDSA Secp256k1 signature but got %v", rawSigInHex))
}
