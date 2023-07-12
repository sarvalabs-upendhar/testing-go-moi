package args

import "github.com/sarvalabs/go-moi/common"

var LatestTesseractHeight int64 = -1

type TesseractNumberOrHash struct {
	TesseractNumber *int64       `json:"tesseract_number"`
	TesseractHash   *common.Hash `json:"tesseract_hash"`
}

func (t *TesseractNumberOrHash) Number() (int64, error) {
	if t.TesseractNumber == nil {
		return 0, common.ErrEmptyHeight
	}

	if *t.TesseractNumber < LatestTesseractHeight { // if tesseract number less than -1 then it is invalid
		return 0, common.ErrInvalidHeight
	}

	return *t.TesseractNumber, nil
}

func (t *TesseractNumberOrHash) Hash() (common.Hash, bool) {
	if t.TesseractHash == nil {
		return common.NilHash, false
	}

	return *t.TesseractHash, true
}
