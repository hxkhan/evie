package vm

import (
	"log"
)

type logger struct {
	log.Logger
	logCaptures bool
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
