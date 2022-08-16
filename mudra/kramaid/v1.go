package kramaid

import (
	hexutil "encoding/hex"
	"errors"
	"strings"

	"github.com/mr-tron/base58"
)

// generateKramaIDV1 used to generate KramaID of version 1, takes MOI id extended Private Key(m/44'/6174')
func generateKramaIDV1(nthValidator uint32,
	moiIDAddress string,
	privKeyBytesOfValidator []byte,
	isNode bool) (KramaID, error) {
	if nthValidator >= HardenedStartIndex {
		return "", errors.New("invalid node index, max value: 2147483648")
	}

	var (
		metaInV1         MetaInfoV1
		finalMOIidString string
	)

	if moiIDAddress[0:1] == "0x" {
		finalMOIidString = moiIDAddress[2:]
	} else {
		finalMOIidString = moiIDAddress
	}

	metaInV1.moiID = finalMOIidString
	metaInV1.nodeIndex = nthValidator

	var err error
	p2pID, err := GeneratePeerID(privKeyBytesOfValidator)

	if err != nil {
		return "", err
	}

	kidString, err := stringV1(metaInV1, p2pID.String(), isNode)

	if err != nil {
		return "", err
	}

	k := KramaID(kidString)

	return k, nil
}

// stringV1 stringifies all parameters of KramaID in version 1 and returns base58 encoded multi-hash
func stringV1(metaInV1 MetaInfoV1, p2pID string, isNode bool) (string, error) {
	moiIDIn32Bytes := make([]byte, 32)
	moiIDInBytes, err := hexutil.DecodeString(metaInV1.moiID)

	if err != nil {
		return "", err
	}

	copy(moiIDIn32Bytes, moiIDInBytes)

	var targetedTypeVersionPrefix byte
	if isNode {
		targetedTypeVersionPrefix = NodeTypeVersion1
	} else {
		targetedTypeVersionPrefix = UserTypeVersion1
	}

	var kramaIDInBytes []byte                                                  // of length 37
	kramaIDInBytes = append(kramaIDInBytes[:], targetedTypeVersionPrefix)      // [0]
	kramaIDInBytes = append(kramaIDInBytes[:], moiIDIn32Bytes[:]...)           // [1:33]
	kramaIDInBytes = append(kramaIDInBytes[:], itob(metaInV1.nodeIndex)[:]...) // [33:37] 4 bytes for uint32

	kramaIDString := base58.Encode(kramaIDInBytes)

	return strings.Join([]string{kramaIDString, p2pID}, "."), nil
}

// unMarshalV1Meta returns MetaInfoV1 in version 1 from the string
func unMarshalV1Meta(kramaIDMetaDecoded []byte) (MetaInfoV1, error) {
	metaInV1 := new(MetaInfoV1)
	if len(kramaIDMetaDecoded) != 36 {
		return *metaInV1, errors.New("invalid KramaID length. KramaID in version 1 takes 37 bytes")
	}

	metaInV1.moiID = hexutil.EncodeToString(kramaIDMetaDecoded[0:32])
	metaInV1.nodeIndex = btoi(kramaIDMetaDecoded[32:36])

	return *metaInV1, nil
}
