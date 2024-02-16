package block

import (
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/syncer/cid"
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
	if storage.PrefixTag(data[0]).IsAccountBasedKey() {
		hash := blake2b.Sum256(data[1:])

		return Block{
			cid:  cid.ContentID(data[0], hash),
			data: data[1:],
		}
	}

	return Block{
		cid:  cid.CID(data[:33]),
		data: data[33:],
	}
}

func NewNonAccountBlockFromRawData(cid cid.CID, data []byte) Block {
	return Block{
		cid:  cid,
		data: data,
	}
}

func NewAccountBlockFromRawData(contentType byte, data []byte) Block {
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
	if storage.PrefixTag(b.cid.ContentType()).IsAccountBasedKey() {
		rawBytes := make([]byte, 0, len(b.data)+1)
		rawBytes = append(rawBytes, b.cid.ContentType())
		rawBytes = append(rawBytes, b.data...)

		return rawBytes
	}

	rawBytes := make([]byte, 0, len(b.cid)+len(b.data))
	rawBytes = append(rawBytes, b.cid.Bytes()...)
	rawBytes = append(rawBytes, b.data...)

	return rawBytes
}
