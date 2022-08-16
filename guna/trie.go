package guna

import (
	"errors"
	"log"

	smt "github.com/aergoio/aergo/pkg/trie"
	"github.com/ipfs/go-cid"
	dhruva "gitlab.com/sarvalabs/moichain/dhruva"
)

type Trie interface {
	TryGet(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Root() []byte
	Copy() Trie
}
type SmtTrie struct {
	db *dhruva.PersistenceManager
	//cache *lru.Cache
	smt *smt.Trie
}

func NewSmtTrie(root []byte, db *dhruva.PersistenceManager) *SmtTrie {
	s := &SmtTrie{
		db:  db,
		smt: smt.NewTrie(root, hashFunc, nil),
	}

	return s
}

func (t *SmtTrie) Copy() Trie {
	newTrie := NewSmtTrie(t.Root(), t.db)

	return newTrie
}

func hashFunc(data ...[]byte) []byte {
	buf := make([]byte, 0)

	for _, v := range data {
		buf = append(buf, v...)
	}

	v1 := cid.V1Builder{
		Codec:    0x50,
		MhType:   0xb220,
		MhLength: -1,
	}

	hash, err := v1.Sum(buf)
	if err != nil {
		log.Fatal(err)
	}

	return hash.Bytes()
}
func (t *SmtTrie) Root() []byte {
	return t.smt.Root
}
func (t *SmtTrie) TryGet(key []byte) ([]byte, error) {
	value, err := t.smt.Get(key)
	if err != nil {
		return nil, err
	}

	if value == nil {
		return nil, errors.New("key Not Found")
	}

	return t.db.ReadEntry(value)
}

func (t *SmtTrie) Put(key, value []byte) error {
	cid, err := t.db.CreateCidEntry(value)
	if err != nil {
		log.Fatal(err)
	}

	keys := make([][]byte, 0)
	values := make([][]byte, 0)

	keys = append(keys, key)
	values = append(values, cid)
	_, err = t.smt.Update(keys, values)

	return err
}
