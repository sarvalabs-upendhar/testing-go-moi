package message

import (
	"github.com/sarvalabs/go-moi-identifiers"
)

type Message interface {
	GetSessionID() identifiers.Address
}
