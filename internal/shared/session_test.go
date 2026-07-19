package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestFlashPersistsAcrossRedirectAndClearsAfterRead(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	manager := NewSessionManager(client, "odyssey_session", "test-secret", time.Hour, false)
	ctx := context.Background()
	request := httptest.NewRequest(http.MethodPost, "/settings", nil)
	session, err := manager.Load(ctx, request)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AddFlash(FlashMessage{Kind: "success", Message: "Pengaturan berhasil disimpan"})
	response := httptest.NewRecorder()
	if err := manager.Commit(ctx, response, request, session); err != nil {
		t.Fatalf("commit redirect session: %v", err)
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/settings", nil)
	nextRequest.AddCookie(response.Result().Cookies()[0])
	nextSession, err := manager.Load(ctx, nextRequest)
	if err != nil {
		t.Fatalf("load redirected session: %v", err)
	}
	flash := nextSession.PopFlash()
	if flash == nil || flash.Message != "Pengaturan berhasil disimpan" {
		t.Fatalf("expected flash after redirect, got %#v", flash)
	}

	readResponse := httptest.NewRecorder()
	if err := manager.Commit(ctx, readResponse, nextRequest, nextSession); err != nil {
		t.Fatalf("commit read session: %v", err)
	}
	finalRequest := httptest.NewRequest(http.MethodGet, "/settings", nil)
	finalRequest.AddCookie(readResponse.Result().Cookies()[0])
	finalSession, err := manager.Load(ctx, finalRequest)
	if err != nil {
		t.Fatalf("load final session: %v", err)
	}
	if flash := finalSession.PopFlash(); flash != nil {
		t.Fatalf("expected flash to be cleared after read, got %#v", flash)
	}
}
