package kramaid

import (
	hexutil "encoding/hex"
	"errors"
	"strings"

	"github.com/mr-tron/base58"
)

// generateKramaIDV1 used to generate KramaID of version 1, takes MOI id extended Private Key(m/44'/6174')
func generateKramaIDV1(
	nthValidator uint32,
	moiIDString string,
	privKeyBytesOfValidator []byte,
	isNode bool,
) (KramaID, error) {
	if nthValidator >= HardenedStartIndex {
		return "", errors.New("invalid node index, max value: 2147483648")
	}

	var (
		metaInV1         MetaInfoV1
		finalMOIidString string
	)

	if moiIDString[0:1] == "0x" {
		finalMOIidString = moiIDString[2:]
	} else {
		finalMOIidString = moiIDString
	}

	metaInV1.moiID = finalMOIidString
	metaInV1.nodeIndex = nthValidator

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
	moiIDInBytes, err := hexutil.DecodeString(metaInV1.moiID)
	if err != nil {
		return "", err
	}

	var targetedTypeVersionPrefix byte
	if isNode {
		targetedTypeVersionPrefix = NodeTypeVersion1
	} else {
		targetedTypeVersionPrefix = UserTypeVersion1
	}

	var kramaIDInBytes []byte                                            // of length 38
	kramaIDInBytes = append(kramaIDInBytes, targetedTypeVersionPrefix)   // [0]
	kramaIDInBytes = append(kramaIDInBytes, moiIDInBytes...)             // [1:34]
	kramaIDInBytes = append(kramaIDInBytes, itob(metaInV1.nodeIndex)...) // [34:38] 4 bytes for uint32

	kramaIDString := base58.Encode(kramaIDInBytes)

	return strings.Join([]string{kramaIDString, p2pID}, "."), nil
}

// unMarshalV1Meta returns MetaInfoV1 in version 1 from the string
func unMarshalV1Meta(kramaIDMetaDecoded []byte) (MetaInfoV1, error) {
	metaInV1 := new(MetaInfoV1)
	if len(kramaIDMetaDecoded) != 38 {
		return *metaInV1, errors.New("invalid KramaID length. Meta info of KramaID  in version 1 takes 38 bytes")
	}

	metaInV1.moiID = hexutil.EncodeToString(kramaIDMetaDecoded[1:34])
	metaInV1.nodeIndex = btoi(kramaIDMetaDecoded[34:38])

	return *metaInV1, nil
}
