package syncer

import (
	"context"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type Session interface {
	ID() identifiers.Address
	GetBlock(ctx context.Context, cID cid.CID) (*block.Block, error)
	GetBlocks(ctx context.Context, cids []cid.CID) chan *block.Block
	Close()
}

type BlockSync interface {
	NewSession(
		ctx context.Context,
		contextPeers []kramaid.KramaID,
		address identifiers.Address,
		stateHash cid.CID,
	) (Session, error)
	Start()
	Close()
}
