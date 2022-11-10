package types

import (
	"encoding/hex"
	"log"
	"math/big"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/crypto/blake2b"
)

const (
	MaxBehaviourContextSize = 8
	MaxRandomContextSize    = 7
)

type BalanceObject struct {
	Bal      AssetMap
	LogicBal map[Hash]AssetMap
	PrvHash  Hash
}

type AssetMap map[AssetID]*big.Int

type MetaContextObject struct {
	BehaviouralContext Hash
	RandomContext      Hash
	StorageContext     Hash
	ComputeContext     Hash
	DefaultMTQ         int32
	PreviousHash       Hash
}

func (m *MetaContextObject) Copy() *MetaContextObject {
	newObject := new(MetaContextObject)
	newObject.BehaviouralContext = m.BehaviouralContext
	newObject.RandomContext = m.RandomContext
	newObject.ComputeContext = m.ComputeContext
	newObject.DefaultMTQ = m.DefaultMTQ

	return newObject
}

type LogicData struct {
	Code       []byte
	Upgradable bool
}

func (b *BalanceObject) TDU() (AssetMap, Hash) {
	return b.Bal, b.PrvHash
}

func (b *BalanceObject) Copy() *BalanceObject {
	newObject := new(BalanceObject)
	newObject.Bal = make(AssetMap)

	for k, v := range b.Bal {
		newObject.Bal[k] = v
	}

	return newObject
}

type ApprovalObject struct {
	Approvals map[Address]AssetMap
	PrvHash   Hash
}

func (a *ApprovalObject) Copy() *ApprovalObject {
	newObject := new(ApprovalObject)
	newObject.PrvHash = a.PrvHash
	newObject.Approvals = make(map[Address]AssetMap)

	for k, v := range a.Approvals {
		newObject.Approvals[k] = v
	}

	return newObject
}

type DimensionID byte

const (
	Economic DimensionID = iota
	Possession
)

// func (dID dimensionID) Toint8() uint8

func (d DimensionID) String() string {
	switch d {
	case Economic:
		return "Economic"

	case Possession:
		return "Possession"
	}

	return ""
}

func StringToDimensionID(str string) DimensionID {
	switch str {
	case "Economic":
		return Economic
	case "Possession":
		return Possession
	}

	return 0
}

type AssetData struct {
	LogicID Hash
	Symbol  string
	Owner   Address
	Extra   []byte
}

func (aID *AssetID) GetCID() ([]byte, error) {
	data, err := hex.DecodeString(string(*aID))
	if err != nil {
		return nil, err
	}

	return data[2:], nil
}

func (aID *AssetID) GetDimension() (DimensionID, error) {
	data, err := hex.DecodeString(string(*aID))
	if err != nil {
		return 0, err
	}

	return DimensionID(data[0]), nil
}

func (aID *AssetID) IsFungible() bool {
	data, err := hex.DecodeString(string(*aID))
	if err != nil {
		log.Fatal(err)
	}

	if data[1]&(0x01<<7) == 0x80 {
		return true
	}

	return false
}

func (aID *AssetID) IsMintable() bool {
	data, err := hex.DecodeString(string(*aID))
	if err != nil {
		log.Fatal(err)
	}

	if 0x01&data[1] == 1 {
		return true
	}

	return false
}

type ContextObject struct {
	Ids []id.KramaID
}

func (c *ContextObject) AddNodes(nodes []id.KramaID, maxSize int) {
	c.Ids = append(c.Ids, nodes...)
	if diff := len(c.Ids) - maxSize; diff > 0 {
		c.Ids = c.Ids[diff:]
	}
}

func (c *ContextObject) Copy() *ContextObject {
	newSlice := make([]id.KramaID, len(c.Ids))

	copy(newSlice, c.Ids)

	newObject := new(ContextObject)
	newObject.Ids = newSlice

	return newObject
}

func (c *ContextObject) Hash() Hash {
	return PoloHash(c)
}

func GetLogicID(code []byte, isUpgradable bool) (Hash, *LogicData) {
	ld := &LogicData{
		Code:       code,
		Upgradable: isUpgradable,
	}
	data := polo.Polorize(ld)
	x := blake2b.Sum256(data)
	logicID := BytesToHash(x[:])

	return logicID, ld
}

func GetAssetID(
	addr Address,
	dimension uint8,
	isFungible bool,
	isMintable bool,
	symbol string,
	totalSupply int64,
	logicID Hash,
) (AssetID, Hash, []byte) {
	var (
		buf  []byte
		info uint8 = 0x00
	)

	assetData := AssetData{
		Owner:  addr,
		Symbol: symbol,
	}

	if isMintable {
		info |= 0x01
	} else {
		assetData.Extra = big.NewInt(totalSupply).Bytes()
	}

	if isFungible {
		info |= 0x80
	}

	buf = append(buf, dimension)
	buf = append(buf, info)
	assetData.LogicID = logicID

	data := polo.Polorize(assetData)
	assetCID := GetHash(data)

	buf = append(buf, assetCID.Bytes()...)
	aID := AssetID(hex.EncodeToString(buf))

	return aID, assetCID, data
}
