package types

import "gitlab.com/sarvalabs/moichain/common/ktypes"

type WatchDogProofs struct {
	MetaData *ktypes.ICSMetaInfo
	Extra    []byte
}
