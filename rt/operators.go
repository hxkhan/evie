package rt

// ADD : '+' operator
func ADD(a, b any) any {
	if a, ok := a.(int32); ok {
		if b, ok := b.(int32); ok {
			return a + b
		}
		if b, ok := b.(float64); ok {
			return float64(a) + b
		}
	}
	if a, ok := a.(float64); ok {
		if b, ok := b.(int32); ok {
			return a + float64(b)
		}
		if b, ok := b.(float64); ok {
			return a + b
		}
	}
	panic(NewError("TypeError", "Operator '+' unsupported on '%T' and '%T'", a, b))
}

// SUB : '-' operator
func SUB(a, b any) any {
	if a, ok := a.(int32); ok {
		if b, ok := b.(int32); ok {
			return a - b
		}
		if b, ok := b.(float64); ok {
			return float64(a) - b
		}
	}
	if a, ok := a.(float64); ok {
		if b, ok := b.(int32); ok {
			return a - float64(b)
		}
		if b, ok := b.(float64); ok {
			return a - b
		}
	}
	panic(NewError("TypeError", "Operator '-' unsupported on '%T' and '%T'", a, b))
}

// MORE : '>' operator
func MORE(a, b any) bool {
	if a, ok := a.(int32); ok {
		if b, ok := b.(int32); ok {
			return a > b
		}
		if b, ok := b.(float64); ok {
			return float64(a) > b
		}
	}
	if a, ok := a.(float64); ok {
		if b, ok := b.(int32); ok {
			return a > float64(b)
		}
		if b, ok := b.(float64); ok {
			return a > b
		}
	}
	panic(NewError("TypeError", "Operator '>' unsupported on '%T' and '%T'", a, b))
}

// LESS : '<' operator
func LESS(a, b any) bool {
	if a, ok := a.(int32); ok {
		if b, ok := b.(int32); ok {
			return a < b
		}
		if b, ok := b.(float64); ok {
			return float64(a) < b
		}
	}
	if a, ok := a.(float64); ok {
		if b, ok := b.(int32); ok {
			return a < float64(b)
		}
		if b, ok := b.(float64); ok {
			return a < b
		}
	}
	panic(NewError("TypeError", "Operator '<' unsupported on '%T' and '%T'", a, b))
}
