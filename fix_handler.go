package main

import (
	"os"
	"strings"
)

func main() {
	// Fix CMMS Outbox
	cmmsContent, _ := os.ReadFile("internal/cmms/outbox.go")
	cmmsStr := string(cmmsContent)
	cmmsStr = strings.Replace(cmmsStr, "type CalibrationRequestedPayload struct {\n\tFindingID int64 `json:\"finding_id\"`\n\tAssetID   int64 `json:\"asset_id\"`\n}", "type CalibrationRequestedPayload struct {\n\tFindingID int64 `json:\"finding_id\"`\n\tAssetID   int64 `json:\"asset_id\"`\n\tActorID   int64 `json:\"actor_id\"`\n}", 1)
	os.WriteFile("internal/cmms/outbox.go", []byte(cmmsStr), 0644)

	// Fix QMS Handler
	qmsContent, _ := os.ReadFile("internal/qms/http/handler.go")
	qmsStr := string(qmsContent)
	
	qmsStr = strings.Replace(qmsStr, "type CalibrationRequestedPayload struct {\n\tFindingID int64 `json:\"finding_id\"`\n\tAssetID   int64 `json:\"asset_id\"`\n}", "type CalibrationRequestedPayload struct {\n\tFindingID int64 `json:\"finding_id\"`\n\tAssetID   int64 `json:\"asset_id\"`\n\tActorID   int64 `json:\"actor_id\"`\n}", 1)

	oldBody := `session := shared.SessionFromContext(ctx)
	
	payload, _ := json.Marshal(CalibrationRequestedPayload{
		FindingID: findingID,
		AssetID:   assetID,
	})
	
	err = h.outbox.Publish(ctx, outbox.Event{
		CompanyID:     session.CompanyID,
		AggregateType: "qms_audit_finding",
		AggregateID:   findingID,
		EventType:     "qms.calibration.required",
		Payload:       payload,
	})`
	
	newBody := `session := shared.SessionFromContext(ctx)
	companyID := currentCompany(r)
	actorID := parseInt64(session.User())
	
	_, err = h.outbox.InsertEvent(ctx, h.outbox.Queries(), outbox.PublishRequest{
		CompanyID:     companyID,
		CorrelationID: uuid.New(),
		EventType:     "qms.calibration.required",
		AggregateType: "qms_audit_finding",
		AggregateID:   findingID,
		Payload:       CalibrationRequestedPayload{
			FindingID: findingID,
			AssetID:   assetID,
			ActorID:   actorID,
		},
	})`
	
	qmsStr = strings.Replace(qmsStr, oldBody, newBody, 1)
	
	// Add uuid import
	if !strings.Contains(qmsStr, "\"github.com/google/uuid\"") {
		qmsStr = strings.Replace(qmsStr, "\"github.com/go-chi/chi/v5\"", "\"github.com/google/uuid\"\n\t\"github.com/go-chi/chi/v5\"", 1)
	}

	os.WriteFile("internal/qms/http/handler.go", []byte(qmsStr), 0644)
}
