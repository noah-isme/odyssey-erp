//go:build prod

package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGotenbergPDFClientRetriesOnBadGateway(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		attempt := atomic.AddInt32(&calls, 1)
		if attempt == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		payload := make([]byte, 2048)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)

	client, err := NewPDFRenderClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	impl := client.(*gotenbergPDFClient)
	impl.httpClient = server.Client()
	impl.httpClient.Timeout = impl.timeout

	pdf, err := client.RenderHTML(context.Background(), "<html></html>")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if len(pdf) != 2048 {
		t.Fatalf("expected 2048 bytes, got %d", len(pdf))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
}

func TestGotenbergPDFClientFailsWhenTooSmall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("small"))
	}))
	t.Cleanup(server.Close)

	client, err := NewPDFRenderClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	impl := client.(*gotenbergPDFClient)
	impl.httpClient = server.Client()
	impl.httpClient.Timeout = impl.timeout
	impl.retries = 1

	_, err = client.RenderHTML(context.Background(), "<html></html>")
	if !errors.Is(err, ErrPDFTooSmall) {
		t.Fatalf("expected ErrPDFTooSmall, got %v", err)
	}
}

func TestGotenbergPDFClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 2048))
	}))
	t.Cleanup(server.Close)

	client, err := NewPDFRenderClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	impl := client.(*gotenbergPDFClient)
	impl.httpClient = server.Client()
	impl.timeout = 10 * time.Millisecond
	impl.httpClient.Timeout = impl.timeout
	impl.retries = 0

	_, err = client.RenderHTML(context.Background(), "<html></html>")
	if !errors.Is(err, ErrPDFTimeout) {
		t.Fatalf("expected ErrPDFTimeout, got %v", err)
	}
}
