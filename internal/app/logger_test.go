package app

import (
	"os"
	"testing"
)

func TestLoggerOutputKeepsRouteDumpJSONClean(t *testing.T) {
	t.Setenv("ODYSSEY_DUMP_ROUTES", "1")
	if got := loggerOutput(); got != os.Stderr {
		t.Fatalf("loggerOutput() = %v, want stderr during route dump", got)
	}
}

func TestLoggerOutputDefaultsToStdout(t *testing.T) {
	t.Setenv("ODYSSEY_DUMP_ROUTES", "")
	if got := loggerOutput(); got != os.Stdout {
		t.Fatalf("loggerOutput() = %v, want stdout by default", got)
	}
}
