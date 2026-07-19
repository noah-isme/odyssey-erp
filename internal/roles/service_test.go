package roles

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct {
	roles     []Role
	created   Role
	listErr   error
	createErr error
}

func (s stubRepository) ListRoles(context.Context, RoleListFilters) ([]Role, error) {
	return s.roles, s.listErr
}
func (s stubRepository) CreateRole(context.Context, string, string) (Role, error) {
	return s.created, s.createErr
}

func TestServiceListRoles(t *testing.T) {
	result, err := NewService(stubRepository{roles: []Role{{ID: 1, Name: "Finance"}}}).ListRoles(context.Background(), RoleListFilters{})
	if err != nil || len(result) != 1 || result[0].Name != "Finance" {
		t.Fatalf("ListRoles() = %#v, %v", result, err)
	}
}

func TestServiceCreateRolePropagatesError(t *testing.T) {
	expected := errors.New("duplicate role")
	_, err := NewService(stubRepository{createErr: expected}).CreateRole(context.Background(), "Finance", "")
	if !errors.Is(err, expected) {
		t.Fatalf("CreateRole() error = %v, want %v", err, expected)
	}
}
