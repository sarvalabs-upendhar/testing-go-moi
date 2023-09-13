package crypto

import (
	"github.com/sarvalabs/go-moi/crypto/common"
)

type Cryptographer int

func (Cryptographer) ValidateSignature(sig []byte) bool {
	return common.CanUnmarshalSignature(sig)
}

func (Cryptographer) VerifySignature(data, sig, pub []byte) (bool, error) {
	return Verify(data, sig, pub)
}
