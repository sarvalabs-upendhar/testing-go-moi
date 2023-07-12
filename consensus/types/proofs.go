package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
)

type ICSMetaInfo struct {
	ClusterID    string
	IxHash       common.Hash
	Operator     string
	ClusterSize  int
	ContextDelta map[string][]string
	GridID       common.Hash
	BinaryHash   common.Hash
	IdentityHash common.Hash
	IcsHash      common.Hash
	ReceiptHash  common.Hash
	Msgs         [][]byte
}

type WatchDogProofs struct {
	MetaData *ICSMetaInfo
	Extra    []byte
}

func (wdp *WatchDogProofs) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(wdp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize watch dog proofs")
	}

	return rawData, nil
}
