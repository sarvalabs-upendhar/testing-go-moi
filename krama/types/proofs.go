package types

import "gitlab.com/sarvalabs/moichain/types"

type WatchDogProofs struct {
	MetaData *types.ICSMetaInfo
	Extra    []byte
}
