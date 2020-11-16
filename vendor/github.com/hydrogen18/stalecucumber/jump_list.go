package stalecucumber

import "errors"

type opcodeFunc func(*PickleMachine) error

var ErrOpcodeInvalid = errors.New("Opcode is invalid")

func (pm *PickleMachine) Opcode_Invalid() error {
	return ErrOpcodeInvalid
}

type opcodeJumpList [256]opcodeFunc

func buildEmptyJumpList() opcodeJumpList {
	jl := opcodeJumpList{}

	for i := range jl {
		jl[i] = (*PickleMachine).Opcode_Invalid
	}
	return jl
}
