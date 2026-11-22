package vm

import (
	"sync"

	"github.com/hxkhan/evie/diag"
)

type taskStatus int

const (
	taskPending taskStatus = iota
	taskCompleted
	taskFailed
)

type taskWaiter struct {
	fn func(*Task)
}

// Task represents an asynchronous unit of execution with memoized state.
type Task struct {
	mu      sync.Mutex
	done    chan struct{}
	status  taskStatus
	result  Value
	err     exception
	stop    *stopper // the run the task belongs to; nil for host tasks
	waiters []*taskWaiter
}

// newTaskInstance creates a new pending Task.
func newTaskInstance() *Task {
	return &Task{
		done:   make(chan struct{}),
		status: taskPending,
	}
}

// onSettle calls fn as soon as the task settles (immediately if already settled)
// and returns a cancel function that unregisters the waiter without leaking.
// fn must not block.
func (t *Task) onSettle(fn func(*Task)) func() {
	t.mu.Lock()
	if t.status != taskPending {
		t.mu.Unlock()
		fn(t)
		return func() {}
	}
	w := &taskWaiter{fn: fn}
	t.waiters = append(t.waiters, w)
	t.mu.Unlock()

	return func() {
		t.mu.Lock()
		for i, cur := range t.waiters {
			if cur == w {
				last := len(t.waiters) - 1
				t.waiters[i] = t.waiters[last]
				t.waiters[last] = nil
				t.waiters = t.waiters[:last]
				break
			}
		}
		t.mu.Unlock()
	}
}

// resolve marks the task as completed with a successful result.
func (t *Task) resolve(res Value) {
	t.mu.Lock()
	var waiters []*taskWaiter
	if t.status == taskPending {
		t.result = res
		t.status = taskCompleted
		waiters = t.waiters
		t.waiters = nil
		close(t.done)
	}
	t.mu.Unlock()
	for _, w := range waiters {
		w.fn(t)
	}
}

// reject marks the task as failed with an exception.
func (t *Task) reject(err exception) {
	t.mu.Lock()
	var waiters []*taskWaiter
	if t.status == taskPending {
		t.err = err
		t.status = taskFailed
		waiters = t.waiters
		t.waiters = nil
		close(t.done)
	}
	t.mu.Unlock()
	for _, w := range waiters {
		w.fn(t)
	}
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

// outcomeFor returns the task's outcome as seen by fbr. The error is a copy,
// since every awaiter records its own trace on it, and a stop from another run
// becomes an ordinary catchable error for a fiber that is still running.
func (t *Task) outcomeFor(fbr *fiber) (Value, exception) {
	res, err := t.outcome()
	if err != nil {
		err = err.clone(err.fatal && fbr != nil && fbr.stop.done())
	}
	return res, err
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
		if t.stop != nil && t.stop.stopsWith(fbr.stop) {
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

	return t.outcomeFor(fbr)
}

// Cancel asks the task to stop. It has no effect when the task has already
// settled.
func (t *Task) Cancel() {
	t.mu.Lock()
	if t.status != taskPending {
		t.mu.Unlock()
		return
	}
	stop := t.stop
	t.mu.Unlock()

	if stop != nil {
		stop.cancel(ErrCanceled)
		stop.stops.signal.Add(1)
	} else {
		t.reject(&Error{Report: diag.Report{Message: ErrCanceled.Error()}, Cause: ErrCanceled})
	}
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
