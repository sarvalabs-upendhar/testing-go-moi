package message

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type HandshakeMSG struct {
	Address []string
	NTQ     float32
	Degree  int32
	Error   string
}

func ConstructHandshakeMSG(
	address []string,
	ntq float32,
	degree int32,
	err string,
) *HandshakeMSG {
	return &HandshakeMSG{
		Address: address,
		NTQ:     ntq,
		Degree:  degree,
		Error:   err,
	}
}

func (hs *HandshakeMSG) Bytes() ([]byte, error) {
	rawBytes, err := polo.Polorize(hs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize handshake message")
	}

	return rawBytes, nil
}

func (hs *HandshakeMSG) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(hs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize handshake message")
	}

	return nil
}
