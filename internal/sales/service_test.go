package sales

import (
	"testing"
)

func TestNewServiceWiresSalesDomainServices(t *testing.T) {
	svc := NewService(nil)
	if svc.Customers == nil || svc.Quotations == nil || svc.Orders == nil || svc.Products == nil {
		t.Fatalf("NewService() left a domain service nil: %#v", svc)
	}
}
