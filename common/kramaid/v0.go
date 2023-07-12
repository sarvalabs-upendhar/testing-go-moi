package kramaid

import (
	"encoding/json"
	"strings"

	"github.com/mr-tron/base58"
)

// GenerateKramaIDV0 is used to generate KramaID in version 0
//
// takes privateKey in bytes to generate peer id and other information about node, and it's owner
func generateKramaIDV0(mID string, nodeIndex int, privKeyBytesOfValidator []byte) (KramaID, error) {
	nodePubBytes := GetPublicKeyFromPrivateBytes(privKeyBytesOfValidator, false)

	m := MetaInfoV0{
		MoiID:      mID,
		NodeIndex:  nodeIndex,
		ProtocolID: 1,
		NodeID:     GetAddressFromPublicBytes(nodePubBytes),
	}

	p2pID, err := GeneratePeerID(privKeyBytesOfValidator)
	if err != nil {
		return "", err
	}

	kidString, err := stringInV0(m, p2pID.String())
	if err != nil {
		return "", err
	}

	k := KramaID(kidString)

	return k, nil
}

// stringV0 used to return the b58 encoded KramaID string consists of metaInfo and P2P id
func stringInV0(metaInfoInV0 MetaInfoV0, p2pID string) (string, error) {
	metabytes, err := json.Marshal(MetaInfoV0{
		MoiID:      metaInfoInV0.MoiID,
		NodeIndex:  metaInfoInV0.NodeIndex,
		ProtocolID: 1,
		NodeID:     metaInfoInV0.NodeID,
	})
	if err != nil {
		return "", err
	}

	currentMetainfo := base58.Encode(metabytes)

	return strings.Join([]string{currentMetainfo, p2pID}, "."), nil
}

// unMarshalV0Meta returns KramaID in version 0 from the string
func unMarshalV0Meta(kramaIDMetaBytes []byte) (MetaInfoV0, error) {
	metaInfoVar := new(MetaInfoV0)

	err := json.Unmarshal(kramaIDMetaBytes, &metaInfoVar)
	if err != nil {
		return *metaInfoVar, err
	}

	return *metaInfoVar, nil
}
