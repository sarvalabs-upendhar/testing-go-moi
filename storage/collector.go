package storage

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/ristretto/z"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
)

const (
	maxMessageSize = 256 * 1024 // 256KB
)

type KVCollector struct {
	logger   hclog.Logger
	MaxSize  uint64
	Size     uint64
	ctx      context.Context
	respChan chan<- common.SnapResponse
}

func NewKVCollector(
	ctx context.Context,
	logger hclog.Logger,
	maxSize uint64,
	respChan chan<- common.SnapResponse,
) *KVCollector {
	return &KVCollector{
		logger:   logger,
		MaxSize:  maxSize,
		Size:     0,
		ctx:      ctx,
		respChan: respChan,
	}
}

func (c *KVCollector) sendSnapResp(resp common.SnapResponse) error {
	select {
	case c.respChan <- resp:
	case <-c.ctx.Done():
		return c.ctx.Err()
	}

	return nil
}

// Send sends the size of the buffered data with a start signal and breaks the buffer into 4 KB parts,
// sending them over the network.
// It then sends an end signal so that the receiver can flush the snapped part into the database.
func (c *KVCollector) Send(buf *z.Buffer) error {
	if c.Size+uint64(buf.LenNoPadding()) > c.MaxSize {
		return errors.New(fmt.Sprintf("Oversize Snapshot %d, %d, %d",
			c.Size, uint64(buf.LenNoPadding()), c.MaxSize))
	}

	if buf.LenNoPadding() > 0 {
		var (
			snapData = buf.Bytes()
			snapResp = common.SnapResponse{}
		)

		c.logger.Trace("snap part size", "size", len(snapData))

		if err := c.sendSnapResp(common.SnapResponse{
			Start:     true,
			ChunkSize: uint64(len(snapData)),
		}); err != nil {
			return err
		}

		for len(snapData) >= maxMessageSize {
			snapResp.Data = snapData[:maxMessageSize]
			if err := c.sendSnapResp(snapResp); err != nil {
				return err
			}

			snapData = snapData[maxMessageSize:]
		}

		// if < 4kb snap is left then it will be sent here
		if len(snapData) != 0 {
			snapResp.Data = snapData
			if err := c.sendSnapResp(snapResp); err != nil {
				return err
			}
		}
	}

	if err := c.sendSnapResp(common.SnapResponse{
		End: true,
	}); err != nil {
		return err
	}

	c.Size += uint64(buf.LenNoPadding())
	c.logger.Trace("Sent Snap info ", "current snap size", c.Size)

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
