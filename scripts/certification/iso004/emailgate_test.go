package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSystemdShow(t *testing.T) {
	out := strings.Join([]string{
		`Environment=SMTP_HOST=10.0.0.9 "GREETING=hello world" SMTP_PORT=2525 ESC=a\ b`,
		`EnvironmentFile=/opt/odyssey-staging/.env (ignore_errors=no)`,
		`EnvironmentFile=/etc/odyssey/smtp.env (ignore_errors=yes)`,
		`EnvironmentFile=-/etc/odyssey/legacy.env`,
		`Unrelated=ignored`,
		``,
	}, "\n")
	show, err := parseSystemdShow(out)
	require.NoError(t, err)
	assert.Equal(t, []string{"SMTP_HOST=10.0.0.9", "GREETING=hello world", "SMTP_PORT=2525", "ESC=a b"}, show.Inline)
	assert.Equal(t, []string{"/opt/odyssey-staging/.env", "/etc/odyssey/smtp.env", "/etc/odyssey/legacy.env"}, show.Files)
	assert.False(t, show.Optional["/opt/odyssey-staging/.env"])
	assert.True(t, show.Optional["/etc/odyssey/smtp.env"])
	assert.True(t, show.Optional["/etc/odyssey/legacy.env"])

	empty, err := parseSystemdShow("Environment=\nEnvironmentFile=\n")
	require.NoError(t, err)
	assert.Empty(t, empty.Inline)
	assert.Empty(t, empty.Files)

	_, err = parseSystemdShow(`Environment="SMTP_HOST=x`)
	assert.Error(t, err, "unterminated quote")
}

func TestParseEnvFileOnlyReturnsSMTPKeys(t *testing.T) {
	body := strings.Join([]string{
		"# comment",
		"; also a comment",
		"SESSION_SECRET=super-secret",
		"SMTP_HOST=127.0.0.1",
		`export SMTP_PORT="1025"`,
		"SMTP_PASSWORD=hunter2",
		"SMTP_HOST='mailpit' ",
		"SMTP_PORT=10\\",
		"25",
		"   ",
	}, "\n")
	kvs, err := parseEnvFile(strings.NewReader(body), smtpKeys)
	require.NoError(t, err)
	assert.Equal(t, [][2]string{
		{"SMTP_HOST", "127.0.0.1"},
		{"SMTP_PORT", "1025"},
		{"SMTP_HOST", "mailpit"},
		{"SMTP_PORT", "1025"},
	}, kvs)
	for _, kv := range kvs {
		assert.NotContains(t, kv[1], "secret")
		assert.NotContains(t, kv[1], "hunter2")
	}
}

func fakeFS(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if v, ok := files[p]; ok {
			return []byte(v), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: os.ErrNotExist}
	}
}

