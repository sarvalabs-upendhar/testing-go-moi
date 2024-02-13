package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractMsg struct {
	RawTesseract []byte
	IxnsHashes   common.Hashes
	Extra        map[string][]byte
}

func (ts *TesseractMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract message")
	}

	return rawData, nil
}

func (ts *TesseractMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ts, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract message")
	}

	return nil
}

func (ts *TesseractMsg) GetTesseract() (*common.Tesseract, error) {
	canonicalTS := new(common.CanonicalTesseract)

	if err := canonicalTS.FromBytes(ts.RawTesseract); err != nil {
		return nil, err
	}

	tesseract := canonicalTS.ToTesseract(nil, nil)

	return tesseract, nil
}
