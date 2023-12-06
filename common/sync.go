package common

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type SyncMode int

func (m SyncMode) String() string {
	switch m {
	case FullSync:
		return "FullSync"
	case LatestSync:
		return "LatestSync"
	default:
		return "Invalid Sync Mode"
	}
}

const (
	FullSync SyncMode = iota
	LatestSync
)

type AccountSyncStatus struct {
	Address            Address
	ExpectedHeight     uint64
	SnapshotDownloaded bool
	Mode               SyncMode
	State              int32
	LastModifiedAt     []byte
}

func (cj *AccountSyncStatus) Bytes() ([]byte, error) {
	return polo.Polorize(cj)
}

func (cj *AccountSyncStatus) FromBytes(rawData []byte) error {
	if err := polo.Depolorize(cj, rawData); err != nil {
		return errors.Wrap(err, "failed to depolarise account sync status")
	}

	return nil
}
