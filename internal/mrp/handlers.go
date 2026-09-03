package mrp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// DecisionSubmissionHandler handles incoming governance decision requests
type DecisionSubmissionHandler struct {
	bomValidator *BOMApprovalValidator
	woValidator  *WorkOrderReleaseValidator
	repo         *SQLRepository
}

// NewDecisionSubmissionHandler creates a new decision submission handler
func NewDecisionSubmissionHandler(
	bomVal *BOMApprovalValidator,
	woVal *WorkOrderReleaseValidator,
	repo *SQLRepository,
) *DecisionSubmissionHandler {
	return &DecisionSubmissionHandler{
		bomValidator: bomVal,
		woValidator:  woVal,
		repo:         repo,
	}
}

// DecisionRequestPayload represents an incoming governance decision HTTP request
type DecisionRequestPayload struct {
	RecordType string                 `json:"record_type"` // BOM, WorkOrder, Operation, etc.
	RecordID   int64                  `json:"record_id"`
	CompanyID  int64                  `json:"company_id"`
	ActorID    int64                  `json:"actor_id"`
	ActorRole  string                 `json:"actor_role"` // QUALITY_LEAD, ENGINEERING, etc.
	Action     string                 `json:"action"`     // Approve, Release, Complete, etc.
	Reason     string                 `json:"reason"`
	Evidence   map[string]interface{} `json:"evidence"`
}

// DecisionResponse represents the response to a decision request
type DecisionResponse struct {
	Success        bool                   `json:"success"`
	Message        string                 `json:"message"`
	ChallengeID    string                 `json:"challenge_id,omitempty"`
	ChallengeText  string                 `json:"challenge_text,omitempty"`
	ValidationData map[string]interface{} `json:"validation_data,omitempty"`
	Error          string                 `json:"error,omitempty"`
}

// ServeHTTP handles POST /decisions requests
func (h *DecisionSubmissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DecisionRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, DecisionResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	ctx := context.Background()
	resp := h.processDecision(ctx, req)
	respondJSON(w, http.StatusOK, resp)
}

// processDecision validates and initiates the decision gate
func (h *DecisionSubmissionHandler) processDecision(ctx context.Context, req DecisionRequestPayload) DecisionResponse {
	// Validate request
	if err := validateDecisionRequest(req); err != nil {
		return DecisionResponse{
			Success: false,
			Error:   shared.UserSafeMessage(err),
		}
	}

	// Route to appropriate validator
	var validator ValidatorResult
	switch req.RecordType {
	case "BOM":
		if h.bomValidator != nil {
			validator = h.bomValidator.Validate(ctx, req.CompanyID, req.RecordID)
		} else {
			validator = ValidatorResult{Valid: true, Reason: "No validator available", Data: make(map[string]interface{})}
		}
	case "WorkOrder":
		if h.woValidator != nil {
			validator = h.woValidator.Validate(ctx, req.CompanyID, req.RecordID)
		} else {
			validator = ValidatorResult{Valid: true, Reason: "No validator available", Data: make(map[string]interface{})}
		}
	default:
		return DecisionResponse{
			Success: false,
			Error:   fmt.Sprintf("Unsupported record type: %s", req.RecordType),
		}
	}

	// Check validation result
	if !validator.Valid {
		return DecisionResponse{
			Success:        false,
			Message:        "Pre-conditions not met",
			Error:          validator.Reason,
			ValidationData: validator.Data,
		}
	}

	// Generate signature challenge (would be done by ComplianceGate service in production)
	challengeID := fmt.Sprintf("challenge-%d-%d", req.RecordID, req.ActorID)

	return DecisionResponse{
		Success:        true,
		Message:        fmt.Sprintf("%s ready for decision gate", req.RecordType),
		ChallengeID:    challengeID,
		ChallengeText:  "Please sign to confirm this decision",
		ValidationData: validator.Data,
	}
}

// ChallengeVerificationHandler handles signature challenge verification
type ChallengeVerificationHandler struct {
	repo     *SQLRepository
	verifier ChallengeVerifier
}

// ChallengeVerifier is the transactional signature challenge boundary. The
// legacy repository constructor remains for compatibility, but verification
// is fail-closed until a real service is supplied.
type ChallengeVerifier interface {
	VerifyChallenge(context.Context, VerifyChallengeInput) (*VerifyChallengeResult, error)
}

// NewChallengeVerificationHandler creates a new challenge verification handler
func NewChallengeVerificationHandler(repo *SQLRepository) *ChallengeVerificationHandler {
	return &ChallengeVerificationHandler{
		repo: repo,
	}
}

func NewChallengeVerificationHandlerWithService(verifier ChallengeVerifier) *ChallengeVerificationHandler {
	return &ChallengeVerificationHandler{verifier: verifier}
}

// ChallengeVerificationRequest represents a signature verification request
type ChallengeVerificationRequest struct {
	ChallengeID   string                 `json:"challenge_id"`
	Signature     string                 `json:"signature"`
	Decision      string                 `json:"decision"` // APPROVE or REJECT
	Comment       string                 `json:"comment"`
	Evidence      map[string]interface{} `json:"evidence"`
	CompanyID     int64                  `json:"company_id"`
	RecordType    string                 `json:"record_type"`
	RecordID      int64                  `json:"record_id"`
	RecordVersion string                 `json:"record_version"`
	ActorID       int64                  `json:"actor_id"`
	ReauthToken   string                 `json:"reauth_token"`
}

