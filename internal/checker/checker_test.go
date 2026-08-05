package checker

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rahacloud/joghd/internal/config"
	"github.com/rahacloud/joghd/internal/domain"
)

// fakeClient returns scripted responses and records how often it was called.
type fakeClient struct {
	mu        sync.Mutex
	responses []response
	calls     int

	inFlight atomic.Int64
	maxSeen  atomic.Int64
	block    chan struct{}
}

type response struct {
	status  int
	latency time.Duration
	err     error
}

func (f *fakeClient) Execute(ctx context.Context, _, _ string, _ map[string]string, _ time.Duration) (int, time.Duration, error) {
	cur := f.inFlight.Add(1)
	for {
		seen := f.maxSeen.Load()
		if cur <= seen || f.maxSeen.CompareAndSwap(seen, cur) {
			break
		}
	}

	defer f.inFlight.Add(-1)

	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	r := f.responses[min(f.calls-1, len(f.responses)-1)]

	return r.status, r.latency, r.err
}

func fastRetry(attempts int) config.RetryConfig {
	return config.RetryConfig{
		MaxAttempts: attempts,
		InitialWait: time.Millisecond,
		MaxWait:     2 * time.Millisecond,
		Multiplier:  2,
	}
}

func target(name string) domain.Target {
	return domain.Target{
		Name:           name,
		URL:            "http://example.test/health",
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		Interval:       time.Second,
		Timeout:        time.Second,
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name         string
		responses    []response
		maxAttempts  int
		wantSuccess  bool
		wantAttempts int
		wantCalls    int
		wantErrText  string
	}{
		{
			name:         "healthy on first attempt",
			responses:    []response{{status: http.StatusOK}},
			maxAttempts:  3,
			wantSuccess:  true,
			wantAttempts: 1,
			wantCalls:    1,
		},
		{
			name:         "recovers on the final attempt",
			responses:    []response{{status: http.StatusBadGateway}, {status: http.StatusBadGateway}, {status: http.StatusOK}},
			maxAttempts:  3,
			wantSuccess:  true,
			wantAttempts: 3,
			wantCalls:    3,
		},
		{
			name:         "status mismatch is retried then reported",
			responses:    []response{{status: http.StatusInternalServerError}},
			maxAttempts:  3,
			wantSuccess:  false,
			wantAttempts: 3,
			wantCalls:    3,
			wantErrText:  "status mismatch: expected 200, got 500",
		},
		{
			name:         "transport error is reported",
			responses:    []response{{err: errors.New("dial tcp: connection refused")}},
			maxAttempts:  2,
			wantSuccess:  false,
			wantAttempts: 2,
			wantCalls:    2,
			wantErrText:  "dial tcp: connection refused",
		},
		{
			// A non-positive max_attempts must never mean "never send a
			// request but report the target as down".
			name:         "non-positive max attempts still checks once",
			responses:    []response{{status: http.StatusOK}},
			maxAttempts:  0,
			wantSuccess:  true,
			wantAttempts: 1,
			wantCalls:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{responses: tc.responses}
			c := New(WithHTTPClient(client), WithRetryConfig(fastRetry(tc.maxAttempts)))

			result := c.Check(context.Background(), target("api"))

			if result.Success != tc.wantSuccess {
				t.Errorf("Success = %v, want %v (error: %v)", result.Success, tc.wantSuccess, result.Error)
			}
			if result.Attempts != tc.wantAttempts {
				t.Errorf("Attempts = %d, want %d", result.Attempts, tc.wantAttempts)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("HTTP calls = %d, want %d", client.calls, tc.wantCalls)
			}
			if tc.wantErrText == "" {
				if result.Error != nil {
					t.Errorf("Error = %v, want nil", result.Error)
				}
			} else if result.Error == nil || result.Error.Error() != tc.wantErrText {
				t.Errorf("Error = %v, want %q", result.Error, tc.wantErrText)
			}
		})
	}
}

func TestCheckBackoffIsBounded(t *testing.T) {
	client := &fakeClient{responses: []response{{status: http.StatusBadGateway}}}
	c := New(WithHTTPClient(client), WithRetryConfig(config.RetryConfig{
		MaxAttempts: 4,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     20 * time.Millisecond,
		Multiplier:  10,
	}))

	start := time.Now()
	c.Check(context.Background(), target("api"))
	elapsed := time.Since(start)

	// Waits are 10ms, 20ms (capped), 20ms (capped) = 50ms, not 10+100+1000ms.
	if elapsed > 300*time.Millisecond {
		t.Errorf("total backoff %s exceeds the configured max_wait cap", elapsed)
	}
}

func TestCheckStopsOnCancelledContext(t *testing.T) {
	client := &fakeClient{responses: []response{{status: http.StatusBadGateway}}}
	c := New(WithHTTPClient(client), WithRetryConfig(config.RetryConfig{
		MaxAttempts: 5,
		InitialWait: time.Hour,
		MaxWait:     time.Hour,
		Multiplier:  1,
	}))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan domain.CheckResult, 1)
	go func() { done <- c.Check(ctx, target("api")) }()

	select {
	case result := <-done:
		if !errors.Is(result.Error, context.Canceled) {
			t.Errorf("Error = %v, want context.Canceled", result.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Check ignored context cancellation")
	}
}

func TestCheckAllReturnsResultsInOrder(t *testing.T) {
	client := &fakeClient{responses: []response{{status: http.StatusOK}}}
	c := New(WithHTTPClient(client), WithRetryConfig(fastRetry(1)), WithConcurrency(2))

	targets := []domain.Target{target("a"), target("b"), target("c")}

	results := c.CheckAll(context.Background(), targets)

	if len(results) != len(targets) {
		t.Fatalf("got %d results, want %d", len(results), len(targets))
	}

	for i, want := range targets {
		if results[i].Target.Name != want.Name {
			t.Errorf("results[%d].Target.Name = %q, want %q", i, results[i].Target.Name, want.Name)
		}
	}
}

func TestCheckAllRespectsConcurrencyLimit(t *testing.T) {
	client := &fakeClient{responses: []response{{status: http.StatusOK}}, block: make(chan struct{})}
	c := New(WithHTTPClient(client), WithRetryConfig(fastRetry(1)), WithConcurrency(2))

	targets := make([]domain.Target, 10)
	for i := range targets {
		targets[i] = target("t")
		targets[i].Name = string(rune('a' + i))
	}

	done := make(chan struct{})
	go func() {
		c.CheckAll(context.Background(), targets)
		close(done)
	}()

	// Let the goroutines pile up against the semaphore, then release them.
	time.Sleep(100 * time.Millisecond)
	close(client.block)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CheckAll did not finish")
	}

	if peak := client.maxSeen.Load(); peak > 2 {
		t.Errorf("peak concurrent requests = %d, want at most 2", peak)
	}
}

// A zero concurrency used to turn the semaphore into an unbuffered channel and
// block CheckAll forever.
func TestCheckAllWithNonPositiveConcurrency(t *testing.T) {
	client := &fakeClient{responses: []response{{status: http.StatusOK}}}
	c := New(WithHTTPClient(client), WithRetryConfig(fastRetry(1)), WithConcurrency(0))

	done := make(chan struct{})
	go func() {
		c.CheckAll(context.Background(), []domain.Target{target("a")})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CheckAll hung with concurrency = 0")
	}
}
