package safety

import (
	"context"
	"sync"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-polo"
)

type vault interface {
	Sign(data []byte, sigType mudracommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
	KramaID() identifiers.KramaID
}

type store interface {
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	GetSafetyData(id identifiers.Identifier) ([]byte, error)
	GetCommitInfo(tsHash common.Hash) ([]byte, error)
	SetSafetyData(id identifiers.Identifier, data []byte) error
	SetConsensusProposalInfo(tsHash common.Hash, data []byte) error
	GetConsensusProposalInfo(tsHash common.Hash) ([]byte, error)
	DeleteConsensusProposalInfo(tsHash common.Hash) error
	GetAllConsensusProposalInfo(ctx context.Context) ([][]byte, error)
	DeleteSafetyData(id identifiers.Identifier) error
}

type ProposalInfo struct {
	TS         *common.Tesseract
	Ixns       *common.Interactions
	Receipts   common.Receipts
	CommitInfo *common.CommitInfo
}

func NewProposalInfo(ts *common.Tesseract) *ProposalInfo {
	ixns := ts.Interactions()

	pi := new(ProposalInfo)
	pi.TS = ts
	pi.Ixns = &ixns
	pi.Receipts = ts.Receipts()
	pi.CommitInfo = ts.CommitInfo()

	return pi
}

func (pi *ProposalInfo) Tesseract() *common.Tesseract {
	pi.TS.WithIxnAndReceipts(*pi.Ixns, pi.Receipts, pi.CommitInfo)

	return pi.TS
}

func (pi *ProposalInfo) Bytes() ([]byte, error) {
	return polo.Polorize(pi)
}

func (pi *ProposalInfo) FromBytes(raw []byte) error {
	return polo.Depolorize(pi, raw)
}

type ConsensusSafety struct {
	mtx   sync.Mutex
	db    store
	vault vault
}

func NewConsensusSafety(db store, v vault) *ConsensusSafety {
	return &ConsensusSafety{
		mtx:   sync.Mutex{},
		db:    db,
		vault: v,
	}
}

func (s *ConsensusSafety) GetLatestSafetyInfo(id identifiers.Identifier) (*ktypes.SafetyData, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	metaInfo, err := s.db.GetAccountMetaInfo(id)
	if err != nil {
		return nil, err
	}

	info, err := s.getSafetyData(metaInfo.ID)
	if info != nil {
		return info, nil
	}

	if errors.Is(err, common.ErrKeyNotFound) {
		commitInfoRaw, err := s.db.GetCommitInfo(metaInfo.TesseractHash)
		if err != nil {
			return nil, err
		}

		commitInfo := new(common.CommitInfo)
		if err = commitInfo.FromBytes(commitInfoRaw); err != nil {
			return nil, err
		}

		return &ktypes.SafetyData{
			Qc:       []*common.Qc{commitInfo.QC},
			TSHashes: []common.Hash{metaInfo.TesseractHash},
		}, nil
	}

	return nil, err
}

func (s *ConsensusSafety) UpdateSafetyInfo(p *ktypes.Proposal, qc *common.Qc) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for id := range p.Heights() {
		safetyInfo, err := s.getSafetyData(id)
		if err != nil {
			safetyInfo = &ktypes.SafetyData{
				Qc:       []*common.Qc{qc},
				TSHashes: []common.Hash{qc.TSHash},
			}
		} else {
			safetyInfo.UpdateQc(qc)
		}

		if err = s.setSafetyData(id, safetyInfo); err != nil {
			return err
		}
	}

	if qc.Type == common.PREVOTE {
		rawInfo, err := NewProposalInfo(p.Tesseract).Bytes()
		if err != nil {
			return err
		}

		if err = s.db.SetConsensusProposalInfo(p.Tesseract.Hash(), rawInfo); err != nil {
			return err
		}
	}

	if qc.Type == common.PRECOMMIT {
		if err := s.db.DeleteConsensusProposalInfo(p.Tesseract.Hash()); err != nil {
			return err
		}
	}

	return nil
}

func (s *ConsensusSafety) GetFailedViewTS() ([]*common.Tesseract, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	tesseracts := make([]*common.Tesseract, 0)

	rawInfos, err := s.db.GetAllConsensusProposalInfo(context.Background())
	if err != nil {
		return nil, err
	}

	for _, rawInfo := range rawInfos {
		info := new(ProposalInfo)
		if err = info.FromBytes(rawInfo); err != nil {
			return nil, err
		}

		tesseracts = append(tesseracts, info.Tesseract())
	}

	return tesseracts, nil
}

func (s *ConsensusSafety) GetTesseract(tsHash common.Hash) (*common.Tesseract, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	rawInfo, err := s.db.GetConsensusProposalInfo(tsHash)
	if err != nil {
		return nil, err
	}

	info := new(ProposalInfo)
	if err = info.FromBytes(rawInfo); err != nil {
		return nil, err
	}

	return info.Tesseract(), nil
}

func (s *ConsensusSafety) getSafetyData(id identifiers.Identifier) (*ktypes.SafetyData, error) {
	raw, err := s.db.GetSafetyData(id)
	if err != nil {
		return nil, err
	}

	safetyData := new(ktypes.SafetyData)

	if err = safetyData.FromBytes(raw); err != nil {
		return nil, err
	}

	return safetyData, nil
}

func (s *ConsensusSafety) setSafetyData(id identifiers.Identifier, data *ktypes.SafetyData) error {
	raw, err := data.Bytes()
	if err != nil {
		return err
	}

	return s.db.SetSafetyData(id, raw)
}

func (s *ConsensusSafety) DeleteSafetyData(id identifiers.Identifier) error {
	return s.db.DeleteSafetyData(id)
}
