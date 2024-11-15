package crypto

import (
	"encoding/hex"

	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	"github.com/sarvalabs/go-moi/crypto/signature/bls"
	"github.com/sarvalabs/go-moi/crypto/signature/ecdsa"
	"github.com/sarvalabs/go-moi/crypto/signature/schnorr"
)

const (
	DefaultMOIWalletPath = "m/44'/6174'/0'/0/1"
	DefaultMOIIDPath     = "m/44'/6174'/0'/0/0"
	GuardianMode         = 0
	UserMode             = 1
)

type KramaVault struct {
	consensusPriv PrivateKey      // Private Key used in consensus for signing etc
	networkPriv   PrivateKey      // Private key used in p2p communication
	kramaID       kramaid.KramaID // KramaID of the user or node
	Address       identifiers.Address
	mnemonic      poi.Mnemonic
}

type VaultConfig struct {
	DataDir                  string
	NodePassword             string
	SeedPhrase               string // Should be loaded from keystore
	Mode                     int8   // 0: Server, 1: Register/User mode
	NodeIndex                uint32 // Requires only in Register mode
	InMemory                 bool
	MnemonicKeystorePath     string // Absolute path to the mnemonic keystore
	MnemonicKeystorePassword string // Password to decrypt mnemonic keystore
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
		kramaIDVersion,
		networkKey,
		nodeIndex,
		participantID,
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

	if cfg.Mode == GuardianMode {
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
			if err := mnemonic.FromString(cfg.SeedPhrase); err != nil {
				return nil, err
			}
		}

		if cfg.MnemonicKeystorePath != "" {
			mnemonicKsBytes, err := poi.GetMnemonicKeystore(cfg.MnemonicKeystorePath)
			if err != nil {
				return nil, err
			}

			if err := mnemonic.FromKeystore(mnemonicKsBytes, cfg.MnemonicKeystorePassword); err != nil {
				return nil, err
			}
		}

		if mnemonic[0] == "" {
			return nil, cryptocommon.ErrMnemonicKeystorePasswordAndPathMandatory
		}

		bothSignAndCommPrivBytes, moiID, err := poi.GetPrivateKeysForSigningAndNetwork(mnemonic.String(), cfg.NodeIndex)
		if err != nil {
			return nil, err
		}

		currentKID, err := kramaid.NewKramaID(
			kramaIDVersion,
			bothSignAndCommPrivBytes[32:],
			cfg.NodeIndex,
			moiID,
			true,
		)
		if err != nil {
			return nil, err
		}

		if !cfg.InMemory {
			if err := poi.SetupKeystore(currentKID,
				bothSignAndCommPrivBytes, validatorType, cfg.DataDir, cfg.NodePassword,
			); err != nil {
				return nil, err
			}
		}

		signingAndNetworkKeys = bothSignAndCommPrivBytes
		moiIDAddress = moiID
		nodeIgcPath = cfg.NodeIndex
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

func (vault *KramaVault) SetNetworkPrivateKey(key PrivateKey) {
	vault.networkPriv = key
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

func (vault *KramaVault) MoiID() (string, error) {
	return vault.kramaID.MoiID()
}

func (vault *KramaVault) MoiIDPublicKey() ([]byte, error) {
	moiIDInString, err := vault.kramaID.MoiID()
	if err != nil {
		return nil, err
	}

	moiIDInBytes, err := hex.DecodeString(moiIDInString)
	if err != nil {
		return nil, err
	}

	return moiIDInBytes[1:], nil
}

func (vault *KramaVault) GetPublicKeyAt(path string) ([]byte, error) {
	_, publicKey, err := poi.GetPrivateKeyAtPath(vault.mnemonic.String(), path)
	if err != nil {
		return nil, err
	}

	return publicKey, nil
}

func (vault *KramaVault) Sign(data []byte, sigType cryptocommon.SigType, signOptions ...SignOption) ([]byte, error) {
	var (
		signingKey     []byte
		err            error
		signingKeyType KeyType
	)

	signingKeyType = BLS
	signingKey = vault.consensusPriv.Bytes()

	if len(signOptions) != 0 {
		signingKeyType = SECP256K1

		opts := &SignOptions{}
		for _, opt := range signOptions {
			opt(opts)
		}

		switch {
		case opts.IgcPath != "":
			signingKey, _, err = poi.GetPrivateKeyAtPath(vault.mnemonic.String(), opts.IgcPath)
			if err != nil {
				return nil, err
			}
		case opts.ShouldSignWithNetworkKey:
			signingKey = vault.networkPriv.Bytes()
		}
	}

	switch sigType {
	case cryptocommon.BlsBLST:
		{
			if signingKeyType == SECP256K1 {
				blsPrivKey := new(BLSPrivKey)
				blsPrivKey.UnMarshal(signingKey)
				signingKey = blsPrivKey.Bytes()
			}

			blsSigWithBlst := bls.BlsWithBlstSignature{}
			if err := blsSigWithBlst.Sign(data, signingKey, vault.kramaID); err != nil {
				return nil, errors.Wrap(cryptocommon.ErrSigningFailed, err.Error())
			}

			return cryptocommon.MarshalSignature(cryptocommon.Signature(blsSigWithBlst)), nil
		}
	case cryptocommon.SchnorrSecp256k1:
		{
			schnorrSig := schnorr.SchnorrSignature{}
			if err := schnorrSig.Sign(data, signingKey, vault.kramaID); err != nil {
				return nil, errors.Wrap(cryptocommon.ErrSigningFailed, err.Error())
			}

			return cryptocommon.MarshalSignature(cryptocommon.Signature(schnorrSig)), nil
		}
	case cryptocommon.EcdsaSecp256k1:
		{
			if signingKeyType == SECP256K1 {
				ecdsaSig := ecdsa.EcdsaSecp256k1Signature{}
				if err := ecdsaSig.Sign(data, signingKey, vault.kramaID); err != nil {
					return nil, errors.Wrap(cryptocommon.ErrSigningFailed, err.Error())
				}

				return cryptocommon.MarshalSignature(cryptocommon.Signature(ecdsaSig)), nil
			} else {
				return nil, cryptocommon.ErrSignOptionsNotPassed
			}
		}
	default:
		{
			return nil, cryptocommon.ErrUnsupportedSigType
		}
	}
}

func Verify(data, signature, pubBytes []byte) (bool, error) {
	sig, err := cryptocommon.UnmarshalSignature(signature)
	if err != nil {
		return false, err
	}

	switch sig.SigPrefix[0] {
	case cryptocommon.BlsBLST.Byte():
		{
			blsSigWithBlst := bls.BlsWithBlstSignature(sig)

			return blsSigWithBlst.Verify(data, pubBytes)
		}
	case cryptocommon.EcdsaSecp256k1.Byte():
		{
			s256Sig := ecdsa.EcdsaSecp256k1Signature(sig)

			return s256Sig.Verify(data, pubBytes)
		}
	case cryptocommon.SchnorrSecp256k1.Byte():
		{
			schSig := schnorr.SchnorrSignature(sig)

			return schSig.Verify(data, pubBytes)
		}
	default:
		return false, cryptocommon.ErrUnsupportedSigType
	}
}

func AggregateSignatures(multipleSignatures [][]byte) ([]byte, error) {
	if len(multipleSignatures) == 0 {
		return nil, cryptocommon.ErrEmpty
	}

	blsBlstSigs := make([]bls.BlsWithBlstSignature, len(multipleSignatures))

	for i := 0; i < len(multipleSignatures); i++ {
		tempSigInBls, err := cryptocommon.UnmarshalSignature(multipleSignatures[i])
		if err != nil {
			return nil, err
		}

		if tempSigInBls.SigPrefix[0] != 0x04 {
			return nil, cryptocommon.ErrUnsupportedAggSignature
		}

		blsBlstSigs[i] = bls.BlsWithBlstSignature(tempSigInBls)
	}

	return bls.AggregateSignatures(blsBlstSigs)
}

func VerifyAggregateSignature(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if len(multiplePubKeys) == 0 {
		return false, cryptocommon.ErrEmpty
	}

	for i := 0; i < len(multiplePubKeys); i++ {
		if len(multiplePubKeys[i]) != 48 {
			return false, cryptocommon.ErrInvalidBLSPublicKeyLength
		}
	}

	return bls.VerifyAggregateSignature(data, aggSignature, multiplePubKeys)
}

func VerifyMultiSig(aggSignature []byte, allMsgs [][]byte, allPubKeys [][]byte) (bool, error) {
	for i := 0; i < len(allPubKeys); i++ {
		if len(allPubKeys[i]) != 48 {
			return false, cryptocommon.ErrInvalidBLSPublicKeyLength
		}
	}

	return bls.VerifyMultiSig(aggSignature, allMsgs, allPubKeys)
}

// GetSignature generates EcdsaSecp256k1 signature using DefaultMOIWallet IGCPath
func GetSignature(bz []byte, mnemonic string) (string, error) {
	cfg := &VaultConfig{
		SeedPhrase: mnemonic,
		Mode:       1,
		InMemory:   true,
	}

	vault, err := NewVault(cfg, moinode.MoiFullNode, UserMode)
	if err != nil {
		return "", err
	}

	sign, err := vault.Sign(bz, cryptocommon.EcdsaSecp256k1, UsingIgcPath(DefaultMOIWalletPath))
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(sign), nil
}

func VerifySignatureUsingKramaID(id kramaid.KramaID, rawData []byte, signature []byte) error {
	peerID, err := id.DecodedPeerID()
	if err != nil {
		return errors.Wrapf(err, "failed to get peer id from krama id")
	}

	pk, err := peerID.ExtractPublicKey()
	if err != nil {
		return errors.Wrapf(err, "failed to get public key from peer id")
	}

	rawPK, err := pk.Raw()
	if err != nil {
		return errors.Wrapf(err, "failed to get raw public key from public key")
	}

	verified, err := Verify(rawData, signature, rawPK)
	if !verified || err != nil {
		return errors.New("Signature verification failed")
	}

	return nil
}
