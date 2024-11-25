package builtin

/* func Export() {
	std.ImportFn(split)
	std.ImportFn(join)
}

func split(str, sep box.Value) (box.Value, error) {
	if str, ok := str.AsString(); ok {
		if sep, ok := sep.AsString(); ok {

			parts := strings.Split(str, sep)
			result := make([]box.Value, len(parts))
			for i, part := range parts {
				result[i] = box.String(part)
			}
			return result, nil
		}
	}
	return nil, core.ErrTypes
}

func join(parts, sep box.Value) (box.Value, error) {
	if parts, ok := parts.([]box.Value); ok {
		if sep, ok := sep.AsString(); ok {

			strs := make([]string, len(parts))
			for i, part := range parts {
				strs[i] = part.AsString()
			}

			return strings.Join(strs, sep), nil
		}
	}
	return nil, core.ErrTypes
} */
