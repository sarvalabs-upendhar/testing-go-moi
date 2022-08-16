package kramaid

import (
	"bytes"
	"math/big"
	"strings"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/mudra/common"
)

const (
	HardenedStartIndex = 2147483648 // 2^31
	// number of bits in a big.Word
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	// number of bytes in a big.Word
	wordBytes = wordBits / 8
)

const (
	UserTypeVersion0 = 0x00
	NodeTypeVersion0 = 0x10
	UserTypeVersion1 = 0x01
	NodeTypeVersion1 = 0x11
)

// MetaInfoV1 in Version 1
type MetaInfoV1 struct {
	moiID     string // MOI id is a unique id used to identity the node owner m/44'/6174'/0'/0/0
	nodeIndex uint32 // nodeIndex is the integer used to identity the nth node of the node owner(m/44'/6174'/5020'/0/n)
}

// MetaInfoV0 in Version 0
type MetaInfoV0 struct {
	MoiID      string // MOI id is a string used to identity the node owner by his/her MOI id address
	NodeIndex  int    // NodeIndex is the integer used to identity the nth node of the node owner(m/44'/6174'/5020/0/n)
	ProtocolID int    // ProtocolID is the integer used to identity the protocol underlying the node (Eg., 1=libp2p)
	NodeID     string // Address specific to account under (m/44'/6174'/5020/0/<NodeIndex>) IGCPath
}

type KramaID string

// NewKramaID generates KramaID with given version
func NewKramaID(commPrivBytes []byte,
	nthValidator uint32,
	moiIDAddress string,
	version int,
	isNode bool) (KramaID, error) {
	switch version {
	case 1:
		return generateKramaIDV1(nthValidator, moiIDAddress, commPrivBytes, isNode)
	case 0:
		return generateKramaIDV0(moiIDAddress, int(nthValidator), commPrivBytes)
	default:
		return "", common.ErrInvalidKramaIDVersion
	}
}

// Version returns Krama id's version
func (kid KramaID) Version() (int, error) {
	kramaIDMetaDecoded, _, err := getKramaIDComponents(kid)
	if err != nil {
		return -1, errors.Wrap(common.ErrParsingKramaID, err.Error())
	}

	currentKramaIDPrefix := kramaIDMetaDecoded[0:1]

	switch {
	case bytes.Equal(
		[]byte{NodeTypeVersion1}, currentKramaIDPrefix) || bytes.Equal([]byte{UserTypeVersion1},
		currentKramaIDPrefix):
		return 1, nil
	case len(kramaIDMetaDecoded) > 120:
		return 0, nil
	default:
		return -1, common.ErrInvalidKramaID
	}
}

// Type returns 0 if kramaID belongs to User and 1 if Node
func (kid KramaID) Type() (int, error) {
	kramaIDMetaDecoded, _, err := getKramaIDComponents(kid)
	if err != nil {
		return -1, errors.Wrap(common.ErrParsingKramaID, err.Error())
	}

	if bytes.Equal(
		[]byte{UserTypeVersion0}, kramaIDMetaDecoded[0:1]) || bytes.Equal([]byte{UserTypeVersion1},
		kramaIDMetaDecoded[0:1]) {
		return 0, nil
	}

	return 1, common.ErrInvalidKramaID
}

// MoiID returns the node owner's MOI id public address
func (kid KramaID) MoiID() (string, error) {
	kramaIDMetaDecoded, _, err := getKramaIDComponents(kid)
	if err != nil {
		return "", errors.Wrap(common.ErrParsingKramaID, err.Error())
	}

	v, err := kid.Version()
	if err != nil {
		return "", err
	}

	switch v {
	case 1:
		{
			metaInV1, err := unMarshalV1Meta(kramaIDMetaDecoded[1:])
			if err != nil {
				return "", errors.Wrap(common.ErrParsingKramaID, err.Error())
			}

			return metaInV1.moiID, nil
		}
	case 0:
		{
			metaInV0, err := unMarshalV0Meta(kramaIDMetaDecoded[:])
			if err != nil {
				return "", errors.Wrap(common.ErrParsingKramaID, err.Error())
			}

			return metaInV0.MoiID, nil
		}
	default:
		return "", common.ErrInvalidKramaID
	}
}

// NodeIndex returns n meaning of the validator
func (kid KramaID) NodeIndex() (uint32, error) {
	kramaIDMetaDecoded, _, err := getKramaIDComponents(kid)
	if err != nil {
		return HardenedStartIndex, errors.Wrap(common.ErrParsingKramaID, err.Error())
	}

	v, err := kid.Version()
	if err != nil {
		return HardenedStartIndex, err
	}

	switch v {
	case 1:
		{
			metaInV1, err := unMarshalV1Meta(kramaIDMetaDecoded[1:])
			if err != nil {
				return HardenedStartIndex, err
			}

			return metaInV1.nodeIndex, nil
		}
	case 0:
		{
			metaInV0, err := unMarshalV0Meta(kramaIDMetaDecoded[:])
			if err != nil {
				return HardenedStartIndex, err
			}
			nIndexInU32 := uint32(metaInV0.NodeIndex)

			return nIndexInU32, nil
		}
	default:
		return HardenedStartIndex, common.ErrInvalidKramaID
	}
}

// PeerID returns peer id for communication
func (kid KramaID) PeerID() (string, error) {
	_, p2pID, err := getKramaIDComponents(kid)
	if err != nil {
		return "", err
	}

	return p2pID, nil
}

func getKramaIDComponents(krmID KramaID) ([]byte, string, error) {
	kramaIDInString := string(krmID)

	if kramaIDInString == "" {
		return nil, "", common.ErrInvalidKramaID
	}

	kramaIDComps := strings.Split(kramaIDInString, ".")
	if len(kramaIDComps) != 2 {
		return nil, "", common.ErrInvalidKramaID
	}

	kramaIDMetaDecoded, err := base58.Decode(kramaIDComps[0])
	if err != nil {
		return nil, "", err
	}

	return kramaIDMetaDecoded, kramaIDComps[1], nil
}
