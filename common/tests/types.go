package tests

import (
	"time"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

// DefaultLocalWaitTime tells amount of time it takes for initial sync to happen on all nodes in CI/CD pipeline
const DefaultLocalWaitTime = 10 * time.Second

type AccountWithMnemonic struct {
	Addr     identifiers.Address `json:"address"`
	Mnemonic string              `json:"mnemonic"`
}
