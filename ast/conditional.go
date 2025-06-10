package ast

import "github.com/hk-32/evie/op"

type Conditional struct {
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

func (cond Conditional) compile(cs *CompilerState) int {
	// OP_RETURN_IF optimisation
	if cond.Otherwise == nil && cs.optimise {
		if ret, isReturn := cond.Action.(Return); isReturn {
			pos := cs.emit(op.RETURN_IF, 0)
			cond.Condition.compile(cs)
			ret.Value.compile(cs)
			offset := cs.len() - pos
			cs.emit(op.END)
			cs.addU8OffsetFor(pos, byte(offset))
			return pos
		}
	}

	pos := cs.emit(op.IF, 0, 0)

	cond.Condition.compile(cs)
	cs.scopeOpenBlock()
	cond.Action.compile(cs)
	offset := cs.len() - pos

	if cond.Otherwise != nil {
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			o.compileAsELIF(cs)
		} else {
			// means it's an else
			posELSE := cs.emit(op.ELSE, 0, 0)
			cs.scopeReuseBlock()
			cond.Otherwise.compile(cs)
			offset := cs.len() - posELSE
			cs.addU16OffsetFor(posELSE, uint16(offset))
		}
	}

	cs.scopeCloseBlock()
	cs.emit(op.END)
	cs.addU16OffsetFor(pos, uint16(offset))

	return pos
}

func (cond Conditional) compileAsELIF(cs *CompilerState) int {
	pos := cs.emit(op.ELIF, 0, 0)

	cond.Condition.compile(cs)
	cs.scopeReuseBlock()
	cond.Action.compile(cs)
	offset := cs.len() - pos

	if cond.Otherwise != nil {
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			o.compileAsELIF(cs)
		} else {
			// means it's an else
			posELSE := cs.emit(op.ELSE, 0, 0)
			cs.scopeReuseBlock()
			cond.Otherwise.compile(cs)
			offset := cs.len() - posELSE
			cs.addU16OffsetFor(posELSE, uint16(offset))
		}
	}
	cs.addU16OffsetFor(pos, uint16(offset))

	return pos
}
