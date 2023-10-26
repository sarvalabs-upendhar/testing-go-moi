package badger

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/storage/db"
)

// BatchWriter is used to perform batch writes to badger database, It implements db.BatchWriter interface
type BatchWriter struct {
	bw      *badger.WriteBatch
	metrics *db.Metrics
}

func (b *BatchWriter) Set(key, value []byte) error {
	b.metrics.CaptureDBWrites(1)

	return b.bw.Set(key, value)
}

// Flush commits all the entries to database
func (b *BatchWriter) Flush() error {
	return b.bw.Flush()
}

// WriteBuffer unmarshal the key-value entries and add the entries to batch writer
// The structure of the buffer is database specific, currently for badger, buffer is serialized KVList
func (b *BatchWriter) WriteBuffer(buf []byte) error {
	err := z.NewBufferSlice(buf).SliceIterate(func(slice []byte) error {
		kv := new(pb.KV)
		err := kv.Unmarshal(slice)
		if err != nil {
			return err
		}

		if err := b.Set(kv.Key, kv.Value); err != nil {
			return errors.Wrap(err, "failed to write list")
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
