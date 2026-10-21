package evie_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/hxkhan/evie"
)

// TestWithoutFastPaths runs the whole suite again with the compiler's fast
// paths off. The general path is what the fast paths must agree with, and
// nothing else exercises it, so without this run it breaks unnoticed.
func TestWithoutFastPaths(t *testing.T) {
	if evie.FastPathsOff() {
		t.Skip("this is the run without fast paths")
	}
	if testing.Short() {
		t.Skip("skipped in -short mode")
	}
	cmd := exec.Command(os.Args[0], "-test.count=1", "-test.timeout=5m")
	cmd.Env = append(os.Environ(), "EVIE_TEST_WITHOUT_FAST_PATHS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the suite fails with the fast paths off: %v\n%s", err, out)
	}
}

// needsFastPaths skips a test of something only the fast paths do, such as
// checking argument counts while compiling
func needsFastPaths(t *testing.T) {
	t.Helper()
	if evie.FastPathsOff() {
		t.Skip("argument counts are checked while compiling only when the callee is resolved statically")
	}
}
