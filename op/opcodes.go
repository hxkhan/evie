package op

// IDEA: add SETLINE x y where x is a line number and y is the address of the first instruction on that line
// will internally be ordered by y in a linear array

const (
	NULL byte = iota

	EXIT
	ECHO

	INT
	FLOAT
	STR

	TRUE
	FALSE

	ADD
	SUB
	MUL
	DIV

	NEG

	EQ
	LS
	MR

	IF
	ELIF
	ELSE
	END

	LOAD_BUILTIN

	LOAD_LOCAL
	STORE_LOCAL

	LOAD_CAPTURED
	STORE_CAPTURED

	FN_DECL
	LAMBDA
	CALL
	RET

	GO
	AWAIT
	AWAIT_ALL
	AWAIT_ANY

	LOOP

	INC // n++ or n += 1
	DEC // n-- or n -= 1

	STORE_ADD // n += x
	STORE_SUB // n -= x

	// OP_SUB_RIGHT_CONST, OP_LS_RIGHT_CONST, RETURN_IF combined give about 100ms improvement on fib(35)

	ADD_RIGHT_CONST
	SUB_RIGHT_CONST
	LS_RIGHT_CONST
	RETURN_IF

	// total number of instructions
	NUM_OPS
)

// IDEAS
// OP_RETURN_LOAD
// OP_RETURN_LOAD_IF variant of RETURN_IF so combining 3 instructions

// easy way to walk the tree
func Walk(input []byte, handler func(ip int) (size int)) {
	ip := 0
	for ip < len(input) {
		ip += handler(ip)
	}
}

// assumes that the arg is a bytecode and returns its name in all caps
func PublicName(inst byte) string {
	switch inst {
	case NULL:
		return "NULL"
	case EXIT:
		return "EXIT"
	case ECHO:
		return "ECHO"

	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case STR:
		return "STR"

	case TRUE:
		return "TRUE"
	case FALSE:
		return "FALSE"

	case ADD:
		return "ADD"
	case SUB:
		return "SUB"
	case MUL:
		return "MUL"
	case DIV:
		return "DIV"
	case NEG:
		return "NEG"

	case EQ:
		return "EQ"
	case LS:
		return "LS"
	case MR:
		return "MR"

	case IF:
		return "IF"
	case ELIF:
		return "ELIF"
	case ELSE:
		return "ELSE"
	case END:
		return "END"

	case LOAD_BUILTIN:
		return "LOAD_BUILTIN"

	case LOAD_LOCAL:
		return "LOAD_LOCAL"
	case STORE_LOCAL:
		return "STORE_LOCAL"

	case LOAD_CAPTURED:
		return "LOAD_CAPTURED"
	case STORE_CAPTURED:
		return "STORE_CAPTURED"

	case FN_DECL:
		return "FN_DECL"
	case LAMBDA:
		return "LAMBDA"
	case CALL:
		return "CALL"
	case RET:
		return "RET"

	case GO:
		return "GO"
	case AWAIT:
		return "AWAIT"
	case AWAIT_ALL:
		return "AWAIT_ALL"
	case AWAIT_ANY:
		return "AWAIT_ANY"

	case LOOP:
		return "LOOP"

	case INC:
		return "INC"
	case DEC:
		return "DEC"

	case STORE_ADD:
		return "STORE_ADD"
	case STORE_SUB:
		return "STORE_SUB"

	case ADD_RIGHT_CONST:
		return "ADD_RIGHT_CONST"
	case SUB_RIGHT_CONST:
		return "SUB_RIGHT_CONST"
	case LS_RIGHT_CONST:
		return "LS_RIGHT_CONST"

	case RETURN_IF:
		return "RETURN_IF"

	default:
		panic("func PublicName() -> unimplemented instruction!")
	}
}
