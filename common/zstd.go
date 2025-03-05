package common

import (
	"github.com/hashicorp/go-hclog"
	"github.com/klauspost/compress/zstd"
)

const CompressionThreshold = 20 * 1024 // 20 KB

type Compressor interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte, dstSize int) ([]byte, error)
	Close()
}

type zstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	logger  hclog.Logger
}

func NewZstdCompressor(logger hclog.Logger) (Compressor, error) {
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
		logger:  logger.Named("Zstd-Compressor"),
	}, nil
}

func (z *zstdCompressor) Decompress(input []byte, dstSize int) ([]byte, error) {
	dst := make([]byte, 0, dstSize)

	return z.decoder.DecodeAll(input, dst)
}

func (z *zstdCompressor) Compress(src []byte) ([]byte, error) {
	return z.encoder.EncodeAll(src, nil), nil
}

func (z *zstdCompressor) Close() {
	err := z.encoder.Close()
	if err != nil {
		z.logger.Error("Error closing the zstd encoder instance", "err", err)
	}

	z.decoder.Close()
}
