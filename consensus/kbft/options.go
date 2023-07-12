package kbft

import (
	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/go-moi/common/utils"
)

type Option = func(kbft *KBFT)

func withDefaultEventMux() Option {
	return func(kbft *KBFT) {
		kbft.mux = &utils.TypeMux{}
	}
}

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
