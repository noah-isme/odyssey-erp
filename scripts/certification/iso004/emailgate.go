package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SMTPTarget is the SMTP endpoint the worker uses, with the source of each
// value. Only SMTP_HOST and SMTP_PORT are ever extracted from the worker
// environment; no other variable (secrets included) is retained.
type SMTPTarget struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	HostSource string `json:"host_source"`
	PortSource string `json:"port_source"`
}

// systemdShow is the parsed `systemctl show -p Environment -p EnvironmentFile`.
type systemdShow struct {
	Inline []string // Environment= assignments in order
	Files  []string // EnvironmentFile= paths in order
	// Optional marks files prefixed with "-" or shown with ignore_errors=yes.
	Optional map[string]bool
}

// parseSystemdShow parses the property output of
// `systemctl show -p Environment -p EnvironmentFile <unit>`. systemd prints
// Environment as one line of space-separated, optionally double-quoted
// assignments and one EnvironmentFile line per file, formatted as
// "path (ignore_errors=yes|no)".
func parseSystemdShow(out string) (systemdShow, error) {
	res := systemdShow{Optional: map[string]bool{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Environment":
			words, err := splitSystemdWords(val)
			if err != nil {
				return systemdShow{}, fmt.Errorf("parse Environment=: %w", err)
			}
			res.Inline = append(res.Inline, words...)
		case "EnvironmentFile":
			val = strings.TrimSpace(val)
			if val == "" {
				continue
			}
			optional := false
			if i := strings.LastIndex(val, " (ignore_errors="); i >= 0 && strings.HasSuffix(val, ")") {
				optional = strings.TrimSuffix(val[i+len(" (ignore_errors="):], ")") == "yes"
				val = val[:i]
			}
			if strings.HasPrefix(val, "-") {
				optional = true
				val = val[1:]
			}
			res.Files = append(res.Files, val)
			if optional {
				res.Optional[val] = true
			}
		}
	}
	return res, nil
}

