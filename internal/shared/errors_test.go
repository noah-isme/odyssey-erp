package shared

import (
	"errors"
	"fmt"
	"testing"
)

func TestRegisterUserSafeMessage(t *testing.T) {
	sentinel := errors.New("test: registered sentinel")
	RegisterUserSafeMessage(sentinel, "Registered message.")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "registered sentinel", err: sentinel, want: "Registered message."},
		{name: "wrapped registered sentinel", err: fmt.Errorf("repo: %w", sentinel), want: "Registered message."},
		{name: "built-in sentinel", err: ErrNotFound, want: "The requested item was not found."},
		{name: "unknown error", err: errors.New("pq: boom"), want: "An unexpected error occurred. Please try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := UserSafeMessage(tc.err); got != tc.want {
				t.Fatalf("UserSafeMessage=%q want %q", got, tc.want)
			}
		})
	}
}
