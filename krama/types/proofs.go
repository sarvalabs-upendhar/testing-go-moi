package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/types"
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

func (wdp *WatchDogProofs) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(wdp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize watch dog proofs")
	}

	return rawData, nil
}
