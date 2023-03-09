package kbft

import (
	"io"

	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/types"
)

type nullWal struct{}

func (n nullWal) Write(message ktypes.ConsensusMessage, id types.ClusterID) error { return nil }

func (n nullWal) WriteSync(message ktypes.ConsensusMessage, id types.ClusterID) error { return nil }

func (n nullWal) FlushAndSync() error { return nil }

func (n nullWal) SearchForClusterID(
	clusterID string,
	options *WALSearchOptions,
) (rd io.ReadCloser, found bool, err error) {
	return nil, false, nil
}

func (n nullWal) Start() error { return nil }

func (n nullWal) Close() {}
