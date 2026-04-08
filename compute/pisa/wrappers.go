package pisa

import (
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/common"
)

type Crypto int

func (Crypto) IsSignature(sig []byte) bool {
	return common.CanUnmarshalSignature(sig)
}

func (Crypto) AuthenticateSignature(data, sig, pub []byte) (bool, error) {
	return crypto.Verify(data, sig, pub)
}
