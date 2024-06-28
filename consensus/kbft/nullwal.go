package kbft

import (
	"io"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

type NullWal struct{}

func (n NullWal) Write(message ktypes.ConsensusMessage, id common.ClusterID) error { return nil }

func (n NullWal) WriteSync(message ktypes.ConsensusMessage, id common.ClusterID) error { return nil }

func (n NullWal) FlushAndSync() error { return nil }

func (n NullWal) SearchForClusterID(
	clusterID string,
	options *WALSearchOptions,
) (rd io.ReadCloser, found bool, err error) {
	return nil, false, nil
}

func (n NullWal) Start() error { return nil }

func (n NullWal) Close() {}
