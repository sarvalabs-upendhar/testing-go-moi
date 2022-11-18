package types

import (
	"gitlab.com/sarvalabs/moichain/types"
)

type ICSMetaInfo struct {
	ClusterID    string
	IxHash       types.Hash
	Operator     string
	ClusterSize  int
	ContextDelta map[string][]string
	GridID       types.Hash
	BinaryHash   types.Hash
	IdentityHash types.Hash
	IcsHash      types.Hash
	ReceiptHash  types.Hash
	Msgs         [][]byte
}

type WatchDogProofs struct {
	MetaData *ICSMetaInfo
	Extra    []byte
}
