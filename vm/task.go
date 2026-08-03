package vm

import (
	"sync"
)

type TaskStatus int

const (
	TaskPending TaskStatus = iota
	TaskCompleted
	TaskFailed
)

// Task represents an asynchronous unit of execution with memoized state.
type Task struct {
	mu     sync.RWMutex
	done   chan struct{}
	status TaskStatus
	result Value
	err    Exception
}

// NewTaskInstance creates a new pending Task.
func NewTaskInstance() *Task {
	return &Task{
		done:   make(chan struct{}),
		status: TaskPending,
	}
}

// Resolve marks the task as completed with a successful result.
func (t *Task) Resolve(res Value) {
	t.mu.Lock()
	if t.status == TaskPending {
		t.result = res
		t.status = TaskCompleted
		close(t.done)
	}
	t.mu.Unlock()
}

// Reject marks the task as failed with an exception.
func (t *Task) Reject(err Exception) {
	t.mu.Lock()
	if t.status == TaskPending {
		t.err = err
		t.status = TaskFailed
		close(t.done)
	}
	t.mu.Unlock()
}

// Finish resolves or rejects the task depending on whether err is non-nil.
func (t *Task) Finish(res Value, err Exception) {
	if err != nil {
		t.Reject(err)
	} else {
		t.Resolve(res)
	}
}

// Done returns a channel that is closed when the task completes.
func (t *Task) Done() <-chan struct{} {
	return t.done
}

// IsDone returns true if the task has settled (resolved or rejected).
func (t *Task) IsDone() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status != TaskPending
}

// Status returns the current status of the task.
func (t *Task) Status() TaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// Result returns the result and error without blocking.
func (t *Task) Result() (Value, Exception) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.result, t.err
}

// Await waits for the task to complete, releasing the GIL if the fiber is synced,
// and returns the result or exception. Safe to call multiple times.
func (t *Task) Await(fbr *Fiber) (Value, Exception) {
	if fbr != nil && fbr.synced && fbr.vm != nil {
		fbr.vm.rt.ReleaseGIL()
		<-t.done
		fbr.vm.rt.AcquireGIL()
	} else {
		<-t.done
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.result, t.err
}

// NewTask wraps a Go function in a Task.
func NewTask(fn func() (Value, Exception)) Value {
	task := NewTaskInstance()
	go func() {
		res, err := fn()
		task.Finish(res, err)
	}()
	return BoxTask(task)
}
