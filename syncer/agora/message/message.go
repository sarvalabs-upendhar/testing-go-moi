package message

import (
	"github.com/sarvalabs/moichain/common"
)

type Message interface {
	GetSessionID() common.Address
}
