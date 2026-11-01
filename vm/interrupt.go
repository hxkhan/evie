package vm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/hxkhan/evie/diag"
)

// ErrInterrupted is the cause of the error a script stops with when the host
// calls Interrupt. A run bound to a context stops with the context's cause
// instead, such as context.DeadlineExceeded.
var ErrInterrupted = errors.New("interrupted")

// ---- stopping running code ----
//
// Code runs under a stopper, which says when it has to give up: when the
// stopper's context is done. Loops and calls do not look at the stopper
// itself, which would cost a channel operation each time. They compare the
// instance's signal, which changes whenever any stopper of the instance is
// done, with the value the fiber saw last. Only when it differs does the fiber
// ask its own stopper, so a stop costs nothing until it is requested and code
// under other stoppers merely looks once and carries on.

// stops is the part of the runtime that keeps track of stoppers
type stops struct {
	signal  atomic.Uint32           // changes whenever a stopper of this instance is done
	current atomic.Pointer[stopper] // for host calls without a context; Interrupt replaces it
	mu      sync.Mutex              // guards scoped
	scoped  map[*stopper]struct{}   // stoppers of runs bound to a context that are in progress
	loading *stopper                // the stopper of the package being loaded; nested loads inherit it, guarded by the runner
}

// stopper is what a run checks to know whether it has to stop
type stopper struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	end    func() bool  // removes the watcher that changes the signal
	scoped bool         // bound to a host context, released when its last user is done
	users  atomic.Int32 // callers and fibers that use a scoped stopper
	stops  *stops
}

func (st *stops) init() {
	st.scoped = make(map[*stopper]struct{})
	st.current.Store(st.newStopper(context.Background()))
}

// newStopper derives a stopper from parent; when it is done the signal changes
func (st *stops) newStopper(parent context.Context) *stopper {
	ctx, cancel := context.WithCancelCause(parent)
	s := &stopper{ctx: ctx, cancel: cancel, stops: st}
	s.end = context.AfterFunc(ctx, func() { st.signal.Add(1) })
	return s
}

// scope makes a stopper for a run bound to ctx. The caller is its first user
// and gives it up with drop; tasks the run spawns keep it alive until they end.
// A context that is never done, such as context.Background, adds nothing to
// what Interrupt does, so such a run shares the instance's current stopper.
func (st *stops) scope(ctx context.Context) *stopper {
	if ctx.Done() == nil {
		return st.current.Load()
	}
	s := st.newStopper(ctx)
	s.scoped = true
	s.users.Store(1)
	st.mu.Lock()
	st.scoped[s] = struct{}{}
	st.mu.Unlock()
	return s
}

// hold records one more user of a scoped stopper
func (s *stopper) hold() {
	if s.scoped {
		s.users.Add(1)
	}
}

// drop gives up one use of a scoped stopper; the last user releases it, which
// detaches it from the host's context
func (s *stopper) drop() {
	if !s.scoped || s.users.Add(-1) != 0 {
		return
	}
	s.stops.mu.Lock()
	delete(s.stops.scoped, s)
	s.stops.mu.Unlock()
	s.end() // finishing normally is not a reason for others to look
	s.cancel(context.Canceled)
}

// done reports whether code under s has to stop
func (s *stopper) done() bool {
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

// failure is the exception code under s stops with. Scripts cannot catch it.
func (s *stopper) failure() exception {
	cause := context.Cause(s.ctx)
	return &Error{Report: diag.Report{Message: cause.Error()}, Cause: cause, fatal: true}
}

// Interrupt stops all code of the instance that is running or waiting now:
// calls from the host, package initializers and the tasks they started. Each
// stops at its next loop iteration or call, or as soon as it wakes from an
// await or a blocking native, and fails with an error whose cause is
// ErrInterrupted. Scripts cannot catch it. Calls made after Interrupt returns
// run normally. A native that is busy in Go finishes first; a blocking native
// stops early only when it watches Call.Context.
func (vm *Instance) Interrupt() {
	st := &vm.rt.stops
	st.mu.Lock()
	old := st.current.Swap(st.newStopper(context.Background()))
	old.cancel(ErrInterrupted)
	for s := range st.scoped {
		s.cancel(ErrInterrupted)
	}
	st.mu.Unlock()
	st.signal.Add(1)
}

// forLoading is the stopper for code that runs while packages load: the one a
// context-bound load set up, or the instance's current one. The runner must be held.
func (st *stops) forLoading() *stopper {
	if st.loading != nil {
		return st.loading
	}
	return st.current.Load()
}

// loadingUnder makes s the stopper of the load in progress until the returned
// function restores the previous one; a nil s keeps the current one. The
// runner must be held.
func (st *stops) loadingUnder(s *stopper) (restore func()) {
	if s == nil {
		return func() {}
	}
	prev := st.loading
	st.loading = s
	return func() { st.loading = prev }
}

// attach makes s the stopper of a fiber that is about to run
func (fbr *fiber) attach(s *stopper) {
	fbr.seen = fbr.vm.rt.signal.Load()
	fbr.stop = s
	if s.done() {
		// the signal may have changed before it was read; look at the first chance
		fbr.seen--
	}
}

// detach gives the fiber's stopper back
func (fbr *fiber) detach() {
	if fbr.stop != nil {
		fbr.stop.drop()
		fbr.stop = nil
	}
}

// signaled is the test every loop iteration and call makes: whether any code
// of the instance was asked to stop since this fiber last looked
func (fbr *fiber) signaled() bool {
	return fbr.vm.rt.signal.Load() != fbr.seen
}

// stopped tells, after the signal changed, whether this fiber has to stop. It
// keeps saying so on every later look, so a native that swallows the error
// does not let the script carry on.
func (fbr *fiber) stopped() exception {
	signal := fbr.vm.rt.signal.Load()
	if fbr.stop.done() {
		return fbr.stop.failure()
	}
	fbr.seen = signal
	return nil
}

// halt is stopped located at s, for the places that check
func (fbr *fiber) halt(s site) exception {
	if exc := fbr.stopped(); exc != nil {
		return exc.at(s)
	}
	return nil
}
