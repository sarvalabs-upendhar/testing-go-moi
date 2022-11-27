package types

import (
	"encoding/hex"
	"math/big"

	"github.com/pkg/errors"

	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

const (
	MaxBehaviourContextSize = 8
	MaxRandomContextSize    = 7
)

type AssetMap map[types.AssetID]*big.Int

type BalanceObject struct {
	Bal      AssetMap
	LogicBal map[types.Hash]AssetMap
	PrvHash  types.Hash
}

func (b *BalanceObject) TDU() (AssetMap, types.Hash) {
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

func (b *BalanceObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(b)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize balance object")
	}

	return rawData, nil
}

func (b *BalanceObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(b, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize balance object")
	}

	return nil
}

type ApprovalObject struct {
	Approvals map[types.Address]AssetMap
	PrvHash   types.Hash
}

func (a *ApprovalObject) Copy() *ApprovalObject {
	newObject := new(ApprovalObject)
	newObject.PrvHash = a.PrvHash
	newObject.Approvals = make(map[types.Address]AssetMap)

	for k, v := range a.Approvals {
		newObject.Approvals[k] = v
	}

	return newObject
}

type AssetData struct {
	LogicID types.LogicID
	Symbol  string
	Owner   types.Address
	Extra   []byte
}

func (ad *AssetData) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ad, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize asset data")
	}

	return nil
}

func GetAssetID(
	addr types.Address,
	dimension uint8,
	isFungible bool,
	isMintable bool,
	symbol string,
	totalSupply int64,
	logicID types.LogicID,
) (types.AssetID, types.Hash, []byte, error) {
	var (
		buf  []byte
		info uint8 = 0x00
	)

	assetData := AssetData{
		Owner:   addr,
		Symbol:  symbol,
		LogicID: logicID,
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

	data, err := polo.Polorize(assetData)
	if err != nil {
		return "", types.NilHash, nil, err
	}

	assetCID := types.GetHash(data)

	buf = append(buf, assetCID.Bytes()...)
	aID := types.AssetID(hex.EncodeToString(buf))

	return aID, assetCID, data, nil
}

type Context interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type MetaContextObject struct {
	BehaviouralContext types.Hash
	RandomContext      types.Hash
	StorageContext     types.Hash
	ComputeContext     types.Hash
	DefaultMTQ         int32
	PreviousHash       types.Hash
}

func (m *MetaContextObject) Copy() *MetaContextObject {
	newObject := new(MetaContextObject)
	newObject.BehaviouralContext = m.BehaviouralContext
	newObject.RandomContext = m.RandomContext
	newObject.ComputeContext = m.ComputeContext
	newObject.DefaultMTQ = m.DefaultMTQ

	return newObject
}

func (m *MetaContextObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize meta context object")
	}

	return rawData, nil
}

func (m *MetaContextObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(m, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize meta context object")
	}

	return nil
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

func (c *ContextObject) Hash() (types.Hash, error) {
	hash, err := types.PoloHash(c)
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to polorize context object")
	}

	return hash, nil
}

func (c *ContextObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize context object")
	}

	return rawData, nil
}

func (c *ContextObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(c, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize context object")
	}

	return nil
}

type LogicData struct {
	Code       []byte
	Upgradable bool
}

func (ld *LogicData) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ld, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize logic data")
	}

	return nil
}

func GetLogicID(code []byte, isUpgradable bool) (types.LogicID, *LogicData, error) {
	ld := &LogicData{
		Code:       code,
		Upgradable: isUpgradable,
	}

	rawData, err := polo.Polorize(ld)
	if err != nil {
		return "", nil, err
	}

	x := blake2b.Sum256(rawData)
	logicID := types.BytesToHash(x[:])

	return types.LogicID(logicID.String()), ld, nil
}
