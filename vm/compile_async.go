package vm

import (
	"github.com/hxkhan/evie/ast"
)

func (vm *Instance) emitGo(node ast.Go) instruction {
	if node, isCall := node.Fn.(ast.Call); isCall {
		// compile arguments
		arguments := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			arguments[i] = vm.compile(arg)
		}

		spawnUserFnTask := func(fn *UserFn, params []Value) *Task {
			task := NewTaskInstance()
			vm.rt.wg.Go(func() {
				// setup new fiber
				fbr := vm.rt.fibers.Get().(*Fiber)
				fbr.active = fn
				fbr.base = 0
				fbr.stack = fbr.stack[:0]
				fbr.synced = fn.mode == ast.SyncedMode

				// unique locals
				var result Value
				var exc Exception

				// setup stack locals
				for idx, escapes := range fn.locals {
					if !escapes {
						fbr.stack = append(fbr.stack, fbr.pop())
					} else {
						fbr.stack = append(fbr.stack, &Value{})
					}

					// assign arguments
					if !fn.isVariadic {
						if idx < len(params) {
							*(fbr.stack[idx]) = params[idx]
						}
					} else {
						variadicIdx := len(fn.args) - 1
						if idx < variadicIdx {
							*(fbr.stack[idx]) = params[idx]
						} else if idx == variadicIdx {
							tailCount := len(params) - variadicIdx
							tail := make([]Value, tailCount)
							copy(tail, params[variadicIdx:])
							*(fbr.stack[idx]) = BoxArray(&Array{Data: tail})
						}
					}
				}

				// run code
				if fn.mode == ast.UndefinedMode {
					result, exc = fn.code(fbr)
				} else {
					vm.rt.AcquireGIL()
					result, exc = fn.code(fbr)
					vm.rt.ReleaseGIL()
				}

				// cleanup fiber and release
				fbr.push(fn.recyclable)
				fbr.popStack(len(fn.locals))
				vm.rt.fibers.Put(fbr)

				// return result but catch relevant signals
				switch exc {
				case nil:
				case signalReturn:
					exc = nil
				}
				task.Finish(result, exc)
			})
			return task
		}

		spawnGoFuncTask := func(fn *GoFunc, params []Value) *Task {
			task := NewTaskInstance()
			vm.rt.wg.Go(func() {
				var result Value
				var exc Exception

				// setup new fiber
				fbr := vm.rt.fibers.Get().(*Fiber)
				fbr.base = 0
				fbr.stack = fbr.stack[:0]

				for _, p := range params {
					box := fbr.pop()
					*box = p
					fbr.stack = append(fbr.stack, box)
				}

				// run code
				if fn.Mode == ast.UndefinedMode {
					result, exc = fn.Fn(fbr)
				} else {
					vm.rt.AcquireGIL()
					result, exc = fn.Fn(fbr)
					vm.rt.ReleaseGIL()
				}

				fbr.push(len(params))
				fbr.popStack(len(params))
				vm.rt.fibers.Put(fbr)

				task.Finish(result, exc)
			})
			return task
		}

		// generic compilation
		value := vm.compile(node.Fn)
		return func(fbr *Fiber) (result Value, exc Exception) {
			value, exc := value(fbr)
			if exc != nil {
				return value, exc
			}

			// check if it is a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {
				if fn.isVariadic {
					minArgs := len(fn.args) - 1
					if len(arguments) < minArgs {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments))
						}
						return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
					}
				} else {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}
				}

				// evaluate arguments
				params := make([]Value, len(arguments))
				for i, argument := range arguments {
					arg, exc := argument(fbr)
					if exc != nil {
						return arg, exc
					}
					params[i] = arg
				}

				return BoxTask(spawnUserFnTask(fn, params)), nil
			}

			// try go func
			if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
				if fn.IsVariadic {
					if len(arguments) < fn.Arguments {
						return Value{}, CustomError("callable requires at least %v argument(s), %v provided", fn.Arguments, len(arguments))
					}
				} else {
					if fn.Arguments != len(arguments) {
						return Value{}, CustomError("callable requires %v argument(s), %v provided", fn.Arguments, len(arguments))
					}
				}

				// evaluate arguments
				params := make([]Value, len(arguments))
				for i, argument := range arguments {
					arg, exc := argument(fbr)
					if exc != nil {
						return arg, exc
					}
					params[i] = arg
				}

				return BoxTask(spawnGoFuncTask(fn, params)), nil
			}

			// try method
			if m, isMethod := value.asMethod(); isMethod {
				if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
					isMethodDef := len(ufn.args) > 0 && ufn.args[0] == "this"
					if isMethodDef {
						if ufn.isVariadic {
							minArgs := len(ufn.args) - 2
							if len(arguments) < minArgs {
								return Value{}, CustomError("method '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
							}
						} else {
							if len(ufn.args)-1 != len(arguments) {
								return Value{}, CustomError("method '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args)-1, len(arguments))
							}
						}

						params := make([]Value, 1+len(arguments))
						params[0] = m.this
						for i, argument := range arguments {
							arg, exc := argument(fbr)
							if exc != nil {
								return arg, exc
							}
							params[1+i] = arg
						}

						return BoxTask(spawnUserFnTask(ufn, params)), nil
					} else {
						if ufn.isVariadic {
							minArgs := len(ufn.args) - 1
							if len(arguments) < minArgs {
								if ufn.name != "λ" {
									return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
								}
								return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
							}
						} else {
							if len(ufn.args) != len(arguments) {
								if ufn.name != "λ" {
									return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args), len(arguments))
								}
								return Value{}, CustomError("callable requires %v argument(s), %v provided", len(ufn.args), len(arguments))
							}
						}

						params := make([]Value, len(arguments))
						for i, argument := range arguments {
							arg, exc := argument(fbr)
							if exc != nil {
								return arg, exc
							}
							params[i] = arg
						}

						return BoxTask(spawnUserFnTask(ufn, params)), nil
					}
				}

				if fn, isGoFunc := m.fn.AsGoFunc(); isGoFunc {
					if fn.IsVariadic {
						if len(arguments) < fn.Arguments-1 {
							return Value{}, CustomError("method requires at least %v argument(s), %v provided", fn.Arguments-1, len(arguments))
						}
					} else {
						if fn.Arguments-1 != len(arguments) {
							return Value{}, CustomError("method requires %v argument(s), %v provided", fn.Arguments-1, len(arguments))
						}
					}

					params := make([]Value, 1+len(arguments))
					params[0] = m.this
					for i, argument := range arguments {
						arg, exc := argument(fbr)
						if exc != nil {
							return arg, exc
						}
						params[1+i] = arg
					}

					return BoxTask(spawnGoFuncTask(fn, params)), nil
				}
			}

			return Value{}, CustomError("cannot call a non-function '%v'", value)
		}
	}
	panic("go expected call, got something else")
}

