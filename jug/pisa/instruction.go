package pisa

import (
	"github.com/pkg/errors"
)

// Instructions represents a set of Instruction objects.
type instructions []instruction

// Instruction represent a single logical instruction
// to execute with an OpCode and the arguments for it.
type instruction struct {
	Op   OpCode
	Args []byte
}

// program is a reader for some Instructions which maintains
// the currently executing instruction with a program counter
type program struct {
	pc   uint64
	code instructions
}

// len returns the number of program instructions
func (program *program) len() uint64 {
	return uint64(len(program.code))
}

// done returns if the program counter is at the last index
func (program *program) done() bool {
	return program.pc >= program.len()
}

// read is a method of program that reads the next Instruction,
// returning it while incrementing the program counter
func (program *program) read() instruction {
	if program.done() {
		return instruction{}
	}

	instruct := program.code[program.pc]
	program.pc++

	return instruct
}

// unread is a method of program that unread the last read Instruction,
// while decrementing the program counter
func (program *program) unread() {
	if program.pc == 0 {
		return
	}

	program.pc--
}

// jump attempts to move the program's counter to a specific point.
// This will fail if the given pc is out of bounds or if the
// instruction at the given point is not a DEST marker.
func (program *program) jump(pc uint64) error {
	if pc > program.len() {
		return errors.New("cannot jump to position: out of bounds")
	}

	if instruct := program.code[pc]; instruct.Op != DEST {
		return errors.New("cannot jump to position: not DEST")
	}

	program.pc = pc

	return nil
}
