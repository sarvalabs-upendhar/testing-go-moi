package message

import (
	"github.com/sarvalabs/go-moi/common"
)

type Message interface {
	GetSessionID() common.Address
}
