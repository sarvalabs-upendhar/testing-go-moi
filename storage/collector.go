package storage

import (
	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
)

type KVCollector struct {
	MaxSize uint64
	Size    uint64
	Entries []byte
}

func NewKVCollector(maxSize uint64) *KVCollector {
	return &KVCollector{
		MaxSize: maxSize,
		Size:    0,
		Entries: make([]byte, 0),
	}
}

func (c *KVCollector) Send(buf *z.Buffer) error {
	if c.Size+uint64(buf.LenNoPadding()) > c.MaxSize {
		return errors.New("Oversize Snapshot")
	}

	if buf.LenNoPadding() > 0 {
		c.Entries = append(c.Entries, buf.Bytes()...)
		c.Size += uint64(buf.LenNoPadding())
	}

	return nil
}

type ValueCollector struct {
	MaxSize uint64
	Entries [][]byte
}

func (v *ValueCollector) Send(buf *z.Buffer) error {
	if err := buf.SliceIterate(func(slice []byte) error {
		kv := new(pb.KV)
		err := kv.Unmarshal(slice)
		if err != nil {
			return err
		}
		if uint64(len(v.Entries)+len(kv.Value)) > v.MaxSize {
			return errors.New("Oversize Snapshot")
		}
		v.Entries = append(v.Entries, kv.Value)

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func NewValueCollector(maxSize uint64) *ValueCollector {
	return &ValueCollector{
		MaxSize: maxSize,
		Entries: make([][]byte, 0),
	}
}
