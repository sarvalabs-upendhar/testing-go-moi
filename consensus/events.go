package consensus

import (
	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

type eventPrepare struct {
	prepare *ktypes.Prepare
}

type eventPrepared struct {
	prepared *ktypes.Prepared
}

type eventCleanup struct {
	clusterID common.ClusterID
}
