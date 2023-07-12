package block

import (
	"github.com/sarvalabs/moichain/syncer/cid"
	"golang.org/x/crypto/blake2b"
)

const MaxPeerListSize = 5

type Block struct {
	cid  cid.CID
	data []byte
}

func NewBlock(cid cid.CID, data []byte) *Block {
	return &Block{
		cid:  cid,
		data: data,
	}
}

func NewBlockFromMessage(data []byte) Block {
	hash := blake2b.Sum256(data[1:])

	return Block{
		cid:  cid.ContentID(data[0], hash),
		data: data[1:],
	}
}

func NewBlockFromRawData(contentType byte, data []byte) Block {
	hash := blake2b.Sum256(data)

	return Block{
		cid:  cid.ContentID(contentType, hash),
		data: data,
	}
}

func (b Block) GetData() []byte {
	return b.data
}

func (b Block) GetCid() cid.CID {
	return b.cid
}

func (b Block) BytesForMessage() []byte {
	rawBytes := make([]byte, 0, len(b.data)+1)

	rawBytes = append(rawBytes, b.cid.ContentType())
	rawBytes = append(rawBytes, b.data...)

	return rawBytes
}
