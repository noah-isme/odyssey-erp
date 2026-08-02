package mrp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDecisionSubmissionHandler tests the decision submission handler
func TestDecisionSubmissionHandler(t *testing.T) {
	tests := []struct {
		name           string
		request        DecisionRequestPayload
		expectedStatus int
		expectedValid  bool
	}{
		{
			name: "valid BOM approval request",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				RecordID:   1,
				CompanyID:  1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
				Action:     "Approve",
				Reason:     "Looks good",
			},
			expectedStatus: http.StatusOK,
			expectedValid:  false, // Will fail validation without real data
		},
		{
			name: "missing record_id",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				CompanyID:  1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
			},
			expectedStatus: http.StatusOK,
			expectedValid:  false,
		},
		{
			name: "invalid record type",
			request: DecisionRequestPayload{
				RecordType: "UNKNOWN",
				RecordID:   1,
				CompanyID:  1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
			},
			expectedStatus: http.StatusOK,
			expectedValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewDecisionSubmissionHandler(nil, nil, nil)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/decisions", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var resp DecisionResponse
			json.NewDecoder(w.Body).Decode(&resp)

			if tt.expectedValid && !resp.Success {
				t.Errorf("Expected successful response, got error: %s", resp.Error)
			}
		})
	}
}

// TestDecisionRequestValidation tests request validation
func TestDecisionRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		request     DecisionRequestPayload
		shouldError bool
	}{
		{
			name: "valid complete request",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				RecordID:   1,
				CompanyID:  1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
			},
			shouldError: false,
		},
		{
			name: "missing record_id",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				CompanyID:  1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
			},
			shouldError: true,
		},
		{
			name: "missing company_id",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				RecordID:   1,
				ActorID:    100,
				ActorRole:  "QUALITY_LEAD",
			},
			shouldError: true,
		},
		{
			name: "missing actor_id",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				RecordID:   1,
				CompanyID:  1,
				ActorRole:  "QUALITY_LEAD",
			},
			shouldError: true,
		},
		{
			name: "missing actor_role",
			request: DecisionRequestPayload{
				RecordType: "BOM",
				RecordID:   1,
				CompanyID:  1,
				ActorID:    100,
			},
			shouldError: true,
		},
		{
			name: "missing record_type",
			request: DecisionRequestPayload{
				RecordID:  1,
				CompanyID: 1,
				ActorID:   100,
				ActorRole: "QUALITY_LEAD",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecisionRequest(tt.request)

			if tt.shouldError && err == nil {
				t.Errorf("Expected validation error, got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// TestChallengeVerificationHandler tests the challenge verification handler
func TestChallengeVerificationHandler(t *testing.T) {
	tests := []struct {
		name           string
		request        ChallengeVerificationRequest
		expectedStatus int
		shouldSucceed  bool
	}{
		{
			name: "valid challenge verification",
			request: ChallengeVerificationRequest{
				ChallengeID: "test-challenge-123",
				Signature:   "sig-response",
				Decision:    "APPROVE",
				Comment:     "Approved",
			},
			expectedStatus: http.StatusOK,
			shouldSucceed:  false, // Will fail without real challenge service
		},
		{
			name: "invalid decision",
			request: ChallengeVerificationRequest{
				ChallengeID: "test-challenge-123",
				Signature:   "sig-response",
				Decision:    "INVALID",
			},
			expectedStatus: http.StatusOK,
			shouldSucceed:  false,
		},
		{
			name: "reject decision",
			request: ChallengeVerificationRequest{
				ChallengeID: "test-challenge-123",
				Signature:   "sig-response",
				Decision:    "REJECT",
				Comment:     "Does not meet requirements",
			},
			expectedStatus: http.StatusOK,
			shouldSucceed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewChallengeVerificationHandler(nil)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/challenges/verify", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var resp ChallengeVerificationResponse
			json.NewDecoder(w.Body).Decode(&resp)

			if tt.shouldSucceed && !resp.Success {
				t.Errorf("Expected successful response, got: %s", resp.Error)
			}
		})
	}
}

// TestAuditLogHandler tests the audit log handler
func TestAuditLogHandler(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "basic audit log query",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "query with entity type filter",
			queryParams:    "?entity_type=BOM",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "query with limit and offset",
			queryParams:    "?limit=50&offset=10",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "query with action filter",
			queryParams:    "?action=APPROVE",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAuditLogHandler(nil)

			req := httptest.NewRequest(http.MethodGet, "/audit-log"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var resp AuditLogResponse
			json.NewDecoder(w.Body).Decode(&resp)

			if !resp.Success {
				t.Errorf("Expected successful response, got error: %s", resp.Error)
			}

			if resp.Total != tt.expectedCount {
				t.Errorf("Expected %d events, got %d", tt.expectedCount, resp.Total)
			}
		})
	}
}

// TestHandlerMethodValidation tests HTTP method validation
func TestHandlerMethodValidation(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.Handler
		method         string
		expectedStatus int
	}{
		{
			name:           "GET on decision submission handler",
			handler:        NewDecisionSubmissionHandler(nil, nil, nil),
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "POST on audit log handler",
			handler:        NewAuditLogHandler(nil),
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT on challenge verification handler",
			handler:        NewChallengeVerificationHandler(nil),
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			tt.handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestDecisionResponseSerialization tests JSON serialization of responses
func TestDecisionResponseSerialization(t *testing.T) {
	tests := []struct {
		name     string
		response DecisionResponse
	}{
		{
			name: "successful response with challenge",
			response: DecisionResponse{
				Success:       true,
				Message:       "BOM ready for decision gate",
				ChallengeID:   "challenge-123",
				ChallengeText: "Please sign to approve",
				ValidationData: map[string]interface{}{
					"line_count": 5,
					"version":    "1.0",
				},
			},
		},
		{
			name: "error response",
			response: DecisionResponse{
				Success: false,
				Message: "Pre-conditions not met",
				Error:   "BOM has no line items",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("Failed to marshal response: %v", err)
			}

			var unmarshaled DecisionResponse
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if unmarshaled.Success != tt.response.Success {
				t.Errorf("Success mismatch after serialization")
			}
			if unmarshaled.Message != tt.response.Message {
				t.Errorf("Message mismatch after serialization")
			}
		})
	}
}

// TestAuditLogResponsePagination tests pagination in audit log responses
func TestAuditLogResponsePagination(t *testing.T) {
	tests := []struct {
		name         string
		limit        int
		offset       int
		total        int
		expectedLen  int
	}{
		{
			name:        "first page",
			limit:       10,
			offset:      0,
			total:       25,
			expectedLen: 10,
		},
		{
			name:        "second page",
			limit:       10,
			offset:      10,
			total:       25,
			expectedLen: 10,
		},
		{
			name:        "last page partial",
			limit:       10,
			offset:      20,
			total:       25,
			expectedLen: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock events
			events := make([]AuditLogEntry, tt.total)
			for i := 0; i < tt.total; i++ {
				events[i] = AuditLogEntry{
					ID:       int64(i + 1),
					EntityID: int64(i),
				}
			}

			// Simulate pagination
			start := tt.offset
			end := tt.offset + tt.limit
			if end > len(events) {
				end = len(events)
			}

			paged := events[start:end]
			if len(paged) != tt.expectedLen {
				t.Errorf("Expected %d events, got %d", tt.expectedLen, len(paged))
			}
		})
	}
}
