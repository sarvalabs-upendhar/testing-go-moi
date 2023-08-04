package engineio

import (
	"github.com/sarvalabs/go-moi/crypto"
	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"
)

func ValidateSignature(sig []byte) bool {
	return cryptocommon.CanUnmarshalSignature(sig)
}

func VerifySignature(data, signature, pub []byte) (bool, error) {
	return crypto.Verify(data, signature, pub)
}
