package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractMsg struct {
	RawTesseract     []byte
	UnCompressedSize int // UnCompressedSize will be zero, if raw tesseract is not compressed
	IxnsHashes       common.Hashes
	CommitInfo       *common.CommitInfo
	Extra            map[string][]byte
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

// CompressTesseract compresses the RawTesseract data if it exceeds the compression threshold.
func (m *TesseractMsg) CompressTesseract(compressor common.Compressor) error {
	size := len(m.RawTesseract)

	if size >= common.CompressionThreshold {
		compressedTS, err := compressor.Compress(m.RawTesseract)
		if err != nil {
			return err
		}

		m.UnCompressedSize = len(m.RawTesseract)
		m.RawTesseract = compressedTS
	}

	return nil
}

// DecompressTesseract decompresses the RawTesseract data if it was previously compressed.
func (m *TesseractMsg) DecompressTesseract(compressor common.Compressor) error {
	if m.UnCompressedSize == 0 {
		return nil
	}

	dst, err := compressor.Decompress(m.RawTesseract, m.UnCompressedSize)
	if err != nil {
		return errors.Wrap(err, "failed to decompress tesseract")
	}

	m.RawTesseract = dst

	return nil
}
