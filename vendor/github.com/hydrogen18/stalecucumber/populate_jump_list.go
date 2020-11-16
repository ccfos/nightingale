package stalecucumber

func populateJumpList(jl *opcodeJumpList) {
	jl[OPCODE_INT] = (*PickleMachine).opcode_INT
	jl[OPCODE_LONG] = (*PickleMachine).opcode_LONG
	jl[OPCODE_STRING] = (*PickleMachine).opcode_STRING
	jl[OPCODE_NONE] = (*PickleMachine).opcode_NONE
	jl[OPCODE_UNICODE] = (*PickleMachine).opcode_UNICODE
	jl[OPCODE_FLOAT] = (*PickleMachine).opcode_FLOAT
	jl[OPCODE_APPEND] = (*PickleMachine).opcode_APPEND
	jl[OPCODE_LIST] = (*PickleMachine).opcode_LIST
	jl[OPCODE_TUPLE] = (*PickleMachine).opcode_TUPLE
	jl[OPCODE_DICT] = (*PickleMachine).opcode_DICT
	jl[OPCODE_SETITEM] = (*PickleMachine).opcode_SETITEM
	jl[OPCODE_POP] = (*PickleMachine).opcode_POP
	jl[OPCODE_DUP] = (*PickleMachine).opcode_DUP
	jl[OPCODE_MARK] = (*PickleMachine).opcode_MARK
	jl[OPCODE_GET] = (*PickleMachine).opcode_GET
	jl[OPCODE_PUT] = (*PickleMachine).opcode_PUT
	jl[OPCODE_GLOBAL] = (*PickleMachine).opcode_GLOBAL
	jl[OPCODE_REDUCE] = (*PickleMachine).opcode_REDUCE
	jl[OPCODE_BUILD] = (*PickleMachine).opcode_BUILD
	jl[OPCODE_INST] = (*PickleMachine).opcode_INST
	jl[OPCODE_STOP] = (*PickleMachine).opcode_STOP
	jl[OPCODE_PERSID] = (*PickleMachine).opcode_PERSID
	jl[OPCODE_BININT] = (*PickleMachine).opcode_BININT
	jl[OPCODE_BININT1] = (*PickleMachine).opcode_BININT1
	jl[OPCODE_BININT2] = (*PickleMachine).opcode_BININT2
	jl[OPCODE_BINSTRING] = (*PickleMachine).opcode_BINSTRING
	jl[OPCODE_SHORT_BINSTRING] = (*PickleMachine).opcode_SHORT_BINSTRING
	jl[OPCODE_BINUNICODE] = (*PickleMachine).opcode_BINUNICODE
	jl[OPCODE_BINFLOAT] = (*PickleMachine).opcode_BINFLOAT
	jl[OPCODE_EMPTY_LIST] = (*PickleMachine).opcode_EMPTY_LIST
	jl[OPCODE_APPENDS] = (*PickleMachine).opcode_APPENDS
	jl[OPCODE_EMPTY_TUPLE] = (*PickleMachine).opcode_EMPTY_TUPLE
	jl[OPCODE_EMPTY_DICT] = (*PickleMachine).opcode_EMPTY_DICT
	jl[OPCODE_SETITEMS] = (*PickleMachine).opcode_SETITEMS
	jl[OPCODE_POP_MARK] = (*PickleMachine).opcode_POP_MARK
	jl[OPCODE_BINGET] = (*PickleMachine).opcode_BINGET
	jl[OPCODE_LONG_BINGET] = (*PickleMachine).opcode_LONG_BINGET
	jl[OPCODE_BINPUT] = (*PickleMachine).opcode_BINPUT
	jl[OPCODE_LONG_BINPUT] = (*PickleMachine).opcode_LONG_BINPUT
	jl[OPCODE_OBJ] = (*PickleMachine).opcode_OBJ
	jl[OPCODE_BINPERSID] = (*PickleMachine).opcode_BINPERSID
	jl[OPCODE_LONG1] = (*PickleMachine).opcode_LONG1
	jl[OPCODE_LONG4] = (*PickleMachine).opcode_LONG4
	jl[OPCODE_NEWTRUE] = (*PickleMachine).opcode_NEWTRUE
	jl[OPCODE_NEWFALSE] = (*PickleMachine).opcode_NEWFALSE
	jl[OPCODE_TUPLE1] = (*PickleMachine).opcode_TUPLE1
	jl[OPCODE_TUPLE2] = (*PickleMachine).opcode_TUPLE2
	jl[OPCODE_TUPLE3] = (*PickleMachine).opcode_TUPLE3
	jl[OPCODE_EXT1] = (*PickleMachine).opcode_EXT1
	jl[OPCODE_EXT2] = (*PickleMachine).opcode_EXT2
	jl[OPCODE_EXT4] = (*PickleMachine).opcode_EXT4
	jl[OPCODE_NEWOBJ] = (*PickleMachine).opcode_NEWOBJ
	jl[OPCODE_PROTO] = (*PickleMachine).opcode_PROTO
}

