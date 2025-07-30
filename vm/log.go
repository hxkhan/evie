package vm

import (
	"log"

	"github.com/hxkhan/evie/ast"
)

type logger struct {
	log.Logger
	logCache    bool
	logCaptures bool

	cacheHits   int
	cacheMisses int
}

func (lg *logger) cacheHit(node ast.Node) {
	lg.cacheHits++
	if lg.logCache {
		lg.Printf("cache hit for %v", node)
	}
}

func (lg *logger) cacheMiss(node ast.Node) {
	lg.cacheMisses++
	if lg.logCache {
		lg.Printf("cache miss for %v", node)
	}
}

func (lg *logger) printf(format string, v ...any) {
	lg.Printf(format, v...)
}

func (lg *logger) capturef(format string, v ...any) {
	if lg.logCaptures {
		lg.Printf(format, v...)
	}
}

func (lg *logger) escapesf(format string, v ...any) {
	if lg.logCaptures {
		lg.Printf(format, v...)
	}
}
