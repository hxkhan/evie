//go:build tools

// The runtimes the cross-language suite builds from this module: two small
// runners of our own for goja and Starlark, and the commands of gopher-lua and
// Yaegi as their authors ship them
package runners

import (
	_ "github.com/traefik/yaegi/cmd/yaegi"
	_ "github.com/yuin/gopher-lua/cmd/glua"
)