// ChallengeVerificationResponse represents the verification result
type ChallengeVerificationResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	GateStatus string `json:"gate_status,omitempty"` // PENDING, APPROVED, REJECTED
	Error      string `json:"error,omitempty"`
}

// ServeHTTP handles POST /challenges/{id}/verify requests
func (h *ChallengeVerificationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChallengeVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, ChallengeVerificationResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	ctx := context.Background()
	resp := h.verifyChallengeAndDecide(ctx, req)
	respondJSON(w, http.StatusOK, resp)
}

// verifyChallengeAndDecide verifies the challenge and records the decision
func (h *ChallengeVerificationHandler) verifyChallengeAndDecide(ctx context.Context, req ChallengeVerificationRequest) ChallengeVerificationResponse {
	if h == nil || h.verifier == nil {
		return ChallengeVerificationResponse{Success: false, Error: "Challenge verification service unavailable"}
	}
	if req.ChallengeID == "" || req.RecordID <= 0 || req.ActorID <= 0 || req.ReauthToken == "" {
		return ChallengeVerificationResponse{
			Success: false,
			Error:   "Challenge ID, record, actor, and reauthentication are required",
		}
	}

	// Validate decision
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		return ChallengeVerificationResponse{
			Success: false,
			Error:   "Decision must be APPROVE or REJECT",
		}
	}

	result, err := h.verifier.VerifyChallenge(ctx, VerifyChallengeInput{
		ChallengeID:   req.ChallengeID,
		CompanyID:     req.CompanyID,
		RecordType:    req.RecordType,
		RecordID:      req.RecordID,
		RecordVersion: req.RecordVersion,
		ReauthToken:   req.ReauthToken,
		ActorID:       req.ActorID,
	})
	if err != nil || result == nil || !result.Valid {
		message := "Challenge verification failed"
		if result != nil && result.Message != "" {
			message = result.Message
		}
		return ChallengeVerificationResponse{Success: false, Error: message}
	}
	return ChallengeVerificationResponse{Success: true, Message: fmt.Sprintf("Decision recorded: %s", req.Decision), GateStatus: "SIGNED"}
}

// AuditLogHandler handles audit event queries
type AuditLogHandler struct {
	repo *SQLRepository
}

// NewAuditLogHandler creates a new audit log handler
func NewAuditLogHandler(repo *SQLRepository) *AuditLogHandler {
	return &AuditLogHandler{repo: repo}
}

// AuditLogQuery represents query parameters for audit events
type AuditLogQuery struct {
	EntityType string `json:"entity_type"` // Optional: filter by entity type
	EntityID   int64  `json:"entity_id"`   // Optional: filter by entity ID
	ActorID    int64  `json:"actor_id"`    // Optional: filter by actor
	Action     string `json:"action"`      // Optional: filter by action
	StartDate  string `json:"start_date"`  // ISO 8601 format
	EndDate    string `json:"end_date"`    // ISO 8601 format
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

// AuditLogEntry represents a single audit event
type AuditLogEntry struct {
	ID         int64                  `json:"id"`
	EntityID   int64                  `json:"entity_id"`
	EntityType string                 `json:"entity_type"`
	Action     string                 `json:"action"`
	ActorID    int64                  `json:"actor_id"`
	Details    map[string]interface{} `json:"details"`
	CreatedAt  time.Time              `json:"created_at"`
}

// AuditLogResponse represents the audit log query response
type AuditLogResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Events  []AuditLogEntry `json:"events"`
	Total   int             `json:"total"`
	Error   string          `json:"error,omitempty"`
}

// ServeHTTP handles GET /audit-log requests
func (h *AuditLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := AuditLogQuery{
		Limit:  100,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		_, _ = fmt.Sscanf(limitStr, "%d", &query.Limit)
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		_, _ = fmt.Sscanf(offsetStr, "%d", &query.Offset)
	}

	query.EntityType = r.URL.Query().Get("entity_type")
	query.Action = r.URL.Query().Get("action")
	query.StartDate = r.URL.Query().Get("start_date")
	query.EndDate = r.URL.Query().Get("end_date")

	ctx := context.Background()
	resp := h.queryAuditLog(ctx, query)
	respondJSON(w, http.StatusOK, resp)
}

// queryAuditLog retrieves audit events from the database
func (h *AuditLogHandler) queryAuditLog(ctx context.Context, query AuditLogQuery) AuditLogResponse {
	// Placeholder implementation - would query audit_events table
	// In production, would use sqlc-generated queries

	return AuditLogResponse{
		Success: true,
		Message: "Audit log retrieved",
		Events:  []AuditLogEntry{},
		Total:   0,
	}
}

// Helper functions

func validateDecisionRequest(req DecisionRequestPayload) error {
	if req.RecordID == 0 {
		return fmt.Errorf("record_id is required")
	}
	if req.CompanyID == 0 {
		return fmt.Errorf("company_id is required")
	}
	if req.ActorID == 0 {
		return fmt.Errorf("actor_id is required")
	}
	if req.ActorRole == "" {
		return fmt.Errorf("actor_role is required")
	}
	if req.RecordType == "" {
		return fmt.Errorf("record_type is required")
	}
	return nil
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
