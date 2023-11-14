package syncer

import (
	"context"

	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type Session interface {
	ID() common.Address
	GetBlock(ctx context.Context, cID cid.CID) (*block.Block, error)
	GetBlocks(ctx context.Context, cids []cid.CID) chan *block.Block
	Close()
}

type BlockSync interface {
	NewSession(
		ctx context.Context,
		contextPeers []id.KramaID,
		address common.Address,
		stateHash cid.CID,
	) (Session, error)
	Start()
	Close()
}
