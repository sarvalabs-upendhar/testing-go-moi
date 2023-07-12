package bls

import (
	blst "github.com/supranational/blst/bindings/go"

	"github.com/sarvalabs/moichain/common/kramaid"
	"github.com/sarvalabs/moichain/crypto/common"
)

const BLSPublicKeyLength = 48

var dstMinSig = []byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_")

type BlsWithBlstSignature common.Signature

func (blsBlst *BlsWithBlstSignature) Type() common.SigType {
	return common.BlsBLST
}

func (blsBlst *BlsWithBlstSignature) Sign(data, signingKey []byte, kid kramaid.KramaID) error {
	// casting into BLST Secret key
	if len(signingKey) != 32 {
		return common.ErrInvalidPrivKeyLength
	}

	pairingFriendlyPrivKey := new(blst.SecretKey).Deserialize(signingKey)

	if pairingFriendlyPrivKey == nil {
		return common.ErrNotPairingFriendlyKey
	}

	if common.IsZeroBytes(pairingFriendlyPrivKey.Serialize()) {
		return common.ErrZeroKey
	}

	tempPointInGroup2ForSignature := new(blst.P2Affine)
	sig := tempPointInGroup2ForSignature.Sign(pairingFriendlyPrivKey, data, dstMinSig)
	sigBytes := sig.Compress()

	var sigPrefix [2]byte
	sigPrefix[0] = common.BlsBLST.Byte()
	sigPrefix[1] = byte(len(sigBytes))
	blsBlst.SigPrefix = sigPrefix
	blsBlst.Digest = sigBytes

	ver, err := kid.Version()
	if err != nil {
		return err
	}

	if ver == 1 {
		blsBlst.Extra = nil
	} else {
		// To derive BLS public key from BLS private key
		pubKey := new(blst.P1Affine).From(pairingFriendlyPrivKey)
		blsBlst.Extra = pubKey.Compress()
	}

	return nil
}

func (blsBlst *BlsWithBlstSignature) Verify(message []byte, nodeBLSPublicKey []byte) (bool, error) {
	if len(nodeBLSPublicKey) != BLSPublicKeyLength {
		return false, common.ErrInvalidBLSPublicKeyLength
	}

	verificationBool := new(blst.P2Affine).VerifyCompressed(blsBlst.Digest, true,
		nodeBLSPublicKey, true,
		message, dstMinSig)

	return verificationBool, nil
}

func AggregateSignatures(multipleSignatures []BlsWithBlstSignature) ([]byte, error) {
	if len(multipleSignatures) == 0 {
		return nil, common.ErrEmpty
	}

	rawBLSSigs := make([]*blst.P2Affine, len(multipleSignatures))
	for i := 0; i < len(multipleSignatures); i++ {
		rawBLSSigs[i] = new(blst.P2Affine).Uncompress(multipleSignatures[i].Digest)
	}

	signature := new(blst.P2Aggregate)
	signature.Aggregate(rawBLSSigs, false)

	affinedSignature := signature.ToAffine()

	return affinedSignature.Compress(), nil
}

func VerifyAggregateSignature(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if len(multiplePubKeys) == 0 {
		return false, common.ErrEmpty
	}

	rawBLSPubKeys := make([]*blst.P1Affine, len(multiplePubKeys))

	for i := 0; i < len(multiplePubKeys); i++ {
		if len(multiplePubKeys[i]) != 48 {
			return false, common.ErrInvalidBLSPublicKeyLength
		}

		rawBLSPubKeys[i] = new(blst.P1Affine).Uncompress(multiplePubKeys[i])
	}

	aggBLSSig := new(blst.P2Affine).Uncompress(aggSignature)

	return aggBLSSig.FastAggregateVerify(true, rawBLSPubKeys, data, dstMinSig), nil
}
