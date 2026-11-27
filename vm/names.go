package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/fields"
)

// NameKind says what a name refers to
type NameKind uint8

const (
	NameVariable  NameKind = iota // a var or const inside a function or script, or a loop variable
	NameParameter                 // a function's parameter
	NameGlobal                    // a var or const at the top level of a package
	NameFunction                  // a function declared with a name
	NameClass                     // a class
	NameField                     // a field or method of a class, as in this.x
	NameImport                    // a package bound by imports(...)
	NameMember                    // a member of another package, as in util.f
	NamePrelude                   // a builtin, or a symbol the host makes visible everywhere
)

// String returns the kind in words, such as "parameter"
func (k NameKind) String() string {
	switch k {
	case NameVariable:
		return "variable"
	case NameParameter:
		return "parameter"
	case NameGlobal:
		return "global"
	case NameFunction:
		return "function"
	case NameClass:
		return "class"
	case NameField:
		return "field"
	case NameImport:
		return "import"
	case NameMember:
		return "member"
	case NamePrelude:
		return "prelude"
	}
	return "unknown"
}

// NameEvent is a name the compiler declared or resolved, for tools such as
// editors: go to definition, find references, hover. Options.OnName receives
// them while code compiles. A name can be reported more than once.
type NameEvent struct {
	Name     string
	Kind     NameKind
	Source   *diag.Source // the file the name is written in
	Pos      diag.Pos     // where it is written
	Declares bool         // a declaration, rather than a use

	// for a use, where the name is declared; unknown for names the host or the
	// prelude defines. A declaration declares itself.
	DeclSource *diag.Source
	DeclPos    diag.Pos

	IsStatic bool     // the binding cannot be assigned
	Package  *Package // for an import or a member of another package, that package
}

// nameDecl is where a binding is declared, kept while recording names
type nameDecl struct {
	src    *diag.Source
	pos    diag.Pos
	kind   NameKind
	static bool
}

// declareName reports a declaration and returns it to keep
func (cp *compiler) declareName(name string, kind NameKind, pos diag.Pos, static bool, pkg *Package) nameDecl {
	d := nameDecl{src: cp.src, pos: pos, kind: kind, static: static}
	cp.onName(NameEvent{Name: name, Kind: kind, Source: cp.src, Pos: pos, Declares: true, DeclSource: cp.src, DeclPos: pos, IsStatic: static, Package: pkg})
	return d
}

// declareLocal records a local binding of the innermost function, by its slot
func (cp *compiler) declareLocal(index int, name string, kind NameKind, pos diag.Pos, static bool) {
	if cp.onName == nil || !pos.Known() {
		return
	}
	cl := cp.closures.Last(0)
	if cl.decls == nil {
		cl.decls = map[int]nameDecl{}
	}
	cl.decls[index] = cp.declareName(name, kind, pos, static, nil)
}

// declareGlobal records a binding of a package
func (cp *compiler) declareGlobal(pkg *Package, name string, kind NameKind, pos diag.Pos, static bool, imported *Package) {
	if cp.onName == nil || !pos.Known() {
		return
	}
	if pkg.decls == nil {
		pkg.decls = map[fields.ID]nameDecl{}
	}
	pkg.decls[fields.Get(name)] = cp.declareName(name, kind, pos, static, imported)
}

// declareField records a field or method of a class
func (cp *compiler) declareField(uc *userClass, name string, pos diag.Pos, static bool) {
	if cp.onName == nil || !pos.Known() {
		return
	}
	if uc.decls == nil {
		uc.decls = map[fields.ID]nameDecl{}
	}
	uc.decls[fields.Get(name)] = cp.declareName(name, NameField, pos, static, nil)
}

// used reports a use of a name declared at d
func (cp *compiler) used(name string, pos diag.Pos, d nameDecl, pkg *Package) {
	cp.onName(NameEvent{Name: name, Kind: d.kind, Source: cp.src, Pos: pos, DeclSource: d.src, DeclPos: d.pos, IsStatic: d.static, Package: pkg})
}

