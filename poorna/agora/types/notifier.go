package types

import (
	"context"
	"github.com/cskr/pubsub"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
)

const BufferSize = 16

type PubSub interface {
	Publish(block Block)
	Subscribe(ctx context.Context, keys ...ktypes.Hash) <-chan Block
	Shutdown()
}

// NewNotifier generates a new PubSub interface.
func NewNotifier() PubSub {
	return &impl{
		wrapped: *pubsub.New(BufferSize),
		closed:  make(chan struct{}),
	}
}

type impl struct {
	lk      sync.RWMutex
	wrapped pubsub.PubSub

	closed chan struct{}
}

func (ps *impl) Publish(block Block) {
	ps.lk.RLock()
	defer ps.lk.RUnlock()
	select {
	case <-ps.closed:
		return
	default:
	}

	ps.wrapped.Pub(block, block.GetID().Hex())
}

func (ps *impl) Shutdown() {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	select {
	case <-ps.closed:
		return
	default:
	}
	close(ps.closed)
	ps.wrapped.Shutdown()
}

// Subscribe returns a channel of blocks for the given |keys|. |blockChannel|
// is closed if the |ctx| times out or is cancelled, or after receiving the blocks
// corresponding to |keys|.
func (ps *impl) Subscribe(ctx context.Context, keys ...ktypes.Hash) <-chan Block {
	blocksCh := make(chan Block, len(keys))
	valuesCh := make(chan interface{}, len(keys)) // provide our own channel to control buffer, prevent blocking

	if len(keys) == 0 {
		close(blocksCh)

		return blocksCh
	}

	// prevent shutdown
	ps.lk.RLock()
	defer ps.lk.RUnlock()

	select {
	case <-ps.closed:
		close(blocksCh)

		return blocksCh
	default:
	}

	// AddSubOnceEach listens for each key in the list, and closes the channel
	// once all keys have been received
	ps.wrapped.AddSubOnceEach(valuesCh, toStrings(keys)...)

	go func() {
		defer func() {
			close(blocksCh)

			ps.lk.RLock()
			defer ps.lk.RUnlock()
			// Don't touch the pubsub instance if we're
			// already closed.
			select {
			case <-ps.closed:
				return
			default:
			}

			ps.wrapped.Unsub(valuesCh)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ps.closed:
			case val, ok := <-valuesCh:
				if !ok {
					return
				}

				block, ok := val.(Block)
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
					return
				case blocksCh <- block: // continue
				case <-ps.closed:
				}
			}
		}
	}()

	return blocksCh
}

func toStrings(hashes []ktypes.Hash) []string {
	keys := make([]string, 0, len(hashes))
	for _, v := range hashes {
		keys = append(keys, v.Hex())
	}

	return keys
}
