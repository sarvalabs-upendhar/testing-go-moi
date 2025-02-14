package message

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type Message interface {
	GetSessionID() identifiers.Identifier
}
