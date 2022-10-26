package api

import "gitlab.com/sarvalabs/moichain/common/ktypes"

// TesseractArgs is a struct that represents an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	// Represents the address for which to retrieve the latest Tesseract
	From string `json:"from"`
	Hash string `json:"hash"`
}

type ContextInfoByHashArgs struct {
	// Represents the address for which to retrieve the latest Tesseract
	From string `json:"from"`
	Hash string `json:"hash"`
}

type TesseractByHashArgs struct {
	Hash string `json:"hash"`
}

type TesseractByHeightArgs struct {
	From   string `json:"from"`
	Height uint64 `json:"height"`
}

type AssetInfoArgs struct {
	AssetID string `json:"asset_id"`
}

type InteractionCountArgs struct {
	From   string `json:"from"`
	Status bool   `json:"status"`
}

// BalArgs is a struct that represents an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	// Represents the address for which to retrieve the balance
	From string `json:"from"`

	// Represents the asset for which to retrieve balance
	AssetID string `json:"assetid"`
}

// SendIXArgs is a struct that represents an argument wrapper for sending Interactions to the pool
type SendIXArgs struct {
	IxType        ktypes.IxType `json:"type"`
	From          string        `json:"from"`
	To            string        `json:"to"`
	Value         int           `json:"value"`
	AssetID       string        `json:"asset_id"`
	AnuPrice      uint64        `json:"anu_price"`
	AssetCreation AssetCreation `json:"asset_creation,omitempty"`
}

type AssetCreation struct {
	Symbol      string `json:"symbol"`
	TotalSupply uint64 `json:"total_supply"`
	IsFungible  bool   `json:"isFungible"`
	IsMintable  bool   `json:"isMintable"`
	Code        []byte `json:"logic,omitempty"`
	Dimension   int    `json:"dimension"`
}

// Response is a struct that represents a response wrapper
type Response struct {
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data"`
}

// ContextResponse is a response object for fetching context info
type ContextResponse struct {
	BehaviourNodes []string
	RandomNodes    []string
	StorageNodes   []string
}

// ReceiptArgs is a struct that represent an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Address string
	Hash    string
}

// ReceiptResponse is a response wrapper for receipts
type ReceiptResponse struct {
	Receipt ktypes.Receipt
}
