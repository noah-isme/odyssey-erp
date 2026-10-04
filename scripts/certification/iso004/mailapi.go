package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Mail sink snapshots (mail-before.json / mail-after.json). The recorder only
// issues GET requests against the Mailpit or MailHog HTTP API that the email
// gate already identified; it never deletes or modifies messages.

const (
	mailSearchLimit = 500
	mailBodyLimit   = 64 << 20
)

// MailMessage is the normalized view of one sink message.
type MailMessage struct {
	ID          string   `json:"id"`
	MessageID   string   `json:"message_id,omitempty"`
	From        string   `json:"from,omitempty"`
	To          []string `json:"to"`
	Subject     string   `json:"subject"`
	Created     string   `json:"created,omitempty"`
	Attachments int      `json:"attachments"`
}

// MailQuery is one API search for one recipient: the raw response plus the
// normalized messages addressed exactly to that recipient.
type MailQuery struct {
	Recipient string          `json:"recipient"`
	URL       string          `json:"url"`
	Status    int             `json:"status"`
	Total     int             `json:"total"`
	Returned  int             `json:"returned"`
	Messages  []MailMessage   `json:"messages"`
	Raw       json.RawMessage `json:"raw_response,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// MailSnapshot is written as mail-before.json / mail-after.json.
type MailSnapshot struct {
	Label            string      `json:"label"`
	Sink             string      `json:"sink"`
	API              string      `json:"api"`
	FetchedUTC       time.Time   `json:"fetched_utc"`
	AfterConvergence bool        `json:"after_convergence"`
	Queries          []MailQuery `json:"queries"`
}

// MailRecorder snapshots the sink for the given recipients.
type MailRecorder interface {
	Snapshot(ctx context.Context, recipients []string) (*MailSnapshot, error)
}

// mailAPIRecorder queries Mailpit (/api/v1/search) or MailHog
// (/api/v2/search). Sink is the API kind the email gate recognized.
type mailAPIRecorder struct {
	Base string
	Sink string
	HTTP *http.Client
	Now  func() time.Time
}

func (m *mailAPIRecorder) Snapshot(ctx context.Context, recipients []string) (*MailSnapshot, error) {
	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	snap := &MailSnapshot{Sink: m.Sink, API: strings.TrimRight(m.Base, "/"), FetchedUTC: now().UTC(), Queries: []MailQuery{}}
	var errs []error
	for _, r := range recipients {
		q := m.query(ctx, r)
		if q.Error != "" {
			errs = append(errs, fmt.Errorf("%s: %s", r, q.Error))
		}
		snap.Queries = append(snap.Queries, q)
	}
	return snap, errors.Join(errs...)
}

func (m *mailAPIRecorder) searchURL(recipient string) (string, error) {
	base := strings.TrimRight(m.Base, "/")
	v := url.Values{}
	switch m.Sink {
	case "mailpit":
		v.Set("query", fmt.Sprintf("to:%q", recipient))
		v.Set("limit", fmt.Sprint(mailSearchLimit))
		return base + "/api/v1/search?" + v.Encode(), nil
	case "mailhog":
		v.Set("kind", "to")
		v.Set("query", recipient)
		v.Set("limit", fmt.Sprint(mailSearchLimit))
		return base + "/api/v2/search?" + v.Encode(), nil
	}
	return "", fmt.Errorf("unknown mail sink %q", m.Sink)
}

func (m *mailAPIRecorder) query(ctx context.Context, recipient string) MailQuery {
	q := MailQuery{Recipient: recipient, Messages: []MailMessage{}}
	u, err := m.searchURL(recipient)
	if err != nil {
		q.Error = err.Error()
		return q
	}
	q.URL = u
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		q.Error = err.Error()
		return q
	}
	req.Header.Set("Accept", "application/json")
	resp, err := m.HTTP.Do(req)
	if err != nil {
		q.Error = err.Error()
		return q
	}
	defer resp.Body.Close()
	q.Status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, mailBodyLimit))
	if err != nil {
		q.Error = "read body: " + err.Error()
		return q
	}
	if json.Valid(body) {
		q.Raw = body
	}
	if resp.StatusCode != http.StatusOK {
		q.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return q
	}
	var msgs []MailMessage
	switch m.Sink {
	case "mailpit":
		msgs, q.Total, q.Returned, err = parseMailpitSearch(body)
	case "mailhog":
		msgs, q.Total, q.Returned, err = parseMailhogSearch(body)
	}
	if err != nil {
		q.Error = err.Error()
		return q
	}
	if q.Total > q.Returned {
		q.Error = fmt.Sprintf("search returned %d of %d messages; snapshot is incomplete", q.Returned, q.Total)
	}
	for _, msg := range msgs {
		if addressedTo(msg.To, recipient) {
			q.Messages = append(q.Messages, msg)
		}
	}
	return q
}

func addressedTo(to []string, recipient string) bool {
	for _, a := range to {
		if strings.EqualFold(strings.TrimSpace(a), recipient) {
			return true
		}
	}
	return false
}

// parseMailpitSearch reads a Mailpit /api/v1/search response
// (messages[].{ID,MessageID,From,To,Cc,Bcc,Subject,Created,Attachments}).
func parseMailpitSearch(body []byte) ([]MailMessage, int, int, error) {
	type addr struct {
		Name    string `json:"Name"`
		Address string `json:"Address"`
	}
	// "total" is the whole mailbox; "messages_count" is the number of
	// messages matching the search (absent in old Mailpit versions).
	var r struct {
		MessagesCount *int `json:"messages_count"`
		Messages      []struct {
			ID          string `json:"ID"`
			MessageID   string `json:"MessageID"`
			From        *addr  `json:"From"`
			To          []addr `json:"To"`
			Cc          []addr `json:"Cc"`
			Bcc         []addr `json:"Bcc"`
			Subject     string `json:"Subject"`
			Created     string `json:"Created"`
			Attachments int    `json:"Attachments"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, 0, fmt.Errorf("parse Mailpit search response: %w", err)
	}
	out := make([]MailMessage, 0, len(r.Messages))
	for _, m := range r.Messages {
		mm := MailMessage{ID: m.ID, MessageID: m.MessageID, Subject: m.Subject, Created: m.Created, Attachments: m.Attachments, To: []string{}}
		if m.From != nil {
			mm.From = m.From.Address
		}
		for _, group := range [][]addr{m.To, m.Cc, m.Bcc} {
			for _, a := range group {
				mm.To = append(mm.To, a.Address)
			}
		}
		out = append(out, mm)
	}
	total := len(r.Messages)
	if r.MessagesCount != nil {
		total = *r.MessagesCount
	}
	return out, total, len(r.Messages), nil
}