func (vm *Instance) emitAwait(node ast.Await) instruction {
	value := vm.compile(node.Task)

	return func(fbr *Fiber) (Value, Exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		if task, ok := v.AsTask(); ok {
			return task.Await(fbr)
		}

		// Awaiting a non-task value returns the value directly (identity await)
		return v, nil
	}
}

func extractTasks(evaluated []Value) ([]*Task, []Value) {
	if len(evaluated) == 1 {
		if arr, ok := evaluated[0].AsArray(); ok {
			arr.MU.RLock()
			defer arr.MU.RUnlock()
			tasks := make([]*Task, len(arr.Data))
			raw := make([]Value, len(arr.Data))
			for i, item := range arr.Data {
				raw[i] = item
				if t, ok := item.AsTask(); ok {
					tasks[i] = t
				}
			}
			return tasks, raw
		}
	}

	tasks := make([]*Task, len(evaluated))
	for i, item := range evaluated {
		if t, ok := item.AsTask(); ok {
			tasks[i] = t
		}
	}
	return tasks, evaluated
}

func (vm *Instance) emitAwaitAll(node ast.AwaitAll) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		results := make([]Value, len(tasks))
		for i, t := range tasks {
			if t != nil {
				res, err := t.Await(fbr)
				if err != nil {
					return res, err
				}
				results[i] = res
			} else {
				results[i] = raw[i]
			}
		}

		return BoxArray(NewArray(results...)), nil
	}
}

func (vm *Instance) emitAwaitAny(node ast.AwaitAny) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		if len(tasks) == 0 {
			return Value{}, CustomError("await.any called with empty list of tasks")
		}

		// Check if any item is already a resolved non-task or completed task without error
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.IsDone() {
				res, err := t.Result()
				if err == nil {
					return res, nil
				}
			}
		}

		type outcome struct {
			res Value
			err Exception
		}

		outcomes := make(chan outcome, len(tasks))
		for _, t := range tasks {
			go func(task *Task) {
				<-task.Done()
				res, err := task.Result()
				outcomes <- outcome{res: res, err: err}
			}(t)
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.ReleaseGIL()
		}

		var firstSuccess *outcome
		failedCount := 0
		for failedCount < len(tasks) {
			out := <-outcomes
			if out.err == nil {
				firstSuccess = &out
				break
			}
			failedCount++
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.AcquireGIL()
		}

		if firstSuccess != nil {
			return firstSuccess.res, nil
		}

		return Value{}, CustomError("all tasks failed in await.any")
	}
}

func (vm *Instance) emitAwaitRace(node ast.AwaitRace) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		if len(tasks) == 0 {
			return Value{}, CustomError("await.race called with empty list of tasks")
		}

		// Check if any item is already a non-task or completed task
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.IsDone() {
				res, err := t.Result()
				return res, err
			}
		}

		type outcome struct {
			res Value
			err Exception
		}

		outcomes := make(chan outcome, len(tasks))
		for _, t := range tasks {
			go func(task *Task) {
				<-task.Done()
				res, err := task.Result()
				outcomes <- outcome{res: res, err: err}
			}(t)
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.ReleaseGIL()
		}

		first := <-outcomes

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.AcquireGIL()
		}

		return first.res, first.err
	}
}
