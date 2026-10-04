package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mailpitSearchBody = `{"total":7,"unread":2,"count":2,"messages_count":2,"start":0,"tags":[],"messages":[
 {"ID":"a1","MessageID":"m1@odyssey","Read":false,"From":{"Name":"","Address":"no-reply@odyssey.local"},
  "To":[{"Name":"","Address":"iso004-payslip-9000000001@staging.invalid"}],"Cc":[],"Bcc":[],
  "Subject":"Payslip","Created":"2026-10-04T05:00:00Z","Attachments":1,"Size":2048},
 {"ID":"a2","MessageID":"m2@odyssey","From":{"Address":"no-reply@odyssey.local"},
  "To":[{"Address":"iso004-payslip-9000000001@staging.invalid.other"}],"Subject":"Other","Created":"2026-10-04T05:01:00Z","Attachments":0}
]}`

const mailhogSearchBody = `{"total":1,"count":1,"start":0,"items":[
 {"ID":"h1","From":{"Mailbox":"no-reply","Domain":"odyssey.local"},"To":[{"Mailbox":"iso004-9000000001","Domain":"staging.invalid"}],
  "Content":{"Headers":{"Subject":["ISO-004 9000000001"],"Message-ID":["<h1@odyssey>"]}},"Created":"2026-10-04T05:00:00Z",
  "MIME":{"Parts":[{"Headers":{"Content-Type":["text/html"]}},{"Headers":{"Content-Disposition":["attachment; filename=\"p.pdf\""]}}]}}
]}`

func TestMailpitRecorder(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query().Get("query")
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mailpitSearchBody))
	}))
	t.Cleanup(srv.Close)

	rec := &mailAPIRecorder{Base: srv.URL + "/", Sink: "mailpit", HTTP: srv.Client(), Now: func() time.Time { return time.Date(2026, 10, 4, 6, 0, 0, 0, time.UTC) }}
	snap, err := rec.Snapshot(context.Background(), []string{"iso004-payslip-9000000001@staging.invalid"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/search", gotPath)
	assert.Equal(t, `to:"iso004-payslip-9000000001@staging.invalid"`, gotQuery)
	assert.Equal(t, "mailpit", snap.Sink)
	assert.Equal(t, srv.URL, snap.API)
	require.Len(t, snap.Queries, 1)
	q := snap.Queries[0]
	assert.Equal(t, 200, q.Status)
	assert.Equal(t, 2, q.Total)
	assert.Equal(t, 2, q.Returned)
	assert.JSONEq(t, mailpitSearchBody, string(q.Raw), "the raw API response is kept")
	require.Len(t, q.Messages, 1, "only messages addressed exactly to the recipient")
	assert.Equal(t, MailMessage{ID: "a1", MessageID: "m1@odyssey", From: "no-reply@odyssey.local", To: []string{"iso004-payslip-9000000001@staging.invalid"}, Subject: "Payslip", Created: "2026-10-04T05:00:00Z", Attachments: 1}, q.Messages[0])
}

func TestMailhogRecorder(t *testing.T) {
	var gotPath, gotKind string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKind = r.URL.Path, r.URL.Query().Get("kind")
		_, _ = w.Write([]byte(mailhogSearchBody))
	}))
	t.Cleanup(srv.Close)
	rec := &mailAPIRecorder{Base: srv.URL, Sink: "mailhog", HTTP: srv.Client()}
	snap, err := rec.Snapshot(context.Background(), []string{"ISO004-9000000001@staging.invalid"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v2/search", gotPath)
	assert.Equal(t, "to", gotKind)
	q := snap.Queries[0]
	require.Len(t, q.Messages, 1)
	assert.Equal(t, MailMessage{ID: "h1", MessageID: "<h1@odyssey>", From: "no-reply@odyssey.local", To: []string{"iso004-9000000001@staging.invalid"}, Subject: "ISO-004 9000000001", Created: "2026-10-04T05:00:00Z", Attachments: 1}, q.Messages[0])
}

func TestMailRecorderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == `to:"broken@staging.invalid"` {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"messages_count":900,"messages":[]}`))
	}))
	t.Cleanup(srv.Close)
	rec := &mailAPIRecorder{Base: srv.URL, Sink: "mailpit", HTTP: srv.Client()}
	snap, err := rec.Snapshot(context.Background(), []string{"broken@staging.invalid", "many@staging.invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
	assert.Contains(t, err.Error(), "incomplete")
	require.Len(t, snap.Queries, 2, "every query is recorded even when one fails")
	assert.JSONEq(t, `{"error":"boom"}`, string(snap.Queries[0].Raw))

	_, err = (&mailAPIRecorder{Base: srv.URL, Sink: "smtp4dev", HTTP: srv.Client()}).Snapshot(context.Background(), []string{"x@y"})
	assert.ErrorContains(t, err, "unknown mail sink")
}
