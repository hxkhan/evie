package vm

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/ds"
	"github.com/hxkhan/evie/internal/fields"
)

// Symbol describes one binding currently defined in a Session's package.
type Symbol struct {
	Name     string
	Value    Value
	IsStatic bool
	IsImport bool
}

// Session supports incremental compilation and execution against a persistent
// package inside an Instance. Top-level variables, functions, classes, and
// imports persist across Eval calls, and failed turns roll back any bindings
// they added or shadowed.
type Session struct {
	vm  *Instance
	pkg *Package
}

// NewSession opens an incremental evaluation session against package name
// (defaulting to "main"). If a compiled package with that name is already
// loaded in the instance, the session attaches to it; otherwise a new package
// is created and registered.
func (vm *Instance) NewSession(name string) *Session {
	if name == "" {
		name = "main"
	}
	vm.rt.acquire()
	defer vm.rt.release()

	pkg := vm.rt.packages[name]
	if pkg == nil || !pkg.compiled {
		pkg = NewPackage(name)
		pkg.id = name
		pkg.compiled = true
		vm.rt.packages[name] = pkg
		vm.rt.byName[name] = pkg
		if !slices.Contains(vm.rt.packageOrder, name) {
			vm.rt.packageOrder = append(vm.rt.packageOrder, name)
		}
	}
	return &Session{vm: vm, pkg: pkg}
}

// Package returns the underlying package that holds this session's symbols.
func (s *Session) Package() *Package {
	return s.pkg
}

// BindImport registers dep under bindName inside the session's package.
func (s *Session) BindImport(bindName, importPath string, dep *Package) {
	s.vm.rt.acquire()
	defer s.vm.rt.release()

	depID := dep.ID()
	if _, exists := s.vm.rt.packages[depID]; !exists {
		s.vm.rt.packages[depID] = dep
		if _, hasShort := s.vm.rt.byName[dep.Name()]; !hasShort {
			s.vm.rt.byName[dep.Name()] = dep
		}
	}

	v := dep.Box()
	s.pkg.globals[fields.Get(bindName)] = pkgGlobal{
		Value:      &v,
		IsPublic:   false,
		IsStatic:   true,
		IsConstant: true,
	}

	label := bindName
	if importPath != "" && importPath != bindName {
		label = fmt.Sprintf("%s as %s", importPath, bindName)
	}
	if !slices.Contains(s.pkg.imports, label) {
		s.pkg.imports = append(s.pkg.imports, label)
	}
}

// Has reports whether name is currently defined in the session (as a user
// binding or an import).
func (s *Session) Has(name string) bool {
	s.vm.rt.acquire()
	defer s.vm.rt.release()
	_, ok := s.pkg.globals[fields.Get(name)]
	return ok
}