// splitSystemdWords splits a systemd Environment= value: words separated by
// whitespace, double or single quotes group, backslash escapes the next rune.
func splitSystemdWords(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped, inWord = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

// parseEnvFile parses an EnvironmentFile/.env body and returns only the
// assignments for the requested keys, in file order (later wins).
// Comments start with '#' or ';'; an optional "export " prefix is accepted
// for .env files used in local rehearsal; matching outer quotes are removed.
// Like systemd, a '#' after a value is part of the value.
func parseEnvFile(r io.Reader, keys map[string]bool) ([][2]string, error) {
	var out [][2]string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	pending := ""
	for sc.Scan() {
		line := pending + sc.Text()
		pending = ""
		if strings.HasSuffix(line, "\\") {
			pending = strings.TrimSuffix(line, "\\")
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if !keys[k] {
			continue
		}
		out = append(out, [2]string{k, unquote(strings.TrimSpace(v))})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

var smtpKeys = map[string]bool{"SMTP_HOST": true, "SMTP_PORT": true}

// resolveSMTPTarget applies systemd's precedence: inline Environment= first,
// then every EnvironmentFile in listed order; a later definition overrides an
// earlier one (systemd.exec: "Settings from these files override settings
// made with Environment="). Unset values fall back to the application
// defaults from internal/app/config.go.
func resolveSMTPTarget(show systemdShow, readFile func(string) ([]byte, error)) (SMTPTarget, error) {
	t := SMTPTarget{Host: defaultSMTPHost, Port: defaultSMTPPort, HostSource: "default", PortSource: "default"}
	set := func(k, v, src string) {
		switch k {
		case "SMTP_HOST":
			t.Host, t.HostSource = v, src
		case "SMTP_PORT":
			t.Port, t.PortSource = v, src
		}
	}
	for _, kv := range show.Inline {
		k, v, ok := strings.Cut(kv, "=")
		if ok && smtpKeys[k] {
			set(k, v, "Environment=")
		}
	}
	for _, f := range show.Files {
		data, err := readFile(f)
		if err != nil {
			if show.Optional[f] && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return SMTPTarget{}, fmt.Errorf("read EnvironmentFile %s: %w", f, err)
		}
		kvs, err := parseEnvFile(strings.NewReader(string(data)), smtpKeys)
		if err != nil {
			return SMTPTarget{}, fmt.Errorf("parse EnvironmentFile %s: %w", f, err)
		}
		for _, kv := range kvs {
			set(kv[0], kv[1], "EnvironmentFile="+f)
		}
	}
	if p, err := strconv.Atoi(t.Port); err != nil || p < 1 || p > 65535 {
		return SMTPTarget{}, fmt.Errorf("SMTP_PORT %q (from %s) is not a valid port", t.Port, t.PortSource)
	}
	if strings.TrimSpace(t.Host) == "" {
		return SMTPTarget{}, fmt.Errorf("SMTP_HOST is empty (from %s)", t.HostSource)
	}
	return t, nil
}

// commandRunner runs an external command and returns its stdout.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// workerSystemdShow returns the worker unit's environment description. With
// --worker-env files the unit is not consulted (local rehearsal).
func workerSystemdShow(ctx context.Context, cfg *Config, run commandRunner) (systemdShow, string, error) {
	if len(cfg.WorkerEnv) > 0 {
		return systemdShow{Files: append([]string(nil), cfg.WorkerEnv...), Optional: map[string]bool{}}, "--worker-env", nil
	}
	out, err := run(ctx, "systemctl", "show", "-p", "Environment", "-p", "EnvironmentFile", cfg.WorkerUnit)
	if err != nil {
		return systemdShow{}, "", fmt.Errorf("systemctl show %s: %w", cfg.WorkerUnit, err)
	}
	show, err := parseSystemdShow(string(out))
	if err != nil {
		return systemdShow{}, "", err
	}
	return show, "systemctl show " + cfg.WorkerUnit, nil
}

// resolver looks up host addresses (net.DefaultResolver.LookupHost in
// production, a stub in tests).
type resolver func(ctx context.Context, host string) ([]string, error)

// isLoopbackHost accepts literal loopback IPs (127.0.0.0/8, ::1) and the name
// "localhost" only when every address it resolves to is loopback. Any other
// name (e.g. "mailpit") is rejected even if it resolves to loopback, because
// the worker host's resolver may differ from the operator's.
func isLoopbackHost(ctx context.Context, host string, lookup resolver) (bool, string) {
	h := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSuffix(host, "]"), "["), ".")
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() {
			return true, "literal loopback address"
		}
		return false, "literal non-loopback address"
	}
	if !strings.EqualFold(h, "localhost") {
		return false, "host name other than localhost is never accepted as loopback"
	}
	addrs, err := lookup(ctx, "localhost")
	if err != nil {
		return false, fmt.Sprintf("resolve localhost: %v", err)
	}
	if len(addrs) == 0 {
		return false, "localhost resolved to no addresses"
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return false, fmt.Sprintf("localhost resolves to non-loopback address %s", a)
		}
	}
	return true, "localhost resolves only to loopback " + strings.Join(addrs, ",")
}

// SinkProbe records what the SMTP banner and mail API revealed.
type SinkProbe struct {
	Banner      string `json:"banner,omitempty"`
	BannerSink  string `json:"banner_sink,omitempty"`
	APIEndpoint string `json:"api_endpoint,omitempty"`
	APISink     string `json:"api_sink,omitempty"`
	APIStatus   int    `json:"api_status,omitempty"`
}

// recognizeBanner returns "mailpit" or "mailhog" when the greeting is a 220
// reply naming one of them.
func recognizeBanner(banner string) (string, error) {
	if !strings.HasPrefix(banner, "220") {
		return "", fmt.Errorf("SMTP greeting is not a 220 reply: %q", banner)
	}
	l := strings.ToLower(banner)
	switch {
	case strings.Contains(l, "mailpit"):
		return "mailpit", nil
	case strings.Contains(l, "mailhog"):
		return "mailhog", nil
	}
	return "", fmt.Errorf("SMTP greeting does not identify Mailpit or MailHog: %q", banner)
}

// readSMTPBanner connects to host:port, reads the first greeting line and
// sends QUIT. It never sends mail.
func readSMTPBanner(ctx context.Context, host, port string, timeout time.Duration) (string, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return "", fmt.Errorf("connect SMTP %s: %w", net.JoinHostPort(host, port), err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("set SMTP deadline: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(conn, 4096)).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read SMTP greeting: %w", err)
	}
	_, _ = conn.Write([]byte("QUIT\r\n"))
	return strings.TrimRight(line, "\r\n"), nil
}

