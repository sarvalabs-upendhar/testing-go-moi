package poi

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/sarvalabs/go-moi/common/identifiers"

	blst "github.com/supranational/blst/bindings/go"
	"github.com/tyler-smith/go-bip39"

	"github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
)

// GetKeystore returns keystore of node that persists private key required for consensus and p2p network communication
func GetKeystore(dataDir string) ([]byte, error) {
	ksFilePath := strings.Join([]string{dataDir, "keystore.json"}, "/")

	ksContent, err := os.ReadFile(ksFilePath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil, common.ErrNoKeystore
		}

		return nil, err
	}

	return ksContent, nil
}

func SetupKeystore(
	currentKramaID identifiers.KramaID,
	bothSignAndCommPrivBytes []byte,
	validatorType moinode.MoiNodeType,
	dataDir string,
	passPhrase string,
) error {
	nodeSpecificPublicBytes := identifiers.GetPublicKeyFromPrivateBytes(bothSignAndCommPrivBytes[0:32], true)
	nodeSpecificPublicAddr := identifiers.GetAddressFromPublicBytes(nodeSpecificPublicBytes)

	// TODO: FIX THIS
	// moiID, err := currentKramaID.MoiID()
	// if err != nil {
	//	 return err
	// }

	// nodeIndex, err := currentKramaID.NodeIndex()
	// if err != nil {
	//	 return err
	// }

	var nodeKS nodeKeystore
	// nodeKS.MoiID = moiID
	nodeKS.IGCPath = [4]uint32{NodeIGCPath[0], NodeIGCPath[1], NodeIGCPath[2], 0}
	nodeKS.KramaID = string(currentKramaID)
	nodeKS.NodeType = validatorType.ByteString()
	nodeKS.NodeAddress = nodeSpecificPublicAddr

	if err := StoreKeystore(bothSignAndCommPrivBytes, passPhrase, dataDir, nodeKS); err != nil {
		return err
	}

	return nil
}

func DecryptKeystore(ksInBytes []byte, nodePassPhrase string) ([]byte, uint32, error) {
	var nKs nodeKeystore

	err := json.Unmarshal(ksInBytes, &nKs)
	if err != nil {
		return nil, 0, err
	}

	prvKeys, err := decryptKeystore(nKs.Crypto, nodePassPhrase)
	if err != nil {
		return nil, 0, err
	}

	return prvKeys, nKs.IGCPath[3], nil
}

func RandGenKeystore(dataDir, localNodePass string) ([]byte, identifiers.KramaID, error) {
	mnemonic := GenerateRandMnemonic()

	seed, err := bip39.NewSeedWithErrorChecking(mnemonic.String(), "")
	if err != nil {
		return nil, "", err
	}

	bothSignAndCommPrivBytes, err := identifiers.GetRandomPrivateKeys([32]byte(seed))
	if err != nil {
		return nil, "", err
	}

	pairingFriendlyPrivKey := blst.KeyGen(bothSignAndCommPrivBytes[0:32])
	blsPkCasted := *pairingFriendlyPrivKey
	pub := new(blst.P1Affine).From(&blsPkCasted)
	pubBytes := pub.Compress()

	// moiIDAddressInString := hex.EncodeToString(moiPubBytes)

	/* Creating KramaID */
	currentKID, err := identifiers.GenerateKramaIDv0(
		identifiers.NetworkZone0,
		bothSignAndCommPrivBytes[32:],
	)
	if err != nil {
		return nil, "", err
	}

	var nodeKS nodeKeystore
	nodeKS.IGCPath = [4]uint32{6174, 5020, 0, 0}
	nodeKS.KramaID = string(currentKID)
	nodeKS.NodeType = "0x07"

	if err = StoreKeystore(bothSignAndCommPrivBytes, localNodePass, dataDir, nodeKS); err != nil {
		return nil, "", err
	}

	return pubBytes, currentKID, nil
}

func GenerateRandMnemonic() Mnemonic {
	seedPhrase := Mnemonic{}

	// not checking for error, since below function will not return error
	// as long as we pass bit-size = 128
	randEntropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(randEntropy)
	seedPhrase.FromString(mnemonic)

	return seedPhrase
}