func TestResolveSMTPTargetPrecedence(t *testing.T) {
	cases := map[string]struct {
		show     systemdShow
		files    map[string]string
		host     string
		port     string
		hostFrom string
		portFrom string
		wantErr  string
	}{
		"defaults when nothing is set": {
			show: systemdShow{}, host: "127.0.0.1", port: "1025", hostFrom: "default", portFrom: "default",
		},
		"inline only": {
			show: systemdShow{Inline: []string{"SMTP_HOST=::1", "SMTP_PORT=2525"}},
			host: "::1", port: "2525", hostFrom: "Environment=", portFrom: "Environment=",
		},
		"inline: last assignment wins": {
			show: systemdShow{Inline: []string{"SMTP_HOST=a", "SMTP_HOST=b"}},
			host: "b", port: "1025", hostFrom: "Environment=", portFrom: "default",
		},
		"file overrides inline (systemd order)": {
			show:  systemdShow{Inline: []string{"SMTP_HOST=10.0.0.5", "SMTP_PORT=25"}, Files: []string{"/a.env"}},
			files: map[string]string{"/a.env": "SMTP_HOST=127.0.0.1\n"},
			host:  "127.0.0.1", port: "25", hostFrom: "EnvironmentFile=/a.env", portFrom: "Environment=",
		},
		"later file overrides earlier file": {
			show:  systemdShow{Files: []string{"/a.env", "/b.env"}},
			files: map[string]string{"/a.env": "SMTP_HOST=127.0.0.1\nSMTP_PORT=1025\n", "/b.env": "SMTP_HOST=smtp.example.com\n"},
			host:  "smtp.example.com", port: "1025", hostFrom: "EnvironmentFile=/b.env", portFrom: "EnvironmentFile=/a.env",
		},
		"later line in the same file wins": {
			show:  systemdShow{Files: []string{"/a.env"}},
			files: map[string]string{"/a.env": "SMTP_PORT=1025\nSMTP_PORT=2525\n"},
			host:  "127.0.0.1", port: "2525", hostFrom: "default", portFrom: "EnvironmentFile=/a.env",
		},
		"optional missing file is skipped": {
			show: systemdShow{Files: []string{"/missing.env"}, Optional: map[string]bool{"/missing.env": true}},
			host: "127.0.0.1", port: "1025", hostFrom: "default", portFrom: "default",
		},
		"required missing file fails": {
			show:    systemdShow{Files: []string{"/missing.env"}},
			wantErr: "read EnvironmentFile /missing.env",
		},
		"invalid port fails": {
			show:    systemdShow{Inline: []string{"SMTP_PORT=smtp"}},
			wantErr: "not a valid port",
		},
		"empty host fails": {
			show:    systemdShow{Inline: []string{"SMTP_HOST="}},
			wantErr: "SMTP_HOST is empty",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := resolveSMTPTarget(tc.show, fakeFS(tc.files))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, SMTPTarget{Host: tc.host, Port: tc.port, HostSource: tc.hostFrom, PortSource: tc.portFrom}, got)
		})
	}
}

func TestWorkerSystemdShowUsesUnitOrWorkerEnv(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("Environment=SMTP_PORT=2525\nEnvironmentFile=/opt/odyssey-staging/.env (ignore_errors=no)\n"), nil
	}
	cfg := &Config{WorkerUnit: "odyssey-staging-worker.service"}
	show, src, err := workerSystemdShow(context.Background(), cfg, run)
	require.NoError(t, err)
	assert.Equal(t, []string{"systemctl", "show", "-p", "Environment", "-p", "EnvironmentFile", "odyssey-staging-worker.service"}, gotArgs)
	assert.Equal(t, "systemctl show odyssey-staging-worker.service", src)
	assert.Equal(t, []string{"/opt/odyssey-staging/.env"}, show.Files)

	gotArgs = nil
	cfg.WorkerEnv = []string{"./.env"}
	show, src, err = workerSystemdShow(context.Background(), cfg, run)
	require.NoError(t, err)
	assert.Nil(t, gotArgs, "--worker-env replaces the unit lookup")
	assert.Equal(t, "--worker-env", src)
	assert.Equal(t, []string{"./.env"}, show.Files)

	_, _, err = workerSystemdShow(context.Background(), &Config{WorkerUnit: "x"}, func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exec: systemctl not found")
	})
	assert.ErrorContains(t, err, "systemctl show x")
}

func TestIsLoopbackHost(t *testing.T) {
	loopbackOnly := func(_ context.Context, host string) ([]string, error) {
		if host != "localhost" {
			return nil, fmt.Errorf("unexpected lookup %s", host)
		}
		return []string{"127.0.0.1", "::1"}, nil
	}
	cases := []struct {
		host   string
		lookup resolver
		want   bool
	}{
		{"127.0.0.1", loopbackOnly, true},
		{"127.10.20.30", loopbackOnly, true},
		{"::1", loopbackOnly, true},
		{"[::1]", loopbackOnly, true},
		{"localhost", loopbackOnly, true},
		{"LOCALHOST.", loopbackOnly, true},
		{"mailpit", func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }, false},
		{"10.0.0.5", loopbackOnly, false},
		{"0.0.0.0", loopbackOnly, false},
		{"::", loopbackOnly, false},
		{"localhost", func(context.Context, string) ([]string, error) { return []string{"127.0.0.1", "10.0.0.5"}, nil }, false},
		{"localhost", func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") }, false},
		{"localhost", func(context.Context, string) ([]string, error) { return nil, nil }, false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, why := isLoopbackHost(context.Background(), tc.host, tc.lookup)
			assert.Equal(t, tc.want, got, why)
			assert.NotEmpty(t, why)
		})
	}
}