// mailAPIEndpoints are probed in order; the first JSON answer wins.
var mailAPIEndpoints = []struct{ Path, Sink string }{
	{"/api/v1/info", "mailpit"},
	{"/api/v1/messages", "mailpit"},
	{"/api/v2/messages", "mailhog"},
}

// probeMailAPI issues GET requests against the mail sink API and accepts the
// first 200 response whose body is a JSON object.
func probeMailAPI(ctx context.Context, client *http.Client, base string) (SinkProbe, error) {
	base = strings.TrimRight(base, "/")
	var errs []error
	for _, ep := range mailAPIEndpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+ep.Path, nil)
		if err != nil {
			return SinkProbe{}, fmt.Errorf("build mail API request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, fmt.Errorf("GET %s: %w", ep.Path, err))
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("GET %s: read body: %w", ep.Path, err))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("GET %s: status %d", ep.Path, resp.StatusCode))
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(body, &obj); err != nil {
			errs = append(errs, fmt.Errorf("GET %s: body is not a JSON object: %w", ep.Path, err))
			continue
		}
		return SinkProbe{APIEndpoint: ep.Path, APISink: ep.Sink, APIStatus: resp.StatusCode}, nil
	}
	return SinkProbe{}, fmt.Errorf("mail API %s is not Mailpit or MailHog: %w", base, errors.Join(errs...))
}

// EmailGate is the email gate outcome recorded in preflight.json.
type EmailGate struct {
	Requested bool        `json:"requested"`
	Enabled   bool        `json:"enabled"`
	Reasons   []string    `json:"reasons,omitempty"`
	EnvSource string      `json:"env_source,omitempty"`
	SMTP      *SMTPTarget `json:"smtp,omitempty"`
	Loopback  string      `json:"loopback,omitempty"`
	Probe     *SinkProbe  `json:"probe,omitempty"`
}

// emailDeps are the injectable effects of the email gate.
type emailDeps struct {
	Run      commandRunner
	ReadFile func(string) ([]byte, error)
	Lookup   resolver
	HTTP     *http.Client
	Timeout  time.Duration
}

// evaluateEmailGate runs every gate condition and records each failure.
// Loopback alone never passes the gate: the banner AND the API must both
// identify a sink.
func evaluateEmailGate(ctx context.Context, cfg *Config, d emailDeps) EmailGate {
	g := EmailGate{Requested: cfg.AllowEmail}
	if !cfg.AllowEmail {
		g.Reasons = []string{"--allow-email not set"}
		return g
	}
	show, src, err := workerSystemdShow(ctx, cfg, d.Run)
	g.EnvSource = src
	if err != nil {
		g.Reasons = append(g.Reasons, err.Error())
		return g
	}
	target, err := resolveSMTPTarget(show, d.ReadFile)
	if err != nil {
		g.Reasons = append(g.Reasons, err.Error())
		return g
	}
	g.SMTP = &target
	loop, why := isLoopbackHost(ctx, target.Host, d.Lookup)
	g.Loopback = why
	if !loop {
		g.Reasons = append(g.Reasons, fmt.Sprintf("SMTP_HOST %q is not loopback: %s", target.Host, why))
	}
	probe := SinkProbe{}
	banner, err := readSMTPBanner(ctx, target.Host, target.Port, d.Timeout)
	probe.Banner = banner
	if err != nil {
		g.Reasons = append(g.Reasons, err.Error())
	} else if sink, err := recognizeBanner(banner); err != nil {
		g.Reasons = append(g.Reasons, err.Error())
	} else {
		probe.BannerSink = sink
	}
	api, err := probeMailAPI(ctx, d.HTTP, cfg.MailAPI)
	if err != nil {
		g.Reasons = append(g.Reasons, err.Error())
	} else {
		probe.APIEndpoint, probe.APISink, probe.APIStatus = api.APIEndpoint, api.APISink, api.APIStatus
	}
	if probe.BannerSink != "" && probe.APISink != "" && probe.BannerSink != probe.APISink {
		g.Reasons = append(g.Reasons, fmt.Sprintf("SMTP banner identifies %s but the mail API identifies %s", probe.BannerSink, probe.APISink))
	}
	g.Probe = &probe
	g.Enabled = len(g.Reasons) == 0
	return g
}

func netDefaultLookup(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}
