package rt

import "strings"

var Split = NewFn("split", func(str any, sep any) any {
	str, ok1 := str.(string)
	str, ok2 := sep.(string)

	if !ok1 || !ok2 {

	}

	parts := strings.Split(str.(string), sep.(string))

	result := make([]any, len(parts))
	for i, part := range parts {
		result[i] = part
	}
	return result
})

var Join = NewFn("join", func(parts any, sep any) any {
	strs := make([]string, len(parts.([]any)))
	for i, part := range parts.([]any) {
		strs[i] = part.(string)
	}

	return strings.Join(strs, sep.(string))
})
