package checker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rahacloud/joghd/internal/config"
)

// jsonServer serves a JSON body and counts how many TCP connections it accepts.
// JSON matters: resty only drains a body it can decode somewhere, so a client
// that ignores the body leaks the connection for exactly this content type.
func jsonServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var conns atomic.Int64

	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	return srv, &conns
}

func okJSONHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Every check used to build its own client and leave the response body open,
// leaking one socket per check until the process ran out of descriptors.
func TestRestyClientReusesConnections(t *testing.T) {
	srv, conns := jsonServer(t, okJSONHandler)

	client := NewRestyClient(config.HTTPConfig{Timeout: 5 * time.Second, UserAgent: "Joghd/test"})
	t.Cleanup(func() { _ = client.Close() })

	const checks = 20
	for range checks {
		status, _, err := client.Execute(context.Background(), http.MethodGet, srv.URL, nil, 5*time.Second)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
	}

	if got := conns.Load(); got != 1 {
		t.Errorf("%d checks opened %d connections, want 1 (response bodies are not being released)", checks, got)
	}
}

// The per-request timeout must not cost the client its TLS configuration.
func TestRestyClientSkipTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(okJSONHandler))
	t.Cleanup(srv.Close)

	client := NewRestyClient(config.HTTPConfig{
		Timeout:             5 * time.Second,
		UserAgent:           "Joghd/test",
		SkipTLSVerification: true,
	})
	t.Cleanup(func() { _ = client.Close() })

	// A per-target timeout is always set in practice, so this is the path
	// production takes.
	status, _, err := client.Execute(context.Background(), http.MethodGet, srv.URL, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("with per-target timeout: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}

	if _, _, err := client.Execute(context.Background(), http.MethodGet, srv.URL, nil, 0); err != nil {
		t.Errorf("without per-target timeout: %v", err)
	}
}

func TestRestyClientPerRequestTimeout(t *testing.T) {
	srv, _ := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			okJSONHandler(w, r)
		case <-r.Context().Done():
		}
	})

	client := NewRestyClient(config.HTTPConfig{Timeout: time.Minute, UserAgent: "Joghd/test"})
	t.Cleanup(func() { _ = client.Close() })

	start := time.Now()
	if _, _, err := client.Execute(context.Background(), http.MethodGet, srv.URL, nil, 100*time.Millisecond); err == nil {
		t.Fatal("expected a timeout error, got nil")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %s; the per-request timeout was not applied", elapsed)
	}
}

func TestRestyClientSendsHeaders(t *testing.T) {
	var gotUA, gotAuth string

	srv, _ := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		okJSONHandler(w, r)
	})

	client := NewRestyClient(config.HTTPConfig{Timeout: 5 * time.Second, UserAgent: "Joghd/test"})
	t.Cleanup(func() { _ = client.Close() })

	if _, _, err := client.Execute(context.Background(), http.MethodGet, srv.URL,
		map[string]string{"Authorization": "Bearer token"}, 5*time.Second); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotUA != "Joghd/test" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "Joghd/test")
	}
	if gotAuth != "Bearer token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer token")
	}
}

func TestRestyClientContextCancellation(t *testing.T) {
	srv, _ := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	client := NewRestyClient(config.HTTPConfig{Timeout: time.Minute, UserAgent: "Joghd/test"})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if _, _, err := client.Execute(ctx, http.MethodGet, srv.URL, nil, time.Minute); err == nil {
		t.Error("expected an error after cancellation, got nil")
	}
}
