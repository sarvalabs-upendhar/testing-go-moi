package common

import (
	"bytes"
	"encoding/binary"

	"github.com/sarvalabs/go-moi/common/identifiers"
)

const KMOITokenSymbol = "KMOI"

var GenesisIxHash = GetHash([]byte("Genesis Interaction"))

var (
	SargaLogicID   = CreateLogicIDFromString("sargaAccount", 0, identifiers.Systemic)
	SargaAccountID = SargaLogicID.AsIdentifier()
)

var (
	SystemLogicID = CreateLogicIDFromString(
		"system-registry",
		0,
		identifiers.Systemic,
		identifiers.LogicIntrinsic,
		identifiers.LogicExtrinsic,
	)
	SystemAccountID = SystemLogicID.AsIdentifier()
)

var (
	KMOITokenAssetID = CreateAssetIDFromString(
		KMOITokenSymbol,
		0,
		uint16(MAS0),
		identifiers.Systemic,
	)
	KMOITokenAccountID = KMOITokenAssetID.AsIdentifier()
)

func ContainsID(ids []identifiers.Identifier, target identifiers.Identifier) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}

	return false
}

func CreateParticipantIDFromString(name string, variant uint32, flags ...identifiers.Flag) identifiers.Identifier {
	hash := GetHash([]byte(name)).Bytes()[:24]

	var AccountID [24]byte

	copy(AccountID[:], hash[:24])

	participantID, _ := identifiers.GenerateParticipantIDv0(AccountID, variant, flags...)

	return participantID.AsIdentifier()
}

func CreateLogicIDFromString(name string, variant uint32, flags ...identifiers.Flag) identifiers.LogicID {
	hash := GetHash([]byte(name)).Bytes()[:24]

	var AccountID [24]byte

	copy(AccountID[:], hash[:24])

	logicID, _ := identifiers.GenerateLogicIDv0(AccountID, variant, flags...)

	return logicID
}

func CreateAssetIDFromString(
	name string,
	variant uint32,
	standard uint16,
	flags ...identifiers.Flag,
) identifiers.AssetID {
	hash := GetHash([]byte(name)).Bytes()[:24]

	var AccountID [24]byte

	copy(AccountID[:], hash[:24])

	assetID, _ := identifiers.GenerateAssetIDv0(AccountID, variant, standard, flags...)

	return assetID
}

func NewAccounIDFromBytes(b []byte) [24]byte {
	var uniqueID [24]byte

	copy(uniqueID[:], b[:24])

	return uniqueID
}

func NewAccountID(sender Sender) [24]byte {
	rawBytes := make([]byte, 48)
	binary.BigEndian.PutUint64(rawBytes[:8], sender.SequenceID)
	binary.BigEndian.PutUint64(rawBytes[8:16], sender.KeyID)

	copy(rawBytes[16:], sender.ID.Bytes())

	hash := GetHash(rawBytes).Bytes()

	var hashArray [24]byte

	copy(hashArray[:], hash[:24])

	return hashArray
}

type IdentifierList []identifiers.Identifier

func (iList IdentifierList) Len() int {
	return len(iList)
}

func (iList IdentifierList) Less(i, j int) bool {
	if polarity := bytes.Compare(iList[i].Bytes(), iList[j].Bytes()); polarity < 0 {
		return true
	}

	return false
}

func (iList IdentifierList) Swap(i, j int) {
	iList[i], iList[j] = iList[j], iList[i]
}

func (iList IdentifierList) Has(id identifiers.Identifier) bool {
	for _, identifier := range iList {
		if identifier == id {
			return true
		}
	}

	return false
}

func IsSystemAccount(id identifiers.Identifier) bool {
	if id == SargaAccountID || id == SystemAccountID {
		return true
	}

	return false
}
