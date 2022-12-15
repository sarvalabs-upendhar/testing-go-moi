package pisa

import (
	"bytes"
)

// operand0 reads 0 bytes from the reader as operands.
func operand0(_ *bytes.Reader) ([]byte, bool) {
	return nil, true
}

// operand1 reads 1 bytes from the reader as operands.
func operand1(reader *bytes.Reader) ([]byte, bool) {
	operand, err := reader.ReadByte()
	if err != nil {
		return nil, false
	}

	return []byte{operand}, true
}

// operand2 reads 2 bytes from the reader as operands.
func operand2(reader *bytes.Reader) ([]byte, bool) {
	operands := make([]byte, 2)
	read, err := reader.Read(operands)

	if read != 2 || err != nil {
		return nil, false
	}

	return operands, true
}

// operand3 reads 3 bytes from the reader as operands.
func operand3(reader *bytes.Reader) ([]byte, bool) {
	operands := make([]byte, 3)
	read, err := reader.Read(operands)

	if read != 3 || err != nil {
		return nil, false
	}

	return operands, true
}

// operandLDPTR reads bytes from the reader for the LDPTR opcode.
// LDPTR takes a variable number of operands. The first operand specifies the size of the pointer to read.
// The second operand specifies the register to load the pointer into, followed by n bytes for the pointer.
func operandLDPTR(reader *bytes.Reader) ([]byte, bool) {
	var (
		err     error
		regID   byte
		ptrSize byte
	)

	if ptrSize, err = reader.ReadByte(); err != nil {
		return nil, false
	}

	if regID, err = reader.ReadByte(); err != nil {
		return nil, false
	}

	if ptrSize == 0 {
		return []byte{regID, 1, 0}, true
	}

	address := make([]byte, ptrSize)
	if n, err := reader.Read(address); n != int(ptrSize) || err != nil {
		return nil, false
	}

	operands := make([]byte, 0, ptrSize+2)
	operands = append(operands, ptrSize, regID)
	operands = append(operands, address...)

	return operands, true
}
