package block

import (
	"github.com/sarvalabs/moichain/syncer/cid"
)

type HaveList struct {
	blocks []Block
}

func NewHaveList() HaveList {
	return HaveList{
		blocks: make([]Block, 0),
	}
}

func (h *HaveList) Size() int {
	return len(h.blocks)
}

func (h *HaveList) GetKeys() []cid.CID {
	cIDs := make([]cid.CID, len(h.blocks))

	for k, v := range h.blocks {
		cIDs[k] = v.cid
	}

	return cIDs
}

func (h *HaveList) GetBlocks() []Block {
	return h.blocks
}

func (h *HaveList) AddBlock(b Block) {
	h.blocks = append(h.blocks, b)
}

func (h *HaveList) GetRawBlocks() [][]byte {
	rawBlocks := make([][]byte, 0, len(h.blocks))

	for _, block := range h.blocks {
		rawBlocks = append(rawBlocks, block.BytesForMessage())
	}

	return rawBlocks
}