// Symbols returns every binding currently defined in the session, sorted by name.
func (s *Session) Symbols() []Symbol {
	s.vm.rt.acquire()
	defer s.vm.rt.release()

	out := make([]Symbol, 0, len(s.pkg.globals))
	for id, g := range s.pkg.globals {
		if g.Value == nil {
			continue
		}
		out = append(out, Symbol{
			Name:     fields.Lookup(id),
			Value:    *g.Value,
			IsStatic: g.IsStatic,
			IsImport: !g.IsPublic,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Eval compiles and executes a parsed snippet in the session.
func (s *Session) Eval(src *diag.Source, node ast.Node) (Value, bool, error) {
	return s.EvalContext(context.Background(), src, node)
}

// Compile compiles a parsed snippet against the session without executing its
// top-level statements (intended for syntax/compile checks and Dump).
func (s *Session) Compile(src *diag.Source, node ast.Node) error {
	_, _, err := s.evalContext(context.Background(), src, node, false)
	return err
}

// EvalContext compiles and executes a parsed snippet inside the session, bound
// to ctx. Top-level variables, functions, and classes persist across turns (and
// may be redeclared on later turns). If compilation or execution fails, any
// declarations added or overwritten by this turn are rolled back.
func (s *Session) EvalContext(ctx context.Context, src *diag.Source, node ast.Node) (Value, bool, error) {
	return s.evalContext(ctx, src, node, true)
}

func (s *Session) evalContext(ctx context.Context, src *diag.Source, node ast.Node, exec bool) (result Value, hasResult bool, err error) {
	vm := s.vm
	pkg := s.pkg

	stop := vm.rt.scope(ctx)
	defer stop.drop()

	vm.rt.acquire()
	defer vm.rt.release()
	defer vm.rt.loadingUnder(stop)()

	prevPkg, prevSrc := vm.cp.pkg, vm.cp.src
	prevClosures := vm.cp.closures.Len()
	vm.cp.pkg = pkg
	vm.cp.src = src

	type savedGlobal struct {
		existed bool
		prev    pkgGlobal
	}
	saved := map[fields.ID]savedGlobal{}
	saveSlot := func(id fields.ID) {
		if _, already := saved[id]; already {
			return
		}
		prev, existed := pkg.globals[id]
		saved[id] = savedGlobal{existed: existed, prev: prev}
	}
	rollback := func() {
		for id, sg := range saved {
			if sg.existed {
				pkg.globals[id] = sg.prev
			} else {
				delete(pkg.globals, id)
			}
		}
	}

	committed := false
	defer func() {
		vm.cp.pkg, vm.cp.src = prevPkg, prevSrc
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				err = ce
			} else if exc, ok := r.(exception); ok {
				err = exc
			} else if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
		if !committed || err != nil {
			rollback()
		}
	}()

	if _, isPkg := node.(ast.Package); isPkg {
		return Value{}, false, vm.compileErrorAt(node, "package headers are not allowed in an incremental session").
			withHint("run statements directly, or import packages via Session.Import")
	}

	var stmts []ast.Node
	if block, ok := node.(ast.Block); ok {
		stmts = block.Code
	} else {
		stmts = []ast.Node{node}
	}
	if len(stmts) == 0 {
		committed = true
		return Value{}, false, nil
	}

	// Track names declared within this single input so duplicates in one turn
	// are still rejected, while redeclaring a name from an earlier turn works.
	declaredThisTurn := map[fields.ID]bool{}
	claim := func(n ast.Node, name string) fields.ID {
		id := fields.Get(name)
		if declaredThisTurn[id] {
			panic(vm.compileErrorAt(n, "'%s' is declared twice in this input", name))
		}
		declaredThisTurn[id] = true
		saveSlot(id)
		return id
	}

	// 1. Hoist top-level function statements and class definitions in this turn.
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case ast.Fn:
			if stmt.UsedAsExpr {
				continue
			}
			if stmt.Name == "" {
				panic(vm.compileErrorAt(stmt, "cannot declare an fn statement with no name"))
			}
			id := claim(stmt, stmt.Name)
			info := vm.newFuncInfo(stmt.Name, stmt.Args, stmt.IsVariadic, false)
			info.def = vm.at(stmt)
			boxed := boxUserFn(userFn{funcInfoStatic: info})
			pkg.globals[id] = pkgGlobal{Value: &boxed, IsPublic: true, IsStatic: true}

		case ast.ClassDefinition:
			if stmt.Name == "" {
				panic(vm.compileErrorAt(stmt, "cannot declare a class with no name"))
			}
			id := claim(stmt, stmt.Name)
			args := make([]fields.ID, len(stmt.Args))
			offsets := make(map[fields.ID]int, len(stmt.Args))
			for i, a := range stmt.Args {
				fid := fields.Get(a)
				args[i] = fid
				offsets[fid] = i
			}
			boxed := boxUserClass(&userClass{
				Name:         stmt.Name,
				def:          vm.at(stmt),
				hasParent:    stmt.Parent != "",
				Fields:       args,
				FieldOffsets: offsets,
				Methods:      make(map[fields.ID]*userFn),
			})
			pkg.globals[id] = pkgGlobal{Value: &boxed, IsPublic: true, IsStatic: true}
		}
	}

	// 2. Declare new top-level variable bindings in this turn (never constant-folded
	// across turns so later redeclarations behave cleanly). Bindings that shadow an
	// existing session name are swapped in after compiling their initializer so
	// `x := x + 1` reads the previous turn's `x`.
	redeclTargets := map[int]*Value{}
	for i, stmt := range stmts {
		decl, ok := stmt.(ast.Decl)
		if !ok {
			continue
		}
		id := claim(decl, decl.Name)
		cell := new(Value)
		if saved[id].existed {
			redeclTargets[i] = cell
		} else {
			pkg.globals[id] = pkgGlobal{Value: cell, IsPublic: true, IsStatic: decl.IsStatic}
		}
	}

	// 3. Compile hoisted functions.
	for _, stmt := range stmts {
		fn, ok := stmt.(ast.Fn)
		if !ok || fn.UsedAsExpr {
			continue
		}
		global := pkg.globals[fields.Get(fn.Name)]
		ufn := (*userFn)(global.pointer)
		vm.compileFunction(ufn.funcInfoStatic, fn.Args, fn.Action, nil)
	}

	// 3b. Compile hoisted class inheritance and methods.
	for _, stmt := range stmts {
		cls, ok := stmt.(ast.ClassDefinition)
		if !ok {
			continue
		}
		global := pkg.globals[fields.Get(cls.Name)]
		uclass := (*userClass)(global.pointer)

		if cls.Parent != "" {
			parentGlobal, exists := pkg.globals[fields.Get(cls.Parent)]
			if !exists {
				panic(vm.compileErrorAt(cls, "cannot find the parent class '%v'", cls.Parent))
			}
			parentClass, ok := parentGlobal.Value.asUserClass()
			if !ok {
				panic(vm.compileErrorAt(cls, "'%v' is %s, not a class", cls.Parent, article(*parentGlobal.Value)))
			}
			for p := parentClass; p != nil; p = p.Parent {
				if p == uclass {
					panic(vm.compileErrorAt(cls, "'%v' and '%v' inherit from each other", cls.Name, cls.Parent))
				}
			}
			uclass.Parent = parentClass
		}

		if block, ok := cls.Action.(ast.Block); ok {
			for _, mStmt := range block.Code {
				mFn, isFn := mStmt.(ast.Fn)
				if !isFn {
					panic(vm.compileErrorAt(mStmt, "only methods ('fn') are allowed inside a class body"))
				}
				if mFn.Name == "" {
					panic(vm.compileErrorAt(mFn, "a class method needs a name"))
				}
				if _, exists := uclass.Methods[fields.Get(mFn.Name)]; exists {
					panic(vm.compileErrorAt(mFn, "method '%s' is declared twice on %s", mFn.Name, cls.Name))
				}
				uclass.Methods[fields.Get(mFn.Name)] = vm.compileClassMethod(uclass, mFn)
			}
		}
	}

	// 4. Compile remaining statements in source order, capturing the last
	// expression value when present.
	initFn, capturesValue := vm.compileSessionInit(pkg, stmts, redeclTargets)
	if !exec {
		if initFn != nil && vm.cp.debugInfo {
			pkg.inits = append(pkg.inits, initFn.funcInfoStatic)
		}
		committed = true
		return Value{}, false, nil
	}
	if initFn == nil {
		committed = true
		return Value{}, false, nil
	}

	fbr := vm.borrowFiber(vm.rt.forLoading())
	defer vm.releaseFiber(fbr)
	fbr.running = true

	val, exc := fbr.runGuarded(func(fbr *fiber) (Value, exception) {
		return initFn.call(fbr, nil)
	})
	if exc != nil {
		return Value{}, false, exc
	}

	hasResult = capturesValue || !val.IsNil()
	if hasResult && !val.IsNil() {
		v := val
		pkg.globals[fields.Get("_")] = pkgGlobal{Value: &v, IsPublic: true, IsStatic: true}
	}
	committed = true
	return val, hasResult, nil
}

func (vm *Instance) compileSessionInit(pkg *Package, stmts []ast.Node, redeclTargets map[int]*Value) (*userFn, bool) {
	var captured map[string]bool
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case ast.Fn:
			if s.UsedAsExpr {
				collectCapturedNames(s, &captured)
			}
		case ast.ClassDefinition:
			continue
		case ast.Decl:
			collectCapturedNames(s.Value, &captured)
		default:
			collectCapturedNames(stmt, &captured)
		}
	}

	info := vm.newFuncInfo("<session>", nil, false, false)
	info.def = site{src: vm.cp.src}
	cl := &closure{freeVars: ds.Set[int]{}, info: info, capturedNames: captured}
	cl.scope.TrackSlots = vm.cp.debugInfo
	vm.cp.closures.Push(cl)
	cl.scope.OpenBlock()

	var steps []instruction
	capturesValue := false
	lastIdx := len(stmts) - 1

	for i, stmt := range stmts {
		switch s := stmt.(type) {
		case ast.ClassDefinition:
			continue
		case ast.Fn:
			if !s.UsedAsExpr {
				continue
			}
			inst := vm.compile(stmt)
			if i == lastIdx {
				capturesValue = true
				steps = append(steps, func(fbr *fiber) (Value, exception) {
					v, exc := inst(fbr)
					if exc != nil {
						return v, exc
					}
					return v, signalReturn
				})
			} else {
				steps = append(steps, inst)
			}
		case ast.Decl:
			value := vm.compile(s.Value)
			var target *Value
			if cell, isRedecl := redeclTargets[i]; isRedecl {
				target = cell
				pkg.globals[fields.Get(s.Name)] = pkgGlobal{Value: cell, IsPublic: true, IsStatic: s.IsStatic}
			} else {
				target = pkg.globals[fields.Get(s.Name)].Value
			}
			steps = append(steps, func(fbr *fiber) (Value, exception) {
				v, exc := value(fbr)
				if exc != nil {
					return v, exc
				}
				*target = v
				return Value{}, nil
			})
		default:
			inst := vm.compile(stmt)
			if i == lastIdx && isSessionExpr(stmt) {
				capturesValue = true
				steps = append(steps, func(fbr *fiber) (Value, exception) {
					v, exc := inst(fbr)
					if exc != nil {
						return v, exc
					}
					return v, signalReturn
				})
			} else {
				steps = append(steps, inst)
			}
		}
	}

	closed := vm.cp.closures.Pop()
	if len(steps) == 0 {
		return nil, false
	}
	info.code = func(fbr *fiber) (Value, exception) {
		for _, step := range steps {
			if v, exc := step(fbr); exc != nil {
				return v, exc
			}
		}
		return Value{}, nil
	}
	vm.closeFunction(info, closed)
	return &userFn{funcInfoStatic: info}, capturesValue
}

func isSessionExpr(node ast.Node) bool {
	switch n := node.(type) {
	case ast.Decl, ast.Assign, ast.MutableBinOp, ast.Return, ast.Throw, ast.Block, ast.Echo,
		ast.Conditional, ast.While, ast.For, ast.Switch, ast.Break, ast.Continue, ast.ClassDefinition:
		return false
	case ast.Fn:
		return n.UsedAsExpr
	}
	return true
}
