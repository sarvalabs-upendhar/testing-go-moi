package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/kramaid"
)

type TesseractReq struct {
	Hash             common.Hash
	Number           uint64
	WithInteractions bool
}

type TesseractMessage struct {
	Sender       kramaid.KramaID
	RawTesseract []byte
	Ixns         []byte
	Receipts     []byte
	Delta        map[common.Hash][]byte
}

func (tm *TesseractMessage) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(tm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract message")
	}

	return rawData, nil
}

func (tm *TesseractMessage) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(tm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract message")
	}

	return nil
}

func (tm *TesseractMessage) GetTesseract() (*common.Tesseract, error) {
	ixns := new(common.Interactions)
	receipts := new(common.Receipts)

	ts := new(common.CanonicalTesseract)

	if err := ts.FromBytes(tm.RawTesseract); err != nil {
		return nil, err
	}

	if tm.Ixns != nil && !ts.InteractionHash().IsNil() {
		if err := ixns.FromBytes(tm.Ixns); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return nil, err
			}
		}
	}

	if tm.Receipts != nil && !ts.ReceiptHash().IsNil() {
		if err := receipts.FromBytes(tm.Receipts); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return nil, err
			}
		}
	}

	return common.NewTesseract(
		ts.Header,
		ts.Body,
		*ixns,
		*receipts,
		ts.Seal,
		ts.Sealer,
	), nil
}
