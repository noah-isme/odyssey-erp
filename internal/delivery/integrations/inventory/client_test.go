package inventory

import (
	"context"
	"testing"
)

func TestClientRequiresInventoryServiceForReduce(t *testing.T) {
	err := NewClient(nil).Reduce(context.Background(), []Item{{ProductID: 7, Quantity: 1}})
	if err == nil || err.Error() != "inventory service not initialized" {
		t.Fatalf("Reduce() error = %v", err)
	}
}

func TestClientReserveOnlyAcceptsEmptyRequestsUntilImplemented(t *testing.T) {
	client := NewClient(nil)
	if err := client.Reserve(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Reserve(context.Background(), []Item{{ProductID: 1, Quantity: 1}}); err == nil {
		t.Fatal("Reserve() accepted a non-empty request for an unimplemented operation")
	}
}
