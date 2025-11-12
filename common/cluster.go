package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/mr-tron/base58"
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

func GenerateClusterID() (ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return ClusterID(base58.Encode(randHash)), nil
}

type ExecutionContext struct {
	CtxDelta ContextDelta
	Cluster  ClusterID
	Time     uint64
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
