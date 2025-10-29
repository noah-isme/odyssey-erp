package guard

import (
	"os"
	"sync"
)

var once sync.Once

func init() {
	once.Do(func() {
		if os.Getenv("ODYSSEY_TEST_MODE") == "" {
			_ = os.Setenv("ODYSSEY_TEST_MODE", "1")
		}
	})
}
