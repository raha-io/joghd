// Package alerter delivers health-check alerts to notification channels.
//
// Every channel implements [Alerter]. [CompositeAlerter] fans one alert out to
// several of them and joins their failures, [CompanyFilter] restricts a
// channel to the targets of particular companies, and [TelegramAlerter] and
// [MattermostAlerter] are the shipped implementations. Payloads are encoded
// with encoding/json/v2 (see json.go).
package alerter

import (
	"context"
	"io"

	"resty.dev/v3"

	"github.com/rahacloud/joghd/internal/domain"
)

// Alerter defines the interface for sending alerts.
type Alerter interface {
	// Send sends an alert notification.
	Send(ctx context.Context, alert domain.Alert) error

	// Name returns the alerter implementation name for logging.
	Name() string
}

// releaseBody drains and closes a response body so its connection can be
// reused. resty leaves the body open whenever it has nowhere to decode it.
func releaseBody(resp *resty.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
