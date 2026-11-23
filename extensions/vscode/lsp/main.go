//go:build js && wasm

// Command lsp is Evie's analysis compiled to WebAssembly for the VS Code
// extension's language server. The server, in JavaScript, speaks the Language
// Server Protocol; it calls the functions this sets on globalThis.evieAnalysis
// and gets JSON back. Files are read through globalThis.evieHost, so the
// editor's unsaved documents take the place of what is on disk.
//
//	GOOS=js GOARCH=wasm go build -o out/evie-lsp.wasm ./extensions/vscode/lsp
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/hxkhan/evie/analysis"
)

// workspaces holds one analysis per workspace root, by the id the host gave it
var workspaces = map[string]*analysis.Workspace{}

func workspace(root string) *analysis.Workspace {
	w, ok := workspaces[root]
	if !ok {
		w = analysis.NewWorkspace(hostFS{root: root, host: js.Global().Get("evieHost")})
		workspaces[root] = w
	}
	return w
}

// reply encodes a result for the host
func reply(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(data)
}

func position(args []js.Value) analysis.Position {
	return analysis.Position{Line: args[2].Int(), Character: args[3].Int()}
}

func main() {
	api := map[string]any{
		// update(root, path): the diagnostics of every file the analysis of path covered
		"update": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Update(args[1].String()))
		}),
		// symbols(root, path): the outline of a file
		"symbols": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Symbols(args[1].String()))
		}),
		// definition(root, path, line, character): a location or null
		"definition": js.FuncOf(func(this js.Value, args []js.Value) any {
			loc, ok := workspace(args[0].String()).Definition(args[1].String(), position(args))
			if !ok {
				return "null"
			}
			return reply(loc)
		}),
		// references(root, path, line, character, includeDeclaration): locations
		"references": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).References(args[1].String(), position(args), args[4].Bool()))
		}),
		// hover(root, path, line, character): Markdown and a range, or null
		"hover": js.FuncOf(func(this js.Value, args []js.Value) any {
			h, ok := workspace(args[0].String()).Hover(args[1].String(), position(args))
			if !ok {
				return "null"
			}
			return reply(h)
		}),
		// complete(root, path, line, character, text): completion items
		"complete": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Complete(args[1].String(), position(args), args[4].String()))
		}),
		// folding(root, path): the folding ranges of a file
		"folding": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Folding(args[1].String()))
		}),
		// highlights(root, path, line, character): the places in the file naming the same thing
		"highlights": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Highlights(args[1].String(), position(args)))
		}),
		// signatureHelp(root, path, line, character, text): the call's signature, or null
		"signatureHelp": js.FuncOf(func(this js.Value, args []js.Value) any {
			h, ok := workspace(args[0].String()).SignatureHelp(args[1].String(), position(args), args[4].String())
			if !ok {
				return "null"
			}
			return reply(h)
		}),
		// prepareRename(root, path, line, character): {range, placeholder} or {error}
		"prepareRename": js.FuncOf(func(this js.Value, args []js.Value) any {
			r, name, err := workspace(args[0].String()).PrepareRename(args[1].String(), position(args))
			if err != nil {
				return reply(map[string]string{"error": err.Error()})
			}
			return reply(map[string]any{"range": r, "placeholder": name})
		}),
		// rename(root, path, line, character, newName): {changes} by path, or {error}
		"rename": js.FuncOf(func(this js.Value, args []js.Value) any {
			edits, err := workspace(args[0].String()).Rename(args[1].String(), position(args), args[4].String())
			if err != nil {
				return reply(map[string]string{"error": err.Error()})
			}
			return reply(map[string]any{"changes": edits})
		}),
		// workspaceSymbols(root, query): the declarations of the workspace that match query
		"workspaceSymbols": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).WorkspaceSymbols(args[1].String()))
		}),
		// links(root, path): the import paths of a file, as links to what they import
		"links": js.FuncOf(func(this js.Value, args []js.Value) any {
			return reply(workspace(args[0].String()).Links(args[1].String()))
		}),
		// runnable(root, path): the program a file starts, or null
		"runnable": js.FuncOf(func(this js.Value, args []js.Value) any {
			r, ok := workspace(args[0].String()).Runnable(args[1].String())
			if !ok {
				return "null"
			}
			return reply(r)
		}),
		// forget(root): drop a root's analysis, as when its folder is closed
		"forget": js.FuncOf(func(this js.Value, args []js.Value) any {
			delete(workspaces, args[0].String())
			return nil
		}),
	}
	js.Global().Set("evieAnalysis", js.ValueOf(api))
	js.Global().Get("evieHost").Call("ready")
	select {}
}
