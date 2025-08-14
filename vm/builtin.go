package vm

var builtins = map[string]*Value{
	"str": BoxGoFunc(func(a Value) (Value, *Exception) {
		return BoxString(a.String()), nil
	}).Allocate(),
}

/*
BUG:
change `fn inc(n)` in inc.ev to `fn inc(n) unsynced`.
1. run it & time it
2. comment out the code below
3. run it & time it
4. Why are we speeding up and slowing down based on dead code that does basically nothing?
*/
/* var bug Value

func init() {
	t := func(Value) (Value, *Exception) {
		return Value{}, nil
	}
	bug = Value{scalar: goFuncType, pointer: unsafe.Pointer(&GoFunc{nargs: 1, ptr: unsafe.Pointer(&t)})}
} */
