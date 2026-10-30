package vm

import (
	"github.com/hxkhan/evie/ast"
)

// spawn runs fn with params on a new fiber of its own and returns the task that
// settles with the outcome. The fiber waits for the instance runner before it
// executes any evie code.
func (vm *Instance) spawn(fn *userFn, params []Value) *Task {
	task := newTaskInstance()
	vm.rt.wg.Go(func() {
		fbr := vm.newFiber()
		fbr.lock()
		result, exc := fbr.runTask(fn, params)
		fbr.unlock()
		vm.releaseFiber(fbr)
		task.finish(result, exc)
	})
	return task
}

// runTask executes fn with params as the first frame of a fresh fiber
func (fbr *fiber) runTask(fn *userFn, params []Value) (result Value, exc exception) {
	defer func() {
		if r := recover(); r != nil {
			result, exc = Value{}, recovered(r)
			fbr.unwind(0, 0, nil)
		}
	}()

	return fn.call(fbr, params)
}

// spawnNative runs a native function with params on a fiber of its own
func (vm *Instance) spawnNative(fn *GoFunc, params []Value) *Task {
	task := newTaskInstance()
	vm.rt.wg.Go(func() {
		fbr := vm.newFiber()
		fbr.lock()
		result, exc := fn.callGuarded(fbr, params...)
		fbr.unlock()
		vm.releaseFiber(fbr)
		task.finish(result, exc)
	})
	return task
}

func (vm *Instance) emitGo(node ast.Go) instruction {
	site := vm.at(node)
	call, isCall := node.Fn.(ast.Call)
	if !isCall {
		panic(vm.compileErrorAt(node, "go expects a function call"))
	}

	// compile arguments
	arguments := make([]instruction, len(call.Args))
	for i, arg := range call.Args {
		arguments[i] = vm.compile(arg)
	}

	// evaluate the arguments into a fresh slice, with an optional receiver in front
	evalArgs := func(fbr *fiber, this *Value) ([]Value, exception) {
		offset := 0
		if this != nil {
			offset = 1
		}
		params := make([]Value, offset+len(arguments))
		if this != nil {
			params[0] = *this
		}
		for i, argument := range arguments {
			arg, exc := argument(fbr)
			if exc != nil {
				return nil, exc
			}
			params[offset+i] = arg
		}
		return params, nil
	}

	value := vm.compile(call.Fn)
	return func(fbr *fiber) (result Value, exc exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		if fn, isUserFn := value.asUserFn(); isUserFn {
			if exc := fn.accepts(len(arguments)); exc != nil {
				return Value{}, exc.at(site)
			}
			params, exc := evalArgs(fbr, nil)
			if exc != nil {
				return Value{}, exc
			}
			return boxTask(vm.spawn(fn, params)), nil
		}

		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			if exc := fn.accepts(len(arguments)); exc != nil {
				return Value{}, exc
			}
			params, exc := evalArgs(fbr, nil)
			if exc != nil {
				return Value{}, exc
			}
			return boxTask(vm.spawnNative(fn, params)), nil
		}

		if m, isMethod := value.asMethod(); isMethod {
			if ufn, isUserFn := m.fn.asUserFn(); isUserFn {
				var this *Value
				if ufn.isMethod {
					this = &m.this
				}
				if exc := ufn.accepts(len(arguments) + boolToInt(this != nil)); exc != nil {
					return Value{}, exc
				}
				params, exc := evalArgs(fbr, this)
				if exc != nil {
					return Value{}, exc
				}
				return boxTask(vm.spawn(ufn, params)), nil
			}

			if fn, isGoFunc := m.fn.AsGoFunc(); isGoFunc {
				var this *Value
				if fn.isMethod {
					this = &m.this
				}
				if exc := fn.accepts(len(arguments) + boolToInt(this != nil)); exc != nil {
					return Value{}, exc
				}
				params, exc := evalArgs(fbr, this)
				if exc != nil {
					return Value{}, exc
				}
				return boxTask(vm.spawnNative(fn, params)), nil
			}
		}

		return Value{}, errorAt(site, "cannot call %s", article(value)).labelled("this is " + article(value))
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (vm *Instance) emitAwait(node ast.Await) instruction {
	value := vm.compile(node.Task)

	return func(fbr *fiber) (Value, exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		if task, ok := v.AsTask(); ok {
			return task.await(fbr)
		}

		// Awaiting a non-task value returns the value directly (identity await)
		return v, nil
	}
}

func extractTasks(evaluated []Value) ([]*Task, []Value) {
	if len(evaluated) == 1 {
		if arr, ok := evaluated[0].AsArray(); ok {
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

	return func(fbr *fiber) (Value, exception) {
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
				res, err := t.await(fbr)
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

type outcome struct {
	res Value
	err exception
}

// settle waits for the first task to settle, or for a successful one when
// successOnly is set, giving up the instance runner while it waits
func settle(fbr *fiber, tasks []*Task, successOnly bool) (outcome, bool) {
	outcomes := make(chan outcome, len(tasks))
	for _, t := range tasks {
		go func(task *Task) {
			<-task.Done()
			res, err := task.outcome()
			outcomes <- outcome{res: res, err: err}
		}(t)
	}

	if fbr.running {
		fbr.unlock()
		defer fbr.lock()
	}

	for failed := 0; failed < len(tasks); {
		out := <-outcomes
		if out.err == nil || !successOnly {
			return out, true
		}
		failed++
	}
	return outcome{}, false
}

func (vm *Instance) emitAwaitAny(node ast.AwaitAny) instruction {
	site := vm.at(node)
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *fiber) (Value, exception) {
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
			return Value{}, errorAt(site, "await.any needs at least one task")
		}

		// Check if any item is already a resolved non-task or completed task without error
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.isDone() {
				res, err := t.outcome()
				if err == nil {
					return res, nil
				}
			}
		}

		if first, ok := settle(fbr, tasks, true); ok {
			return first.res, nil
		}
		return Value{}, customError("all tasks failed in await.any")
	}
}

func (vm *Instance) emitAwaitRace(node ast.AwaitRace) instruction {
	site := vm.at(node)
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *fiber) (Value, exception) {
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
			return Value{}, errorAt(site, "await.race needs at least one task")
		}

		// Check if any item is already a non-task or completed task
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.isDone() {
				return t.outcome()
			}
		}

		first, _ := settle(fbr, tasks, false)
		return first.res, first.err
	}
}
