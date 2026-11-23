package ast

// Inspect walks the syntax under node depth first, calling visit for node and
// then for each of its children; when visit returns false the children of
// that node are skipped. A nil node is not visited.
func Inspect(node Node, visit func(Node) bool) {
	if node == nil || !visit(node) {
		return
	}
	all := func(nodes []Node) {
		for _, n := range nodes {
			Inspect(n, visit)
		}
	}
	switch n := node.(type) {
	case Package:
		all(n.Code)
	case Block:
		all(n.Code)
	case Fn:
		Inspect(n.Action, visit)
	case ClassDefinition:
		Inspect(n.Action, visit)
	case Decl:
		Inspect(n.Value, visit)
	case Assign:
		Inspect(n.Lhs, visit)
		Inspect(n.Value, visit)
	case MutableBinOp:
		Inspect(n.Lhs, visit)
		Inspect(n.Rhs, visit)
	case BinOp:
		Inspect(n.Lhs, visit)
		Inspect(n.Rhs, visit)
	case IsInstanceOf:
		Inspect(n.Lhs, visit)
		Inspect(n.Rhs, visit)
	case Plus:
		Inspect(n.Value, visit)
	case Neg:
		Inspect(n.Value, visit)
	case Not:
		Inspect(n.Value, visit)
	case Exists:
		Inspect(n.Value, visit)
	case Echo:
		Inspect(n.Value, visit)
	case Return:
		Inspect(n.Value, visit)
	case Throw:
		Inspect(n.Value, visit)
	case Catch:
		Inspect(n.Action, visit)
	case Conditional:
		Inspect(n.Condition, visit)
		Inspect(n.Action, visit)
		Inspect(n.Otherwise, visit)
	case While:
		Inspect(n.Condition, visit)
		Inspect(n.Action, visit)
	case For:
		Inspect(n.Collection, visit)
		Inspect(n.Action, visit)
	case Switch:
		Inspect(n.Value, visit)
		for _, c := range n.Cases {
			Inspect(c, visit)
		}
		Inspect(n.Default, visit)
	case SwitchCase:
		all(n.Values)
		Inspect(n.Action, visit)
	case Go:
		Inspect(n.Fn, visit)
	case Call:
		Inspect(n.Fn, visit)
		all(n.Args)
	case Await:
		Inspect(n.Task, visit)
	case AwaitAll:
		all(n.Tasks)
	case AwaitAny:
		all(n.Tasks)
	case AwaitRace:
		all(n.Tasks)
	case AwaitTimeout:
		Inspect(n.Task, visit)
		Inspect(n.Timeout, visit)
	case FieldAccess:
		Inspect(n.Lhs, visit)
	case Subscript:
		Inspect(n.Lhs, visit)
		Inspect(n.Key, visit)
	case Slice:
		Inspect(n.Lhs, visit)
		Inspect(n.Start, visit)
		Inspect(n.End, visit)
	case Object:
		for _, f := range n.Fields {
			Inspect(f.Value, visit)
		}
	case ArrayLiteral:
		all(n.Elements)
	case StringTemplate:
		all(n.Args)
	}
}
