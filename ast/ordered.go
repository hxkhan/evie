package ast

// OrderedCode returns the code of this package ordered by rules:
//
// 1. Put all variable declarations at the top.
//
// 2. For any variable that depends on a function, move that function above the variable.
//
// This preserves the original order of variables among themselves and
// functions among themselves, except where rule 2 forces a change.
func (pkg *Package) OrderedCode() []Node {
	// 1. build maps
	funcs := map[string]int{}
	usedFuncs := map[string]bool{}

	for i, node := range pkg.Code {
		if fn, ok := node.(Fn); ok {
			funcs[fn.Name] = i
		}
	}

	// 2. separate vars and funcs
	var result []Node
	for _, node := range pkg.Code {
		if iDec, ok := node.(Decl); ok {
			// check dependencies
			deps := dependencies(iDec.Value)
			for _, dep := range deps {
				if fnPos, exists := funcs[dep]; exists && !usedFuncs[dep] {
					// insert the function before this variable
					result = append(result, pkg.Code[fnPos])
					usedFuncs[dep] = true
				}
			}

			// add the variable
			result = append(result, node)
		}
	}

	// 3. add remaining functions not yet used
	for _, node := range pkg.Code {
		if fn, ok := node.(Fn); ok && !usedFuncs[fn.Name] {
			result = append(result, node)
		}
	}

	return result
}

// dependencies recursively finds all dependency identifiers
func dependencies(node Node) []string {
	var deps []string

	switch e := node.(type) {
	case Ident:
		// simple identifier like "x" or "someFunc"
		deps = append(deps, e.Name)

	case Call:
		// someFunc(a, b) -> depends on someFunc, a, b
		deps = append(deps, dependencies(e.Fn)...)
		for _, arg := range e.Args {
			deps = append(deps, dependencies(arg)...)
		}

	case BinOp:
		// a + b -> depends on a, b
		deps = append(deps, dependencies(e.Lhs)...)
		deps = append(deps, dependencies(e.Rhs)...)
	}

	return deps
}
