package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type AccountsStatusMsg struct {
	TotalAccounts []byte
	BucketSizes   map[int32][]byte
	NTQ           float32
}

type AccountSyncRequest struct {
	BulkSync bool
	Bucket   int32
	Address  identifiers.Address
}

func (asr *AccountSyncRequest) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(asr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account sync request")
	}

	return rawData, nil
}

func (asr *AccountSyncRequest) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(asr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account sync request")
	}

	return nil
}

type AccountSyncResponse struct {
	Slot     int32
	Bucket   int32
	Accounts []*common.AccountMetaInfo
}

func (asr *AccountSyncResponse) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(asr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account sync request")
	}

	return nil
}
