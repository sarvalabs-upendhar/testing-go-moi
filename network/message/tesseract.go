package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractMsg struct {
	RawTesseract []byte
	IxnsHashes   common.Hashes
	CommitInfo   *common.CommitInfo
	Extra        map[string][]byte
}

func (m *TesseractMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract message")
	}

	return rawData, nil
}

func (m *TesseractMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(m, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract message")
	}

	return nil
}

func (m *TesseractMsg) GetTesseract() (*common.Tesseract, error) {
	ts := new(common.Tesseract)

	if err := ts.FromBytes(m.RawTesseract); err != nil {
		return nil, err
	}

	ts.WithIxnAndReceipts(common.Interactions{}, nil, m.CommitInfo)

	return ts, nil
}
