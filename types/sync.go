package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type SyncMode int

const (
	FullSync SyncMode = iota
	LatestSync
)

type AccountSyncStatus struct {
	Address            Address
	ExpectedHeight     uint64
	SnapshotDownloaded bool
	Mode               SyncMode
	CurrentHash        Hash
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
