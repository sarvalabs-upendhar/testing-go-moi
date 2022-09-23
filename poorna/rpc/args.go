package rpc

import "gitlab.com/sarvalabs/moichain/common/ktypes"

// GetBalArgs is a struct that represents an argument wrapper for retrieving balance of an asset
type GetBalArgs struct {
	// Represents the address for which to retrieve the balance
	From string `json:"from"`

	// Represents the asset for which to retrieve balance
	AssetID string `json:"assetid"`
}

// GetTesseract is a struct that represents an argument wrapper for retrieving the latest Tesseract
type GetTesseract struct {
	// Represents the address for which to retrieve the latest Tesseract
	From string `json:"from"`
}

type GetTesseractByHashArgs struct {
	Hash string `json:"hash"`
}

type GetTesseractByHeightArgs struct {
	From   string `json:"from"`
	Height uint64 `json:"height"`
}

type GetAssetInfoArgs struct {
	AssetID string `json:"asset_id"`
}

type GetTransactionCountByAddressArgs struct {
	From   string `json:"from"`
	Status bool   `json:"status"`
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

// GetReceiptArgs is a struct that represent an argument wrapper for retrieving the receipt of an interaction
type GetReceiptArgs struct {
	Address string
	Hash    string
}

// ReceiptResponse is a response wrapper for receipts
type ReceiptResponse struct {
	Receipt ktypes.Receipt
}
