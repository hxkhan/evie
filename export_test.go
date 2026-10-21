package evie

import "os"

// EVIE_TEST_WITHOUT_FAST_PATHS reruns the tests with the compiler's fast
// paths off; TestWithoutFastPaths sets it for a second run of the suite
func init() {
	testWithoutFastPaths = os.Getenv("EVIE_TEST_WITHOUT_FAST_PATHS") != ""
}

// FastPathsOff reports whether this test run compiles without the fast paths
func FastPathsOff() bool {
	return testWithoutFastPaths
}
