package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
)

type apWorkerServiceFake struct {
	accounting procurement.IntegrationHandler
	tax        ap.TaxServicePort
	fx         ap.FXRateResolver
}

func (f *apWorkerServiceFake) SetIntegrationHandler(handler procurement.IntegrationHandler) {
	f.accounting = handler
}

func (f *apWorkerServiceFake) SetTaxService(service ap.TaxServicePort) {
	f.tax = service
}

func (f *apWorkerServiceFake) SetFXResolver(resolver ap.FXRateResolver) {
	f.fx = resolver
}

type apWorkerAccountingFake struct{}

func (apWorkerAccountingFake) HandleGRNPosted(context.Context, procurement.GRNPostedEvent) error {
	return nil
}

func (apWorkerAccountingFake) HandleAPInvoicePosted(context.Context, procurement.APInvoicePostedEvent) error {
	return nil
}

func (apWorkerAccountingFake) HandleAPPaymentPosted(context.Context, procurement.APPaymentPostedEvent) error {
	return nil
}

func (apWorkerAccountingFake) HandleGoodsReturnConfirmed(context.Context, procurement.GoodsReturnConfirmedEvent) error {
	return nil
}

func (apWorkerAccountingFake) HandleDebitNotePosted(context.Context, procurement.DebitNotePostedEvent) error {
	return nil
}

type apWorkerTaxFake struct{}

func (apWorkerTaxFake) RecordAPInvoice(context.Context, int64, int64) error { return nil }
func (apWorkerTaxFake) RecordAPDebitNote(context.Context, int64, int64) error {
	return nil
}
func (apWorkerTaxFake) RecordAPPayment(context.Context, int64, int64) error { return nil }
func (apWorkerTaxFake) CancelAPInvoice(context.Context, int64, int64, string) error {
	return nil
}

type apWorkerFXFake struct{}

func (apWorkerFXFake) Resolve(context.Context, string, string, time.Time) (fx.FXQuote, error) {
	return fx.FXQuote{}, nil
}

func TestConfigureAPWorkerServiceWiresAccountingBoundaries(t *testing.T) {
	target := &apWorkerServiceFake{}
	accounting := apWorkerAccountingFake{}
	taxService := apWorkerTaxFake{}
	fxResolver := apWorkerFXFake{}

	if err := configureAPWorkerService(target, accounting, taxService, fxResolver); err != nil {
		t.Fatalf("configureAPWorkerService() error = %v", err)
	}
	if target.accounting == nil || target.tax == nil || target.fx == nil {
		t.Fatalf("configured dependencies = %#v, want accounting, tax, and FX ports", target)
	}
}

func TestConfigureAPWorkerServiceFailsClosedWhenBoundaryMissing(t *testing.T) {
	accounting := apWorkerAccountingFake{}
	taxService := apWorkerTaxFake{}
	fxResolver := apWorkerFXFake{}
	tests := []struct {
		name        string
		service     apWorkerServiceConfigurator
		accounting  procurement.IntegrationHandler
		taxService  ap.TaxServicePort
		fxResolver  ap.FXRateResolver
		wantMessage string
	}{
		{name: "service", wantMessage: "AP service"},
		{name: "accounting", service: &apWorkerServiceFake{}, taxService: taxService, fxResolver: fxResolver, wantMessage: "accounting integration"},
		{name: "tax", service: &apWorkerServiceFake{}, accounting: accounting, fxResolver: fxResolver, wantMessage: "tax service"},
		{name: "fx", service: &apWorkerServiceFake{}, accounting: accounting, taxService: taxService, wantMessage: "FX resolver"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := configureAPWorkerService(tt.service, tt.accounting, tt.taxService, tt.fxResolver); err == nil {
				t.Fatal("configureAPWorkerService() error = nil, want failure")
			} else if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("configureAPWorkerService() error = %q, want %q", err, tt.wantMessage)
			}
		})
	}
}

func TestCashForecastScheduleEnabledOnlyForFinanceProfiles(t *testing.T) {
	tests := []struct {
		profile string
		want    bool
	}{
		{profile: "v0.11-finance", want: true},
		{profile: "full", want: true},
		{profile: "v0.10-core", want: false},
		{profile: "", want: false},
		{profile: "not-a-profile", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			if got := cashForecastScheduleEnabled(tt.profile); got != tt.want {
				t.Fatalf("cashForecastScheduleEnabled(%q) = %v, want %v", tt.profile, got, tt.want)
			}
		})
	}
}
