package testing

import (
	"os"
	"sync"
	stdtesting "testing"
)

var once sync.Once

func ensureTestMode() {
	once.Do(func() {
		_ = os.Setenv("ODYSSEY_TEST_MODE", "1")
		if os.Getenv("GOTENBERG_URL") == "" {
			_ = os.Setenv("GOTENBERG_URL", "http://127.0.0.1:0")
		}
	})
}

func init() {
	ensureTestMode()
}

func TestMain(m *stdtesting.M) {
	ensureTestMode()
	os.Exit(m.Run())
}
