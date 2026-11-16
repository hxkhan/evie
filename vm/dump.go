package vm

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

// Dump writes a human-readable summary of every compiled package, global,
// class, function, local stack slot, escape analysis decision, and closure
// capture table in the VM. Enable Options.DebugInfo before compiling to also
// include local slot names, local classes, nested closures, and resolved
// capture names.
func (vm *Instance) Dump(w io.Writer) {
	fastPaths := "on"
	if !vm.cp.inline {
		fastPaths = "off"
	}
	fmt.Fprintf(w, "evie dump (fast-paths: %s)\n", fastPaths)

	for _, name := range vm.rt.packageOrder {
		pkg := vm.rt.packages[name]
		if pkg == nil || !pkg.compiled {
			continue
		}
		fmt.Fprintln(w)
		dumpPackage(w, pkg)
	}

	if vm.rt.scriptInfo != nil {
		fmt.Fprintln(w)
		dumpFn(w, vm.rt.scriptInfo, "", nil)
	}
}

func dumpPackage(w io.Writer, pkg *Package) {
	if pkg.origin != "" {
		fmt.Fprintf(w, "package %s (%s)\n", pkg.name, pkg.origin)
	} else {
		fmt.Fprintf(w, "package %s\n", pkg.name)
	}

	var imports []string
	type globalEntry struct {
		name string
		g    pkgGlobal
	}
	var globals []globalEntry
	var classes []*userClass
	var funcs []*funcInfoStatic

	for fid, g := range pkg.globals {
		if g.Value == nil {
			continue
		}
		name := fields.Lookup(fid)
		if !g.IsPublic {
			if _, isPkg := g.Value.AsPackage(); isPkg {
				imports = append(imports, name)
			}
			continue
		}
		if uc, ok := g.Value.asUserClass(); ok {
			classes = append(classes, uc)
			continue
		}
		if ufn, ok := g.Value.asUserFn(); ok {
			funcs = append(funcs, ufn.funcInfoStatic)
			continue
		}
		globals = append(globals, globalEntry{name: name, g: g})
	}

	sort.Strings(imports)
	if len(imports) > 0 {
		fmt.Fprintf(w, "  imports: %s\n", strings.Join(imports, ", "))
	}

	sort.Slice(globals, func(i, j int) bool { return globals[i].name < globals[j].name })
	if len(globals) > 0 {
		fmt.Fprintln(w, "  globals:")
		for _, entry := range globals {
			kind := "var"
			if entry.g.IsStatic {
				kind = "const"
			}
			if entry.g.IsConstant {
				fmt.Fprintf(w, "    %s %s = %s [folded]\n", kind, entry.name, formatConst(*entry.g.Value))
			} else {
				fmt.Fprintf(w, "    %s %s [runtime]\n", kind, entry.name)
			}
		}
	}

	sort.Slice(classes, func(i, j int) bool {
		if cmp := compareSite(classes[i].def, classes[j].def); cmp != 0 {
			return cmp < 0
		}
		return classes[i].Name < classes[j].Name
	})
	for _, uc := range classes {
		fmt.Fprintln(w)
		dumpClass(w, uc, "  ", nil)
	}

	sort.Slice(funcs, func(i, j int) bool {
		if cmp := compareSite(funcs[i].def, funcs[j].def); cmp != 0 {
			return cmp < 0
		}
		return funcs[i].name < funcs[j].name
	})
	for _, fn := range funcs {
		fmt.Fprintln(w)
		dumpFn(w, fn, "  ", nil)
	}

	for _, initFn := range pkg.inits {
		fmt.Fprintln(w)
		dumpFn(w, initFn, "  ", nil)
	}
}

func dumpClass(w io.Writer, uc *userClass, indent string, ancestors []*funcInfoStatic) {
	header := "class " + uc.Name
	if uc.hasParent {
		if uc.Parent != nil {
			header += " : " + uc.Parent.Name
		} else {
			header += " : <dynamic>"
		}
	}
	if loc := siteLocation(uc.def); loc != "" {
		header += " (" + loc + ")"
	}
	fmt.Fprintf(w, "%s%s\n", indent, header)

	if len(uc.Fields) == 0 {
		fmt.Fprintf(w, "%s  fields: (none)\n", indent)
	} else {
		parts := make([]string, len(uc.Fields))
		for i, fid := range uc.Fields {
			parts[i] = fmt.Sprintf("[%d] %s", i, fields.Lookup(fid))
		}
		fmt.Fprintf(w, "%s  fields: %s\n", indent, strings.Join(parts, ", "))
	}

	if len(uc.Methods) > 0 {
		methods := make([]*funcInfoStatic, 0, len(uc.Methods))
		for _, m := range uc.Methods {
			methods = append(methods, m.funcInfoStatic)
		}
		sort.Slice(methods, func(i, j int) bool {
			if cmp := compareSite(methods[i].def, methods[j].def); cmp != 0 {
				return cmp < 0
			}
			return methods[i].name < methods[j].name
		})
		fmt.Fprintf(w, "%s  methods:\n", indent)
		for _, m := range methods {
			dumpFn(w, m, indent+"    ", ancestors)
		}
	}
}

