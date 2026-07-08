package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

const maxWebhookResponseBytes = 4096

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type baseReceiver struct {
	spec ReceiverSpec
}

func (r baseReceiver) ID() string {
	return r.spec.ID
}

func (r baseReceiver) Enabled() bool {
	return r.spec.Enabled
}

func (r baseReceiver) Type() string {
	return r.spec.Type
}

func (r baseReceiver) CostClass() string {
	if r.spec.CostClass == "" {
		return "free_core"
	}
	return r.spec.CostClass
}

func (r baseReceiver) Summary() string {
	if r.spec.DisplayName != "" {
		return redaction.Redact(r.spec.DisplayName)
	}
	return redaction.Redact(r.spec.ID)
}

func (r baseReceiver) Matches(event Event, severity incidents.Severity) bool {
	if len(r.spec.Events) > 0 && !slices.Contains(r.spec.Events, event) {
		return false
	}
	if event == EventResolved {
		return true
	}
	if len(r.spec.Severities) > 0 && !slices.Contains(r.spec.Severities, severity) {
		return false
	}
	return true
}

type WebhookReceiver struct {
	baseReceiver
	client HTTPDoer
}

func NewWebhookReceiver(spec ReceiverSpec, client HTTPDoer) WebhookReceiver {
	if client == nil {
		client = noRedirectHTTPClient()
	}
	return WebhookReceiver{baseReceiver: baseReceiver{spec: spec}, client: client}
}

func noRedirectHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (r WebhookReceiver) Deliver(ctx context.Context, payload Payload) DeliveryOutcome {
	if r.spec.URL == "" {
		return DeliveryOutcome{Success: false, ErrorClass: "config", Summary: "webhook URL is not configured"}
	}
	timeout := r.spec.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return DeliveryOutcome{Success: false, ErrorClass: "render", Summary: "notification payload could not be encoded"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.spec.URL, bytes.NewReader(body))
	if err != nil {
		return DeliveryOutcome{Success: false, ErrorClass: "config", Summary: "webhook request could not be created"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pooly-sentinel")

	resp, err := r.client.Do(req)
	if err != nil {
		return classifyWebhookError(err)
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxWebhookResponseBytes+1))
	if readErr != nil {
		return DeliveryOutcome{Success: false, ErrorClass: "response_read", Summary: "webhook response could not be read"}
	}
	truncated := len(responseBody) > maxWebhookResponseBytes
	if truncated {
		responseBody = responseBody[:maxWebhookResponseBytes]
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		summary := fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)
		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			if len(responseBody) > 0 {
				summary += ": " + redaction.Redact(string(responseBody))
			}
		}
		if truncated {
			summary += " (truncated)"
		}
		return DeliveryOutcome{Success: false, ErrorClass: "http_status", Summary: summary}
	}
	return DeliveryOutcome{Success: true, Summary: "webhook delivered"}
}

func classifyWebhookError(err error) DeliveryOutcome {
	if errors.Is(err, context.Canceled) {
		return DeliveryOutcome{Success: false, ErrorClass: "canceled", Summary: "webhook delivery canceled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return DeliveryOutcome{Success: false, ErrorClass: "timeout", Summary: "webhook delivery timed out"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return DeliveryOutcome{Success: false, ErrorClass: "timeout", Summary: "webhook delivery timed out"}
	}
	return DeliveryOutcome{Success: false, ErrorClass: "delivery_error", Summary: redaction.Redact(err.Error())}
}

type NoopReceiver struct {
	baseReceiver
}

func NewNoopReceiver(spec ReceiverSpec) NoopReceiver {
	return NoopReceiver{baseReceiver: baseReceiver{spec: spec}}
}

func (r NoopReceiver) Deliver(ctx context.Context, payload Payload) DeliveryOutcome {
	if err := ctx.Err(); err != nil {
		return DeliveryOutcome{Success: false, ErrorClass: "canceled", Summary: "noop delivery canceled"}
	}
	return DeliveryOutcome{Success: true, Summary: "noop delivered"}
}