// stubSMTP accepts connections on loopback and writes greeting, then records
// whether the client sent QUIT.
func stubSMTP(t *testing.T, greeting string) (host, port string, quit chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	quit = make(chan string, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				if greeting != "" {
					_, _ = c.Write([]byte(greeting))
				}
				line, _ := bufio.NewReader(c).ReadString('\n')
				quit <- strings.TrimSpace(line)
			}(conn)
		}
	}()
	h, p, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return h, p, quit
}

func TestBannerRecognitionAgainstStubServers(t *testing.T) {
	cases := []struct {
		name     string
		greeting string
		sink     string
		wantErr  string
	}{
		{"mailpit", "220 staging-host Mailpit ESMTP Service ready\r\n", "mailpit", ""},
		{"mailhog", "220 mailhog.example ESMTP MailHog\r\n", "mailhog", ""},
		{"postfix", "220 mx.example.com ESMTP Postfix (Debian/GNU)\r\n", "", "does not identify Mailpit or MailHog"},
		{"rejecting mailpit", "554 Mailpit says no\r\n", "", "not a 220 reply"},
		{"silent server", "", "", "read SMTP greeting"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, quit := stubSMTP(t, tc.greeting)
			banner, err := readSMTPBanner(context.Background(), host, port, 300*time.Millisecond)
			if err == nil {
				var sink string
				sink, err = recognizeBanner(banner)
				if tc.wantErr == "" {
					require.NoError(t, err)
					assert.Equal(t, tc.sink, sink)
					select {
					case got := <-quit:
						assert.Equal(t, "QUIT", got, "the probe only says QUIT")
					case <-time.After(2 * time.Second):
						t.Fatal("stub did not receive QUIT")
					}
					return
				}
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}

	_, err := readSMTPBanner(context.Background(), "127.0.0.1", closedPort(t), 300*time.Millisecond)
	assert.ErrorContains(t, err, "connect SMTP")
}

func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, ln.Close())
	return p
}

func stubAPI(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "the probe is read-only")
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMailAPIRecognitionAgainstStubServers(t *testing.T) {
	cases := []struct {
		name     string
		routes   map[string]string
		endpoint string
		sink     string
		wantErr  string
	}{
		{"mailpit info", map[string]string{"/api/v1/info": `{"Version":"v1.21.0","Messages":0}`}, "/api/v1/info", "mailpit", ""},
		{"mailpit messages only", map[string]string{"/api/v1/messages": `{"total":0,"messages":[]}`}, "/api/v1/messages", "mailpit", ""},
		{"mailhog v2", map[string]string{"/api/v2/messages": `{"total":0,"count":0,"start":0,"items":[]}`}, "/api/v2/messages", "mailhog", ""},
		{"html page", map[string]string{"/api/v1/info": `<html>login</html>`}, "", "", "not a JSON object"},
		{"json array is not accepted", map[string]string{"/api/v1/info": `[]`}, "", "", "not a JSON object"},
		{"nothing there", map[string]string{}, "", "", "status 404"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := stubAPI(t, tc.routes)
			probe, err := probeMailAPI(context.Background(), srv.Client(), srv.URL+"/")
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.endpoint, probe.APIEndpoint)
			assert.Equal(t, tc.sink, probe.APISink)
			assert.Equal(t, http.StatusOK, probe.APIStatus)
		})
	}
}

func gateDeps(t *testing.T, files map[string]string, srv *httptest.Server) emailDeps {
	t.Helper()
	client := http.DefaultClient
	if srv != nil {
		client = srv.Client()
	}
	return emailDeps{
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("systemctl must not run when --worker-env is given")
			return nil, nil
		},
		ReadFile: fakeFS(files),
		Lookup:   func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil },
		HTTP:     client,
		Timeout:  500 * time.Millisecond,
	}
}

