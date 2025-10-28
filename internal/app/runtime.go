package app

import (
	"os"
	"sync"
	"sync/atomic"
)

const testModeEnv = "ODYSSEY_TEST_MODE"

var (
	testModeFlag atomic.Bool
	testModeOnce sync.Once
)

// detectTestMode reads the ODYSSEY_TEST_MODE flag once.
func detectTestMode() {
	testModeFlag.Store(os.Getenv(testModeEnv) == "1")
}

// InTestMode reports whether the application should skip runtime side effects.
func InTestMode() bool {
	testModeOnce.Do(detectTestMode)
	return testModeFlag.Load()
}

// RefreshTestMode updates the cached flag after environment changes.
func RefreshTestMode() {
	detectTestMode()
}
