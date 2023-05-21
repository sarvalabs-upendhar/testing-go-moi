package mudra

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	"github.com/sarvalabs/moichain/mudra/signature/bls"
	"github.com/sarvalabs/moichain/mudra/signature/ecdsa"
	"github.com/sarvalabs/moichain/mudra/signature/schnorr"
	"github.com/sarvalabs/moichain/types"
)

type KramaVault struct {
	consensusPriv PrivateKey      // Private Key used in consensus for signing etc
	networkPriv   PrivateKey      // Private key used in p2p communication
	kramaID       kramaid.KramaID // KramaID of the user or node
	Address       types.Address
	mnemonic      poi.Mnemonic
}
type VaultConfig struct {
	DataDir      string
	NodePassword string
	SeedPhrase   string
	Mode         int8   // 0: Server, 1: Register/User mode
	NodeIndex    uint32 // Requires only in Register mode
}

func loadVault(signingAndNetworkKeys []byte,
	participantID string,
	nodeIndex uint32,
	kramaIDVersion int,
	seed poi.Mnemonic,
) (*KramaVault, error) {
	vault := new(KramaVault)
	signingKey := signingAndNetworkKeys[:32]
	networkKey := signingAndNetworkKeys[32:]

	currentKID, err := kramaid.NewKramaID(
		networkKey,
		nodeIndex,
		participantID,
		kramaIDVersion,
		true,
	)
	if err != nil {
		return nil, err
	}

	cPriv := new(BLSPrivKey)
	cPriv.UnMarshal(signingKey)

	nPriv := new(SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)

	vault.consensusPriv = cPriv
	vault.networkPriv = nPriv
	vault.kramaID = currentKID
	vault.mnemonic = seed

	return vault, nil
}

func NewVault(cfg *VaultConfig, validatorType moinode.MoiNodeType, kramaIDVersion int) (*KramaVault, error) {
	var (
		signingAndNetworkKeys []byte
		nodeIgcPath           uint32
		moiIDAddress          string
	)

	mnemonic := poi.Mnemonic{}

	if cfg.Mode == 0 {
		nodeKeystore, err := poi.GetKeystore(cfg.DataDir)
		if err != nil {
			return nil, err
		}

		signingAndNetworkKeys, moiIDAddress, nodeIgcPath, err = poi.DecryptKeystore(nodeKeystore, cfg.NodePassword)

		if err != nil {
			return nil, err
		}
	} else {
		if cfg.SeedPhrase != "" {
			var err error
			if err = mnemonic.FromString(cfg.SeedPhrase); err != nil {
				return nil, err
			}

			bothSignAndCommPrivBytes, moiID, err := poi.GetPrivateKeysForSigningAndNetwork(mnemonic.String(), cfg.NodeIndex)
			if err != nil {
				return nil, err
			}

			currentKID, err := kramaid.NewKramaID(
				bothSignAndCommPrivBytes[32:],
				cfg.NodeIndex,
				moiID,
				kramaIDVersion,
				true,
			)
			if err != nil {
				return nil, err
			}

			if err := poi.SetupKeystore(currentKID,
				bothSignAndCommPrivBytes, validatorType, cfg.DataDir, cfg.NodePassword,
			); err != nil {
				return nil, err
			}

			signingAndNetworkKeys = bothSignAndCommPrivBytes
			moiIDAddress = moiID
			nodeIgcPath = cfg.NodeIndex
		} else {
			return nil, common.ErrMnemonicMandatory
		}
	}

	return loadVault(signingAndNetworkKeys, moiIDAddress, nodeIgcPath, kramaIDVersion, mnemonic)
}

func (vault *KramaVault) GetConsensusPrivateKey() PrivateKey {
	return vault.consensusPriv
}

func (vault *KramaVault) SetConsensusPrivateKey(key PrivateKey) {
	vault.consensusPriv = key
}

func (vault *KramaVault) GetNetworkPrivateKey() PrivateKey {
	return vault.networkPriv
}

func (vault *KramaVault) KramaID() kramaid.KramaID {
	return vault.kramaID
}

func (vault *KramaVault) GetMnemonic() poi.Mnemonic {
	return vault.mnemonic
}

func (vault *KramaVault) SetKramaID(id kramaid.KramaID) {
	vault.kramaID = id
}

func (vault *KramaVault) MOiID() (string, error) {
	return vault.kramaID.MoiID()
}

func (vault *KramaVault) Sign(data []byte, sigType common.SigType, signOptions ...SignOption) ([]byte, error) {
	var (
		signingKey     []byte
		err            error
		signingKeyType KeyType
	)

	if len(signOptions) != 0 {
		signingKeyType = SECP256K1

		opts := &SignOptions{}
		for _, opt := range signOptions {
			opt(opts)
		}

		signingKey, _, err = poi.GetPrivateKeyAtPath(vault.mnemonic.String(), opts.IgcPath)
		if err != nil {
			return nil, err
		}
	} else {
		signingKeyType = BLS
		signingKey = vault.consensusPriv.Bytes()
	}

	switch sigType {
	case common.BlsBLST:
		{
			if signingKeyType == SECP256K1 {
				blsPrivKey := new(BLSPrivKey)
				blsPrivKey.UnMarshal(signingKey)
				signingKey = blsPrivKey.Bytes()
			}

			blsSigWithBlst := bls.BlsWithBlstSignature{}
			if err := blsSigWithBlst.Sign(data, signingKey, vault.kramaID); err != nil {
				return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
			}

			return common.MarshalSignature(common.Signature(blsSigWithBlst)), nil
		}
	case common.SchnorrSecp256k1:
		{
			schnorrSig := schnorr.SchnorrSignature{}
			if err := schnorrSig.Sign(data, signingKey, vault.kramaID); err != nil {
				return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
			}

			return common.MarshalSignature(common.Signature(schnorrSig)), nil
		}
	case common.EcdsaSecp256k1:
		{
			if signingKeyType == SECP256K1 {
				ecdsaSig := ecdsa.EcdsaSecp256k1Signature{}
				if err := ecdsaSig.Sign(data, signingKey, vault.kramaID); err != nil {
					return nil, errors.Wrap(common.ErrSigningFailed, err.Error())
				}

				return common.MarshalSignature(common.Signature(ecdsaSig)), nil
			} else {
				return nil, common.ErrSignOptionsNotPassed
			}
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
