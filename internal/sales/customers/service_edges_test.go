package customers

import (
	"context"
	"testing"
)

func TestUpdateWithNoChangesReturnsExistingCustomer(t *testing.T) {
	repo := newMemoryRepo()
	service := NewService(repo)
	created, err := service.Create(context.Background(), CreateCustomerRequest{CompanyID: 1, Code: "C-1", Name: "Customer"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(context.Background(), created.ID, UpdateCustomerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Customer" {
		t.Fatalf("Update() = %#v", updated)
	}
}

func TestUpdateUnknownCustomerReturnsNotFound(t *testing.T) {
	_, err := NewService(newMemoryRepo()).Update(context.Background(), 99, UpdateCustomerRequest{})
	if err == nil {
		t.Fatal("Update() did not report a missing customer")
	}
}