func dumpFn(w io.Writer, fn *funcInfoStatic, indent string, ancestors []*funcInfoStatic) {
	name := fn.name
	if name == "" {
		name = "<anon>"
	}
	params := make([]string, len(fn.args))
	for i, a := range fn.args {
		if fn.isVariadic && i == len(fn.args)-1 {
			params[i] = "..." + a
		} else {
			params[i] = a
		}
	}
	header := fmt.Sprintf("fn %s(%s)", name, strings.Join(params, ", "))
	if loc := siteLocation(fn.def); loc != "" {
		header += " (" + loc + ")"
	}
	fmt.Fprintf(w, "%s%s\n", indent, header)

	if len(fn.captures) > 0 {
		fmt.Fprintf(w, "%s  captures: %d\n", indent, len(fn.captures))
		for i, cap := range fn.captures {
			if resolved := resolveCaptureName(cap, ancestors); resolved != "" {
				fmt.Fprintf(w, "%s    [%d] <- %s '%s'\n", indent, i, cap, resolved)
			} else {
				fmt.Fprintf(w, "%s    [%d] <- %s\n", indent, i, cap)
			}
		}
	}

	escaped := len(fn.locals) - fn.recyclable
	fmt.Fprintf(w, "%s  locals: %d (%d escape, %d recyclable)\n", indent, len(fn.locals), escaped, fn.recyclable)
	for i, esc := range fn.locals {
		var tags []string
		slotName := ""
		if i < len(fn.args) {
			slotName = fn.args[i]
			tags = append(tags, "param")
		} else if fn.debug != nil && i < len(fn.debug.slots) {
			slotName = fn.debug.slots[i].Name
			if fn.debug.slots[i].IsStatic {
				tags = append(tags, "const")
			} else {
				tags = append(tags, "var")
			}
		}
		if esc {
			tags = append(tags, "escapes")
		}
		if slotName == "" {
			slotName = "<local>"
		}
		if len(tags) > 0 {
			fmt.Fprintf(w, "%s    [%d] %s (%s)\n", indent, i, slotName, strings.Join(tags, ", "))
		} else {
			fmt.Fprintf(w, "%s    [%d] %s\n", indent, i, slotName)
		}
	}

	if fn.debug != nil && len(fn.debug.classes) > 0 {
		nextAncestors := append(ancestors[:len(ancestors):len(ancestors)], fn)
		fmt.Fprintf(w, "%s  classes:\n", indent)
		for _, uc := range fn.debug.classes {
			dumpClass(w, uc, indent+"    ", nextAncestors)
		}
	}

	if fn.debug != nil && len(fn.debug.children) > 0 {
		nextAncestors := append(ancestors[:len(ancestors):len(ancestors)], fn)
		fmt.Fprintf(w, "%s  closures:\n", indent)
		for _, child := range fn.debug.children {
			dumpFn(w, child, indent+"    ", nextAncestors)
		}
	}
}

func resolveCaptureName(c capture, ancestors []*funcInfoStatic) string {
	for i := len(ancestors) - 1; i >= 0; i-- {
		parent := ancestors[i]
		if c.isLocal {
			if c.index < len(parent.args) {
				return parent.args[c.index]
			}
			if parent.debug != nil && c.index < len(parent.debug.slots) {
				return parent.debug.slots[c.index].Name
			}
			return ""
		}
		if c.index >= len(parent.captures) {
			return ""
		}
		c = parent.captures[c.index]
	}
	return ""
}

func compareSite(a, b site) int {
	aName, bName := "", ""
	if a.src != nil {
		aName = a.src.Name
	}
	if b.src != nil {
		bName = b.src.Name
	}
	if aName != bName {
		return strings.Compare(aName, bName)
	}
	if a.pos.Line != b.pos.Line {
		return a.pos.Line - b.pos.Line
	}
	return a.pos.Column - b.pos.Column
}

func siteLocation(s site) string {
	if s.pos.Known() {
		return fmt.Sprintf("line %d", s.pos.Line)
	}
	if s.src != nil && s.src.Name != "" {
		return s.src.Name
	}
	return ""
}

func formatConst(v Value) string {
	if s, ok := v.AsString(); ok {
		return fmt.Sprintf("%q", s)
	}
	return v.String()
}
