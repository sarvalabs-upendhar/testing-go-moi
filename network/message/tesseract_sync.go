package message

import (
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractSyncMsg struct {
	Sender       kramaid.KramaID
	RawTesseract []byte
	Ixns         []byte
	Receipts     []byte
	Delta        map[string][]byte
}

func (tm *TesseractSyncMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(tm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract message")
	}

	return rawData, nil
}

func (tm *TesseractSyncMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(tm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract message")
	}

	return nil
}

func (tm *TesseractSyncMsg) GetTesseract() (*common.Tesseract, error) {
	ixns := new(common.Interactions)
	receipts := new(common.Receipts)

	ts := new(common.CanonicalTesseract)

	if err := ts.FromBytes(tm.RawTesseract); err != nil {
		return nil, err
	}

	if tm.Ixns != nil && !ts.InteractionsHash.IsNil() {
		if err := ixns.FromBytes(tm.Ixns); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return nil, err
			}
		}
	}

	if tm.Receipts != nil && !ts.ReceiptsHash.IsNil() {
		if err := receipts.FromBytes(tm.Receipts); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return nil, err
			}
		}
	}

	return ts.ToTesseract(*ixns, *receipts), nil
}
