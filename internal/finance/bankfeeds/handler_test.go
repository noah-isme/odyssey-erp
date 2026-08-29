package bankfeeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestWebhookHandlerRejectsMissingServiceWithoutPanicking(t *testing.T) {
	handler := NewHandler(nil, nil)
	router := chi.NewRouter()
	handler.MountRoutes(router)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/fake?connection_id=9", strings.NewReader(`{"type":"transaction.updated"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestWebhookHandlerRejectsMissingEnqueuerBeforePersisting(t *testing.T) {
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "fake", Status: "ACTIVE"}}
	service := NewService(repo, &bankingImportFake{}, map[string]FeedPort{"fake": fakeFeed{}})
	handler := NewHandler(service, nil)
	router := chi.NewRouter()
	handler.MountRoutes(router)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/fake?connection_id=9", strings.NewReader(`{"type":"transaction.updated"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if repo.event.ID != 0 {
		t.Fatalf("event persisted without an enqueuer: %+v", repo.event)
	}
}

func TestWebhookHandlerSignalsQueueFailureAfterDurableSave(t *testing.T) {
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "fake", Status: "ACTIVE"}}
	service := NewService(repo, &bankingImportFake{}, map[string]FeedPort{"fake": fakeFeed{}})
	handler := NewHandler(service, nil)
	handler.SetEventEnqueuer(func(context.Context, int64) error { return errors.New("redis unavailable") })
	router := chi.NewRouter()
	handler.MountRoutes(router)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/fake?connection_id=9", strings.NewReader(`{"type":"transaction.updated"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if repo.event.ID == 0 || repo.event.Status != "PENDING" {
		t.Fatalf("event = %+v, want durable pending event for retry", repo.event)
	}
}
