package kbft

import (
	"io"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

type nullWal struct{}

func (n nullWal) Write(message ktypes.ConsensusMessage, id common.ClusterID) error { return nil }

func (n nullWal) WriteSync(message ktypes.ConsensusMessage, id common.ClusterID) error { return nil }

func (n nullWal) FlushAndSync() error { return nil }

func (n nullWal) SearchForClusterID(
	clusterID string,
	options *WALSearchOptions,
) (rd io.ReadCloser, found bool, err error) {
	return nil, false, nil
}

func (n nullWal) Start() error { return nil }

func (n nullWal) Close() {}
