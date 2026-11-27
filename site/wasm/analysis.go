//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"
	"testing/fstest"

	"github.com/hxkhan/evie/analysis"
)

// The editor's help: the playground runs a second worker of this same build
// for it, so a busy or stopped program never holds it up. Each function takes
// the editor's code, checks it as playground.ev in the playground's file
// system, and returns JSON in the language server's terms.
//
//	evieCheck(code)                   diagnostics of playground.ev
//	evieComplete(code, line, char)    completions at a position
//	evieHover(code, line, char)       hover text at a position, or null
func registerAnalysis() {
	fsys := fstest.MapFS{}
	for name, f := range files {
		fsys[name] = f
	}
	ws := analysis.NewWorkspace(fsys)
	ws.Snippets = true // code without a package header runs as a session does
	checked := "\x00"  // the code the workspace last analyzed; no code is this

	// see makes playground.ev hold code and the workspace know it
	see := func(code string) {
		if code == checked {
			return
		}
		fsys["playground.ev"] = &fstest.MapFile{Data: []byte(code)}
		ws.Update("playground.ev")
		checked = code
	}
	position := func(args []js.Value) analysis.Position {
		return analysis.Position{Line: args[1].Int(), Character: args[2].Int()}
	}

	js.Global().Set("evieCheck", js.FuncOf(func(this js.Value, args []js.Value) any {
		code := args[0].String()
		fsys["playground.ev"] = &fstest.MapFile{Data: []byte(code)}
		diags := ws.Update("playground.ev")["playground.ev"]
		checked = code
		return encode(diags)
	}))
	js.Global().Set("evieComplete", js.FuncOf(func(this js.Value, args []js.Value) any {
		code := args[0].String()
		see(code)
		return encode(ws.Complete("playground.ev", position(args), code))
	}))
	js.Global().Set("evieHover", js.FuncOf(func(this js.Value, args []js.Value) any {
		see(args[0].String())
		h, ok := ws.Hover("playground.ev", position(args))
		if !ok {
			return "null"
		}
		return encode(h)
	}))
}

func encode(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(data)
}
