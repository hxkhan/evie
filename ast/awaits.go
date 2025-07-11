package ast

import (
	"github.com/hk-32/evie/core"
)

type Await struct {
	What Node
}

func (await Await) compile(cs *Machine) core.Instruction {
	/* pos := cs.emit(op.AWAIT)
	await.What.compile(cs)
	return pos */

	panic("implement")
}

type AwaitAll struct {
	Names []string
}

func (await AwaitAll) compile(cs *Machine) core.Instruction {
	/* pos := cs.emit(op.AWAIT_ALL, byte(len(await.Names)))
	for _, name := range await.Names {
		IdentGet{name}.compile(cs)
	}
	return pos */

	panic("implement")
}

type AwaitAny struct {
	Names []string
}

func (await AwaitAny) compile(cs *Machine) core.Instruction {
	/* pos := cs.emit(op.AWAIT_ANY, byte(len(await.Names)))
	for _, name := range await.Names {
		IdentGet{name}.compile(cs)
	}
	return pos */

	panic("implement")
}
