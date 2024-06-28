package common

import (
	"encoding/hex"
	"fmt"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
)

// ClusterID ...
type ClusterID string

func (c ClusterID) String() string {
	return string(c)
}

func (c ClusterID) Hash() Hash {
	rawHash, err := base58.Decode(c.String())
	if err != nil {
		return NilHash
	}

	return BytesToHash(rawHash)
}

type ExecutionContext struct {
	Participants map[identifiers.Address]IxParticipant // TODO: This should be replaced with transition object
	CtxDelta     ContextDelta
	Cluster      ClusterID
	Time         uint64
}

func (ctx ExecutionContext) Timestamp() uint64 {
	return ctx.Time
}

func (ctx ExecutionContext) ClusterID() string {
	return ctx.Cluster.String()
}

func (ctx ExecutionContext) ContextDelta() ContextDelta {
	return ctx.CtxDelta
}

type ICSClusterInfo struct {
	RandomSet                 []kramaid.KramaID
	RandomSetSizeWithoutDelta uint32
	ObserverSet               []kramaid.KramaID
	Responses                 []*ArrayOfBits
}

func (ci *ICSClusterInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ci)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics cluster info")
	}

	return rawData, nil
}

func (ci *ICSClusterInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ci, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics cluster info")
	}

	return nil
}

type LotteryKey [64]byte

func NewLotteryKey(ixHash Hash, icsSeed [32]byte) LotteryKey {
	var array [64]byte

	copy(array[:32], ixHash.Bytes())
	copy(array[32:], icsSeed[:])

	return array
}

func (lk LotteryKey) String() string {
	return fmt.Sprintf("ix-hash 0x%s seed 0x%s", hex.EncodeToString(lk[:32]), hex.EncodeToString(lk[32:]))
}

func (lk *LotteryKey) IxHash() Hash {
	return BytesToHash(lk[:32])
}

func (lk *LotteryKey) Seed() Hash {
	return BytesToHash(lk[32:])
}
