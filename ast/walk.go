package ast

// Walk traverses an AST in depth-first order. It calls visit for each non-nil
// node; if visit returns false, Walk does not descend into that node's children.
func Walk(node Node, visit func(Node) bool) {
	if node == nil || !visit(node) {
		return
	}

	switch n := node.(type) {
	case Package:
		for _, stmt := range n.Code {
			Walk(stmt, visit)
		}
	case Block:
		for _, stmt := range n.Code {
			Walk(stmt, visit)
		}
	case Decl:
		Walk(n.Value, visit)
	case Assign:
		Walk(n.Lhs, visit)
		Walk(n.Value, visit)
	case BinOp:
		Walk(n.Lhs, visit)
		Walk(n.Rhs, visit)
	case MutableBinOp:
		Walk(n.Lhs, visit)
		Walk(n.Rhs, visit)
	case Plus:
		Walk(n.Value, visit)
	case Neg:
		Walk(n.Value, visit)
	case Not:
		Walk(n.Value, visit)
	case Exists:
		Walk(n.Value, visit)
	case Echo:
		Walk(n.Value, visit)
	case Return:
		Walk(n.Value, visit)
	case Throw:
		Walk(n.Value, visit)
	case IsInstanceOf:
		Walk(n.Lhs, visit)
		Walk(n.Rhs, visit)
	case FieldAccess:
		Walk(n.Lhs, visit)
	case Subscript:
		Walk(n.Lhs, visit)
		Walk(n.Key, visit)
	case Slice:
		Walk(n.Lhs, visit)
		Walk(n.Start, visit)
		Walk(n.End, visit)
	case ArrayLiteral:
		for _, el := range n.Elements {
			Walk(el, visit)
		}
	case Object:
		for _, f := range n.Fields {
			Walk(f.Value, visit)
		}
	case StringTemplate:
		for _, arg := range n.Args {
			Walk(arg, visit)
		}
	case Call:
		Walk(n.Fn, visit)
		for _, arg := range n.Args {
			Walk(arg, visit)
		}
	case Fn:
		Walk(n.Action, visit)
	case ClassDefinition:
		Walk(n.Action, visit)
	case Conditional:
		Walk(n.Condition, visit)
		Walk(n.Action, visit)
		Walk(n.Otherwise, visit)
	case While:
		Walk(n.Condition, visit)
		Walk(n.Action, visit)
	case For:
		Walk(n.Collection, visit)
		Walk(n.Action, visit)
	case Switch:
		Walk(n.Value, visit)
		for _, c := range n.Cases {
			for _, v := range c.Values {
				Walk(v, visit)
			}
			Walk(c.Action, visit)
		}
		Walk(n.Default, visit)
	case Catch:
		Walk(n.Action, visit)
	case Go:
		Walk(n.Fn, visit)
	case Await:
		Walk(n.Task, visit)
	case AwaitAll:
		for _, t := range n.Tasks {
			Walk(t, visit)
		}
	case AwaitAny:
		for _, t := range n.Tasks {
			Walk(t, visit)
		}
	case AwaitRace:
		for _, t := range n.Tasks {
			Walk(t, visit)
		}
	case AwaitTimeout:
		Walk(n.Task, visit)
		Walk(n.Timeout, visit)
	}
}
