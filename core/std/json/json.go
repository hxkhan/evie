package json

import (
	"encoding/json"

	"github.com/hk-32/evie/core"
)

func decode(v any) (any, error) {
	str, ok := v.(string)
	if ok {
		var v any
		err := json.Unmarshal([]byte(str), &v)
		return v, err
	}
	return nil, core.ErrTypes

}
