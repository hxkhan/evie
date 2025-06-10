package ast

import (
	"github.com/hk-32/evie/op"
)

type Await struct {
	What Node
}

func (await Await) compile(cs *CompilerState) int {
	pos := cs.emit(op.AWAIT)
	await.What.compile(cs)
	return pos
}

type AwaitAll struct {
	Names []string
}

func (await AwaitAll) compile(cs *CompilerState) int {
	pos := cs.emit(op.AWAIT_ALL, byte(len(await.Names)))
	for _, name := range await.Names {
		IdentGet{name}.compile(cs)
	}
	return pos
}

type AwaitAny struct {
	Names []string
}

func (await AwaitAny) compile(cs *CompilerState) int {
	pos := cs.emit(op.AWAIT_ANY, byte(len(await.Names)))
	for _, name := range await.Names {
		IdentGet{name}.compile(cs)
	}
	return pos
}
