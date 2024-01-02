package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type ICSResponseCode int32

const (
	SlotsFull ICSResponseCode = iota + 1
	InvalidHash
	InvalidInteractions
	InternalError
	Success
)

type CanonicalICSRequest struct {
	ClusterID            string
	Operator             string
	ContextLock          map[identifiers.Address]common.ContextLockInfo
	IxData               []byte
	Ntq                  int32
	Timestamp            int64
	StakingContractState common.Hash
	ContextType          int32
}

func (ics CanonicalICSRequest) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical ics request")
	}

	return rawData, nil
}

func (ics *CanonicalICSRequest) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ics, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize canonical ics request")
	}

	return nil
}

type ICSRequest struct {
	ReqData   []byte
	Signature []byte
}

func NewICSRequest(rawICSReqData []byte, signature []byte) ICSRequest {
	return ICSRequest{
		ReqData:   rawICSReqData,
		Signature: signature,
	}
}

type ICSResponse struct {
	ClusterID   string
	StatusCode  ICSResponseCode
	RandomNodes []string
}

type ICSSuccessMsg struct {
	ClusterID   string
	RandomSet   []kramaid.KramaID
	ObserverSet []kramaid.KramaID
	Responses   []*common.ArrayOfBits
	Signature   []byte
	QuorumSizes []int
}

func (ism *ICSSuccessMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ism)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics success message")
	}

	return rawData, nil
}

func (ism *ICSSuccessMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ism, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics success message")
	}

	return nil
}
