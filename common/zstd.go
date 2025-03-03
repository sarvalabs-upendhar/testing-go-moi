package common

import "github.com/klauspost/compress/zstd"

type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte, dstSize int) ([]byte, error)
}

type zstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

func NewZstdWriter() (Compressor, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, err
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	return &zstdCompressor{
		encoder: encoder,
		decoder: decoder,
	}, nil
}

func (z *zstdCompressor) Decompress(input []byte, dstSize int) ([]byte, error) {
	dst := make([]byte, 0, dstSize)

	return z.decoder.DecodeAll(input, dst)
}

func (z *zstdCompressor) Compress(src []byte) ([]byte, error) {
	return z.encoder.EncodeAll(src, nil), nil
}
