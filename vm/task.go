package vm

import (
	"sync"
)

type taskStatus int

const (
	taskPending taskStatus = iota
	taskCompleted
	taskFailed
)

// Task represents an asynchronous unit of execution with memoized state.
type Task struct {
	mu     sync.Mutex
	done   chan struct{}
	status taskStatus
	result Value
	err    exception
	stop   *stopper // the run the task belongs to; nil for host tasks
}

// newTaskInstance creates a new pending Task.
func newTaskInstance() *Task {
	return &Task{
		done:   make(chan struct{}),
		status: taskPending,
	}
}

// resolve marks the task as completed with a successful result.
func (t *Task) resolve(res Value) {
	t.mu.Lock()
	if t.status == taskPending {
		t.result = res
		t.status = taskCompleted
		close(t.done)
	}
	t.mu.Unlock()
}

// reject marks the task as failed with an exception.
func (t *Task) reject(err exception) {
	t.mu.Lock()
	if t.status == taskPending {
		t.err = err
		t.status = taskFailed
		close(t.done)
	}
	t.mu.Unlock()
}

// finish resolves or rejects the task depending on whether err is non-nil.
func (t *Task) finish(res Value, err exception) {
	if err != nil {
		t.reject(err)
	} else {
		t.resolve(res)
	}
}

// Done returns a channel that is closed when the task completes.
func (t *Task) Done() <-chan struct{} {
	return t.done
}

// isDone returns true if the task has settled (resolved or rejected).
func (t *Task) isDone() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status != taskPending
}

// outcome returns the result and error without blocking.
func (t *Task) outcome() (Value, exception) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.result, t.err
}

// await waits for the task to settle and returns its result or exception.
// A running fiber gives up the instance runner while it waits so that other
// fibers of the instance can make progress. Safe to call multiple times.
//
// The wait ends early when the fiber has to stop. A task that was itself
// stopped fails with the stop's error, which only the run that was stopped
// cannot catch.
func (t *Task) await(fbr *fiber) (Value, exception) {
	if fbr == nil {
		<-t.done
		return t.outcome()
	}

	select {
	case <-t.done:
	default:
		if fbr.running {
			fbr.unlock()
			defer fbr.lock()
		}
		if t.stop == fbr.stop {
			// the task stops when this fiber has to, and then it settles
			<-t.done
			break
		}
		select {
		case <-t.done:
		case <-fbr.stop.ctx.Done():
			return Value{}, fbr.stop.failure()
		}
	}

	res, err := t.outcome()
	if err != nil && err.fatal && !fbr.stop.done() {
		// another run was stopped; for this one it is an ordinary failure
		err = &Error{Report: err.Report, Code: err.Code, Cause: err.Cause}
	}
	return res, err
}

// Result returns what the task finished with, or an error that it failed with.
// It does not wait; use Done to wait for the task to finish.
func (t *Task) Result() (Value, error) {
	v, exc := t.outcome()
	if exc != nil {
		return v, exc
	}
	return v, nil
}

// Async runs fn on its own goroutine and returns a task scripts can await,
// for natives whose work takes a while. An error fn returns is what awaiting
// the task throws.
func Async(fn func() (Value, error)) Value {
	task := newTaskInstance()
	go func() {
		res, err := fn()
		task.finish(res, toException(err))
	}()
	return boxTask(task)
}
