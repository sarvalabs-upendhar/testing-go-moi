package types

import (
	"encoding/hex"
	"log"
	"math/big"
)

type AssetInfo struct {
	Owner       string
	Dimension   uint8
	TotalSupply uint64
	Symbol      string
	IsFungible  bool
	IsMintable  bool
	LogicID     LogicID
}

type AccountMetaInfo struct {
	Address       Address
	Type          AccType
	Mode          string
	Height        *big.Int
	TesseractHash Hash
	LatticeExists bool
	StateExists   bool
}

// AssetID ...
type AssetID string

func (aID AssetID) GetCID() []byte {
	data, err := hex.DecodeString(string(aID))
	if err != nil {
		return nil
	}

	return data[2:]
}

func (aID AssetID) GetDimension() (DimensionID, error) {
	data, err := hex.DecodeString(string(aID))
	if err != nil {
		return 0, err
	}

	return DimensionID(data[0]), nil
}

func (aID AssetID) IsFungible() bool {
	data, err := hex.DecodeString(string(aID))
	if err != nil {
		log.Fatal(err)
	}

	if data[1]&(0x01<<7) == 0x80 {
		return true
	}

	return false
}

func (aID AssetID) IsMintable() bool {
	data, err := hex.DecodeString(string(aID))
	if err != nil {
		log.Fatal(err)
	}

	if 0x01&data[1] == 1 {
		return true
	}

	return false
}

type DimensionID byte

const (
	Economic DimensionID = iota
	Possession
)

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

// LogicID ...
type LogicID string

func (l LogicID) Bytes() []byte {
	rawID, err := hex.DecodeString(string(l))
	if err != nil {
		return nil
	}

	return rawID
}