func TestEvaluateEmailGate(t *testing.T) {
	t.Run("disabled without --allow-email", func(t *testing.T) {
		g := evaluateEmailGate(context.Background(), &Config{}, emailDeps{})
		assert.False(t, g.Enabled)
		assert.Equal(t, []string{"--allow-email not set"}, g.Reasons)
	})

	t.Run("mailpit banner and API pass", func(t *testing.T) {
		host, port, _ := stubSMTP(t, "220 h Mailpit ESMTP Service ready\r\n")
		api := stubAPI(t, map[string]string{"/api/v1/info": `{"Version":"x"}`})
		cfg := &Config{AllowEmail: true, MailAPI: api.URL, WorkerEnv: []string{"/w.env"}}
		g := evaluateEmailGate(context.Background(), cfg, gateDeps(t, map[string]string{"/w.env": "SMTP_HOST=" + host + "\nSMTP_PORT=" + port + "\nSMTP_PASSWORD=pw\n"}, api))
		assert.True(t, g.Enabled, "%v", g.Reasons)
		assert.Empty(t, g.Reasons)
		require.NotNil(t, g.SMTP)
		assert.Equal(t, "EnvironmentFile=/w.env", g.SMTP.HostSource)
		assert.Equal(t, "mailpit", g.Probe.BannerSink)
		assert.Equal(t, "mailpit", g.Probe.APISink)
	})

	t.Run("loopback alone is not proof of a sink", func(t *testing.T) {
		host, port, _ := stubSMTP(t, "220 mx ESMTP Postfix\r\n")
		api := stubAPI(t, map[string]string{})
		cfg := &Config{AllowEmail: true, MailAPI: api.URL, WorkerEnv: []string{"/w.env"}}
		g := evaluateEmailGate(context.Background(), cfg, gateDeps(t, map[string]string{"/w.env": "SMTP_HOST=" + host + "\nSMTP_PORT=" + port + "\n"}, api))
		assert.False(t, g.Enabled)
		joined := strings.Join(g.Reasons, "\n")
		assert.Contains(t, joined, "does not identify Mailpit or MailHog")
		assert.Contains(t, joined, "is not Mailpit or MailHog")
	})

	t.Run("non-loopback host fails even with a sink banner", func(t *testing.T) {
		_, port, _ := stubSMTP(t, "220 h Mailpit ESMTP Service ready\r\n")
		api := stubAPI(t, map[string]string{"/api/v1/info": `{}`})
		cfg := &Config{AllowEmail: true, MailAPI: api.URL, WorkerEnv: []string{"/w.env"}}
		g := evaluateEmailGate(context.Background(), cfg, gateDeps(t, map[string]string{"/w.env": "SMTP_HOST=mailpit\nSMTP_PORT=" + port + "\n"}, api))
		assert.False(t, g.Enabled)
		assert.Contains(t, strings.Join(g.Reasons, "\n"), `SMTP_HOST "mailpit" is not loopback`)
	})

	t.Run("banner and API disagree", func(t *testing.T) {
		host, port, _ := stubSMTP(t, "220 h ESMTP MailHog\r\n")
		api := stubAPI(t, map[string]string{"/api/v1/info": `{}`})
		cfg := &Config{AllowEmail: true, MailAPI: api.URL, WorkerEnv: []string{"/w.env"}}
		g := evaluateEmailGate(context.Background(), cfg, gateDeps(t, map[string]string{"/w.env": "SMTP_HOST=" + host + "\nSMTP_PORT=" + port + "\n"}, api))
		assert.False(t, g.Enabled)
		assert.Contains(t, strings.Join(g.Reasons, "\n"), "banner identifies mailhog but the mail API identifies mailpit")
	})

	t.Run("unreadable worker env", func(t *testing.T) {
		cfg := &Config{AllowEmail: true, MailAPI: "http://127.0.0.1:1", WorkerEnv: []string{filepath.Join(t.TempDir(), "absent.env")}}
		g := evaluateEmailGate(context.Background(), cfg, emailDeps{ReadFile: os.ReadFile})
		assert.False(t, g.Enabled)
		assert.Contains(t, strings.Join(g.Reasons, "\n"), "read EnvironmentFile")
	})
}
