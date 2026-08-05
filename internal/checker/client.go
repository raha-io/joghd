package checker

import (
	"context"
	"crypto/tls"
	"io"
	"time"

	"resty.dev/v3"

	"github.com/rahacloud/joghd/internal/config"
)

// maxDrainBytes caps how much of an unused response body is read back before
// the connection is released. Draining lets the connection return to the pool;
// capping it stops an oversized body from wasting bandwidth.
const maxDrainBytes = 64 << 10

// HTTPClient abstracts HTTP operations for testability.
type HTTPClient interface {
	Execute(ctx context.Context, method, url string, headers map[string]string, timeout time.Duration) (statusCode int, latency time.Duration, err error)
}

// RestyClient wraps resty for HTTP operations.
type RestyClient struct {
	client *resty.Client
}

// NewRestyClient creates a new HTTP client with the given configuration.
func NewRestyClient(cfg config.HTTPConfig) *RestyClient {
	client := resty.New().
		SetTimeout(cfg.Timeout).
		SetHeader("User-Agent", cfg.UserAgent)

	if cfg.SkipTLSVerification {
		client.SetTLSClientConfig(&tls.Config{
			//nolint:gosec // explicitly opted into via http.skip_tls_verification
			InsecureSkipVerify: true,
		})
	}

	return &RestyClient{client: client}
}

// Execute performs an HTTP request and returns the status code, latency, and any error.
func (c *RestyClient) Execute(ctx context.Context, method, url string, headers map[string]string, timeout time.Duration) (int, time.Duration, error) {
	req := c.client.R().SetContext(ctx)

	// Override the client-wide timeout for this request only. Building a
	// separate client per request would drop the TLS and header settings
	// configured above and leak its connection pool.
	if timeout > 0 {
		req.SetTimeout(timeout)
	}

	for k, v := range headers {
		req.SetHeader(k, v)
	}

	start := time.Now()
	resp, err := req.Execute(method, url)
	latency := time.Since(start)

	// Only the status code is of interest, and resty leaves the body open
	// when it has nowhere to decode it, so release it here.
	releaseBody(resp)

	if err != nil {
		return 0, latency, err
	}

	return resp.StatusCode(), latency, nil
}

// Close releases the client's idle connections.
func (c *RestyClient) Close() error {
	return c.client.Close()
}

// releaseBody drains and closes a response body so its connection can be
// reused instead of being held open for the lifetime of the process.
func releaseBody(resp *resty.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_, _ = io.CopyN(io.Discard, resp.Body, maxDrainBytes)
	_ = resp.Body.Close()
}