// parseMailhogSearch reads a MailHog /api/v2/search response
// (items[].{ID,From,To,Content.Headers,Created,MIME.Parts}).
func parseMailhogSearch(body []byte) ([]MailMessage, int, int, error) {
	type path struct {
		Mailbox string `json:"Mailbox"`
		Domain  string `json:"Domain"`
	}
	type part struct {
		Headers map[string][]string `json:"Headers"`
	}
	var r struct {
		Total int `json:"total"`
		Count int `json:"count"`
		Items []struct {
			ID      string `json:"ID"`
			From    *path  `json:"From"`
			To      []path `json:"To"`
			Created string `json:"Created"`
			Content struct {
				Headers map[string][]string `json:"Headers"`
			} `json:"Content"`
			MIME *struct {
				Parts []part `json:"Parts"`
			} `json:"MIME"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, 0, fmt.Errorf("parse MailHog search response: %w", err)
	}
	first := func(h map[string][]string, k string) string {
		if v := h[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	out := make([]MailMessage, 0, len(r.Items))
	for _, it := range r.Items {
		mm := MailMessage{ID: it.ID, MessageID: first(it.Content.Headers, "Message-ID"), Created: it.Created, To: []string{}}
		subj := first(it.Content.Headers, "Subject")
		if dec, err := new(mime.WordDecoder).DecodeHeader(subj); err == nil {
			subj = dec
		}
		mm.Subject = subj
		if it.From != nil {
			mm.From = it.From.Mailbox + "@" + it.From.Domain
		}
		for _, p := range it.To {
			mm.To = append(mm.To, p.Mailbox+"@"+p.Domain)
		}
		if it.MIME != nil {
			for _, p := range it.MIME.Parts {
				if strings.HasPrefix(strings.ToLower(first(p.Headers, "Content-Disposition")), "attachment") {
					mm.Attachments++
				}
			}
		}
		out = append(out, mm)
	}
	return out, r.Total, len(r.Items), nil
}
