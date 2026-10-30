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
	mu     sync.RWMutex
	done   chan struct{}
	status taskStatus
	result Value
	err    exception
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
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status != taskPending
}

// outcome returns the result and error without blocking.
func (t *Task) outcome() (Value, exception) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.result, t.err
}

// await waits for the task to settle and returns its result or exception.
// A running fiber gives up the instance runner while it waits so that other
// fibers of the instance can make progress. Safe to call multiple times.
func (t *Task) await(fbr *fiber) (Value, exception) {
	if fbr != nil && fbr.running {
		fbr.unlock()
		<-t.done
		fbr.lock()
	} else {
		<-t.done
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.result, t.err
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
