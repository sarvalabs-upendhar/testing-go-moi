package mudra

import (
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/mudra/common"
	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/mudra/poi"
	"gitlab.com/sarvalabs/moichain/mudra/poi/moinode"
	"gitlab.com/sarvalabs/moichain/mudra/signature/bls"
	"gitlab.com/sarvalabs/moichain/mudra/signature/ecdsa"
	"gitlab.com/sarvalabs/moichain/mudra/signature/schnorr"
	"gitlab.com/sarvalabs/moichain/types"
)

type KramaVault struct {
	consensusPriv PrivateKey      // Private Key used in consensus for signing etc
	networkPriv   PrivateKey      // Private key used in p2p communication
	kramaID       kramaid.KramaID // KramaID of the user
	Address       types.Address
}
type VaultConfig struct {
	DataDir       string
	MoiIDUsername string
	MoiIDPassword string
	MoiIDURL      string
	NodePassword  string
}

func NewVault(cfg *VaultConfig, validatorType moinode.MoiNodeType, kramaIDVersion int) (*KramaVault, error) {
	var (
		signingAndNetworkKeys []byte
		nodeIgcPath           uint32
		moiIDAddress          string
	)

	vault := new(KramaVault)

	isNewNode := false // to check if we need to register the node

	nodeKeystore, err := poi.GetKeystore(cfg.DataDir)
	if err != nil {
		if errors.Is(err, common.ErrNoKeystore) {
			signingAndNetworkKeys,
				moiIDAddress,
				nodeIgcPath, err = poi.GenerateKeysForVault(cfg.MoiIDUsername, cfg.MoiIDPassword, cfg.MoiIDURL)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}

		isNewNode = true
	} else {
		signingAndNetworkKeys, moiIDAddress, nodeIgcPath, err = poi.DecryptKeystore(nodeKeystore, cfg.NodePassword)
		if err != nil {
			return nil, err
		}
	}

	signingKey := signingAndNetworkKeys[:32]
	networkKey := signingAndNetworkKeys[32:]

	currentKID, err := kramaid.NewKramaID(
		networkKey,
		nodeIgcPath,
		moiIDAddress,
		kramaIDVersion,
		true,
	)
	if err != nil {
		return nil, err
	}

	if isNewNode {
		err := poi.RegisterNode(signingAndNetworkKeys,
			moiIDAddress,
			cfg.DataDir,
			cfg.NodePassword,
			currentKID,
			validatorType,
			cfg.MoiIDURL)
		if err != nil {
			return nil, err
		}
	}

	cPriv := new(BLSPrivKey)
	cPriv.UnMarshal(signingKey)

	nPriv := new(SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)

	vault.consensusPriv = cPriv
	vault.networkPriv = nPriv
	vault.kramaID = currentKID

	return vault, nil
}

func (vault *KramaVault) GetConsensusPrivateKey() PrivateKey {
	return vault.consensusPriv
}

func (vault *KramaVault) GetNetworkPrivateKey() PrivateKey {
	return vault.networkPriv
}

func (vault *KramaVault) KramaID() kramaid.KramaID {
	return vault.kramaID
}

func (vault *KramaVault) MOiID() (string, error) {
	return vault.kramaID.MoiID()
}

func (vault *KramaVault) Sign(data []byte, sigType common.SigType) ([]byte, error) {
	switch sigType {
	case common.BlsBLST:
		{
			pkType := vault.consensusPriv.KeyType()
			if pkType != BLS {
				return nil, common.ErrUnsupportedSigTypeForPrivateKey
			}

			blsSigWithBlst := bls.BlsWithBlstSignature{}
			if err := blsSigWithBlst.Sign(data, vault.consensusPriv.Bytes(), vault.kramaID); err != nil {
				return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
			}

			return common.MarshalSignature(common.Signature(blsSigWithBlst)), nil
		}
	case common.SchnorrSecp256k1:
		{
			pkType := vault.consensusPriv.KeyType()
			if pkType != SECP256K1 {
				return nil, common.ErrUnsupportedSigTypeForPrivateKey
			}

			schnorrSig := schnorr.SchnorrSignature{}
			if err := schnorrSig.Sign(data, vault.consensusPriv.Bytes(), vault.kramaID); err != nil {
				return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
			}

			return common.MarshalSignature(common.Signature(schnorrSig)), nil
		}
	case common.EcdsaSecp256k1:
		{
			pkType := vault.consensusPriv.KeyType()
			if pkType != SECP256K1 {
				return nil, common.ErrUnsupportedSigTypeForPrivateKey
			}

			ecdsaSig := ecdsa.EcdsaSecp256k1Signature{}
			if err := ecdsaSig.Sign(data, vault.consensusPriv.Bytes(), vault.kramaID); err != nil {
				return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
			}

			return common.MarshalSignature(common.Signature(ecdsaSig)), nil
		}
	default:
		{
			return nil, common.ErrUnsupportedSigType
		}
	}
}

func Verify(data, signature, pubBytes []byte) (bool, error) {
	sig, err := common.UnmarshalSignature(signature)
	if err != nil {
		return false, err
	}

	switch sig.SigPrefix[0] {
	case 0x04:
		{
			blsSigWithBlst := bls.BlsWithBlstSignature(sig)

			return blsSigWithBlst.Verify(data, pubBytes)
		}
	case 0x01:
		{
			s256Sig := ecdsa.EcdsaSecp256k1Signature(sig)

			return s256Sig.Verify(data, pubBytes)
		}
	case 0x02:
		{
			schSig := schnorr.SchnorrSignature(sig)

			return schSig.Verify(data, pubBytes)
		}
	default:
		return false, common.ErrUnsupportedSigType
	}
}

func AggregateSignatures(multipleSignatures [][]byte) ([]byte, error) {
	if len(multipleSignatures) == 0 {
		return nil, common.ErrEmpty
	}

	blsBlstSigs := make([]bls.BlsWithBlstSignature, len(multipleSignatures))

	for i := 0; i < len(multipleSignatures); i++ {
		tempSigInBls, err := common.UnmarshalSignature(multipleSignatures[i])
		if err != nil {
			return nil, err
		}

		if tempSigInBls.SigPrefix[0] != 0x04 {
			return nil, common.ErrUnsupportedAggSignature
		}

		blsBlstSigs[i] = bls.BlsWithBlstSignature(tempSigInBls)
	}

	return bls.AggregateSignatures(blsBlstSigs)
}

func VerifyAggregateSignature(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if len(multiplePubKeys) == 0 {
		return false, common.ErrEmpty
	}

	for i := 0; i < len(multiplePubKeys); i++ {
		if len(multiplePubKeys[i]) != 48 {
			return false, common.ErrInvalidBLSPublicKeyLength
		}
	}

	return bls.VerifyAggregateSignature(data, aggSignature, multiplePubKeys)
}
