package main

import (
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// wireAuditLoggers attaches the shared audit logger to the services whose
// writes the certification checks in audit_logs (ar_invoice, ap_invoice and
// the cmms_* entities). The logger implements RecordTx, so AR and AP record
// their posting audit rows inside the posting transaction.
func wireAuditLoggers(audit *shared.AuditLogger, arService *ar.Service, apService *ap.Service, cmmsService *cmms.Service) {
	arService.SetAuditLogger(audit)
	apService.SetAuditLogger(audit)
	cmmsService.SetAuditLogger(audit)
}
