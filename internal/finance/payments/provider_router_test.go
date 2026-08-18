package payments_test

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

type routerPort struct{ submits int }

func (p *routerPort) ValidateConnection(context.Context, automation.ConnectionRef) error { return nil }
func (p *routerPort) Submit(context.Context, automation.ConnectionRef, payments.Instruction) (payments.Submission, error) {
	p.submits++
	return payments.Submission{Status: payments.SubmissionStatusSubmitted}, nil
}
func (p *routerPort) Lookup(context.Context, automation.ConnectionRef, automation.ExternalReference) (payments.Settlement, error) {
	return payments.Settlement{Status: payments.SettlementStatusSettled}, nil
}
func (p *routerPort) Cancel(context.Context, automation.ConnectionRef, automation.ExternalReference) (payments.Settlement, error) {
	return payments.Settlement{Status: payments.SettlementStatusCancelled}, nil
}
func (p *routerPort) GenerateFile(context.Context, automation.ConnectionRef, []payments.Instruction) (payments.ExportArtifact, error) {
	return payments.ExportArtifact{Checksum: "test"}, nil
}

func TestProviderRouterNormalizesProviderNamesAndFailsClosed(t *testing.T) {
	port := &routerPort{}
	router := payments.NewProviderRouter(map[string]payments.ExecutionPort{"midtrans-iris": port})
	ref := automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "MIDTRANS_IRIS"}
	instruction := payments.Instruction{
		Reference:      automation.ExternalReference{Connection: ref, ObjectType: "payment", ObjectID: "p-1"},
		Correlation:    automation.Correlation{ID: "c-1"},
		BeneficiaryRef: "bank:123",
		Amount:         automation.MustParseExact("1"),
	}
	if _, err := router.Submit(context.Background(), ref, instruction); err != nil {
		t.Fatal(err)
	}
	if port.submits != 1 {
		t.Fatalf("submits = %d, want 1", port.submits)
	}
	_, err := router.Submit(context.Background(), automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "stripe"}, instruction)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