// usedLocal reports a use of a slot of the function scroll levels out
func (cp *compiler) usedLocal(scroll, index int, name string, pos diag.Pos, static bool) {
	if cp.onName == nil || !pos.Known() {
		return
	}
	d, ok := cp.closures.Last(scroll).decls[index]
	if !ok {
		d = nameDecl{kind: NameVariable, static: static} // such as a method's receiver
	}
	cp.used(name, pos, d, nil)
}

// usedGlobal reports a use of a binding of pkg, or of the prelude when pkg is nil
func (cp *compiler) usedGlobal(pkg *Package, id fields.ID, name string, pos diag.Pos, g pkgGlobal) {
	if cp.onName == nil || !pos.Known() {
		return
	}
	var d nameDecl
	switch {
	case pkg == nil:
		d = nameDecl{kind: NamePrelude, static: g.IsStatic}
	default:
		var ok bool
		if d, ok = pkg.decls[id]; !ok {
			d = nameDecl{kind: NameGlobal, static: g.IsStatic}
		}
	}
	var imported *Package
	if g.Value != nil {
		imported, _ = g.Value.AsPackage()
	}
	if imported != nil && d.kind == NameGlobal && !d.pos.Known() {
		d.kind = NameImport // bound by the host or a session rather than by imports(...)
	}
	cp.used(name, pos, d, imported)
}

// noteMembers reports the members a node names on a package or on this, as
// in util.f or this.x, when names are recorded; resolving them is spread
// over the compiler's many paths for reads, calls and assignments
func (vm *Instance) noteMembers(node ast.Node) {
	switch node := node.(type) {
	case ast.Call:
		vm.noteMembers(node.Fn)
	case ast.Assign:
		vm.noteMembers(node.Lhs)
	case ast.MutableBinOp:
		vm.noteMembers(node.Lhs)
	case ast.FieldAccess:
		if this, ok := node.Lhs.(ast.Ident); ok && this.Name == "this" {
			vm.noteField(node)
			return
		}
		// a package is a constant, which evaluate folds only with the fast
		// paths on; folding has no effects, so it can be on just for this
		defer func(inline bool) { vm.cp.inline = inline }(vm.cp.inline)
		vm.cp.inline = true
		if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
			if pkg, ok := lhs.AsPackage(); ok {
				vm.notePackageMember(pkg, node)
			}
		}
	}
}

// notePackageMember reports a use of a member of pkg
func (vm *Instance) notePackageMember(pkg *Package, node ast.FieldAccess) {
	cp := &vm.cp
	if cp.onName == nil || !node.RhsPos.Known() {
		return
	}
	id, known := fields.Find(node.Rhs)
	if !known {
		return
	}
	g, exists := pkg.globals[id]
	if !exists || !g.IsPublic {
		return
	}
	d, ok := pkg.decls[id]
	if !ok {
		d = nameDecl{kind: NameMember, static: g.IsStatic}
	}
	d.kind = NameMember
	cp.used(node.Rhs, node.RhsPos, d, pkg)
}

// noteField reports a use of a field or method of the class being compiled
func (vm *Instance) noteField(node ast.FieldAccess) {
	cp := &vm.cp
	if cp.onName == nil || !node.RhsPos.Known() || cp.closures.Len() == 0 {
		return
	}
	id, known := fields.Find(node.Rhs)
	if !known {
		return
	}
	// the class of the method this is in, which may be a function or more out
	var class *userClass
	for depth := range cp.closures.Len() {
		if class = cp.closures.Last(depth).uclass; class != nil {
			break
		}
	}
	for uc := class; uc != nil; uc = uc.Parent {
		if d, ok := uc.decls[id]; ok {
			cp.used(node.Rhs, node.RhsPos, d, nil)
			return
		}
	}
}
