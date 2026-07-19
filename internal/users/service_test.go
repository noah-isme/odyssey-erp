package users

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct {
	users []User
	err   error
}

func (s stubRepository) ListUsers(context.Context) ([]User, error) { return s.users, s.err }

func TestServiceListUsers(t *testing.T) {
	expected := []User{{ID: 1, Email: "user@example.com"}}
	result, err := NewService(stubRepository{users: expected}).ListUsers(context.Background())
	if err != nil || len(result) != 1 || result[0].Email != expected[0].Email {
		t.Fatalf("ListUsers() = %#v, %v", result, err)
	}
}

func TestServiceListUsersReturnsRepositoryError(t *testing.T) {
	expected := errors.New("database unavailable")
	_, err := NewService(stubRepository{err: expected}).ListUsers(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("ListUsers() error = %v, want %v", err, expected)
	}
}
