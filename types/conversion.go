package types

import "github.com/hxkhan/evie/ast"

func From(node ast.Node) (Type, bool) {
	switch v := node.(type) {
	case ast.Ident:
		switch v.Name {
		case "int":
			return Int, true
		case "float":
			return Float, true
		case "string":
			return String, true
		case "bool":
			return Bool, true
		default:
			return nil, false
		}

	case ast.FnType:
		params := make([]Type, len(v.Params))
		for i, param := range v.Params {
			p, ok := From(param)
			if !ok {
				return nil, false
			}
			params[i] = p
		}

		returnType, ok := From(v.Returns)
		if !ok {
			return nil, false
		}

		return &FnType{
			Params: params,
			Return: returnType,
		}, true

		/* case ast.UnionType:
		return UnionTypeFrom(v), true */
	}
	return nil, false
}
