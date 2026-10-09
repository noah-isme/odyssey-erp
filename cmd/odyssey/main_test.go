package main

import (
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestWireAuditLoggers(t *testing.T) {
	arService := ar.NewService(ar.NewRepository(nil))
	apService := ap.NewService(ap.NewRepository(nil), nil)
	cmmsService := cmms.NewService(cmms.NewRepository(nil))

	wireAuditLoggers(shared.NewAuditLogger(nil), arService, apService, cmmsService)

	tests := []struct {
		name    string
		service interface{ AuditLoggerConfigured() bool }
	}{
		{name: "ar", service: arService},
		{name: "ap", service: apService},
		{name: "cmms", service: cmmsService},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.service.AuditLoggerConfigured() {
				t.Fatalf("%s service has no audit logger after wiring", tc.name)
			}
		})
	}
}
