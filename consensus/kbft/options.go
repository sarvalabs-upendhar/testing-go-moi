package kbft

import (
	"github.com/hashicorp/go-hclog"
)

type Option = func(kbft *KBFT)

func WithEvidence(evidence *Evidence) Option {
	return func(kbft *KBFT) {
		kbft.evidence = evidence
	}
}

func WithWal(wal WAL) Option {
	return func(kbft *KBFT) {
		kbft.wal = wal
	}
}

func WithLogger(logger hclog.Logger) Option {
	return func(kbft *KBFT) {
		kbft.logger = logger
	}
}
