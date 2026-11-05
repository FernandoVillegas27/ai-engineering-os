package toolhealth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type FailureKind string

const (
	FailureNone       FailureKind = ""
	FailureUnavailable FailureKind = "unavailable"
	FailureTimeout     FailureKind = "timeout"
	FailureAuth        FailureKind = "auth"
	FailurePermission  FailureKind = "permission"
	FailureRateLimit   FailureKind = "rate-limit"
	FailureTransport   FailureKind = "transport"
	FailureProtocol    FailureKind = "protocol"
	FailureUnhealthy   FailureKind = "unhealthy"
	FailureUnknown     FailureKind = "unknown"
)

const (
	DefaultProbeTimeout     = 5 * time.Second
	DefaultOperationTimeout = 30 * time.Second
)

type Failure struct {
	Kind      FailureKind `json:"kind,omitempty"`
	Retryable bool        `json:"retryable"`
	Detail    string      `json:"detail,omitempty"`
}

type Status struct {
	Provider  string        `json:"provider"`
	Available bool          `json:"available"`
	Healthy   bool          `json:"healthy"`
	Version   string        `json:"version,omitempty"`
	Details   string        `json:"details,omitempty"`
	Failure   Failure       `json:"failure,omitempty"`
	Duration  time.Duration `json:"duration"`
}

func Run(
	ctx context.Context,
	timeout time.Duration,
	fn func(context.Context) (string, error),
) (string, Failure, time.Duration) {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := fn(runCtx)
	duration := time.Since(started)
	if err == nil {
		return output, Failure{}, duration
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return output, Failure{
			Kind:      FailureTimeout,
			Retryable: true,
			Detail:    fmt.Sprintf("operation exceeded %s", timeout),
		}, duration
	}
	return output, Classify(err, output), duration
}

func Classify(err error, output string) Failure {
	if err == nil {
		return Failure{}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return Failure{Kind: FailureUnavailable, Detail: err.Error()}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Kind: FailureTimeout, Retryable: true, Detail: err.Error()}
	}

	combined := strings.ToLower(strings.TrimSpace(err.Error() + "\n" + output))
	switch {
	case containsAny(combined, "401", "unauthorized", "authentication failed", "invalid token", "not authenticated"):
		return Failure{Kind: FailureAuth, Detail: compactDetail(err, output)}
	case containsAny(combined, "403", "permission denied", "forbidden", "access denied"):
		return Failure{Kind: FailurePermission, Detail: compactDetail(err, output)}
	case containsAny(combined, "429", "rate limit", "too many requests", "quota exceeded"):
		return Failure{Kind: FailureRateLimit, Retryable: true, Detail: compactDetail(err, output)}
	case containsAny(combined, "connection refused", "connection reset", "network is unreachable", "no such host", "tls handshake", "temporary failure"):
		return Failure{Kind: FailureTransport, Retryable: true, Detail: compactDetail(err, output)}
	case containsAny(combined, "invalid json", "protocol error", "unexpected response", "malformed response"):
		return Failure{Kind: FailureProtocol, Detail: compactDetail(err, output)}
	default:
		return Failure{Kind: FailureUnknown, Detail: compactDetail(err, output)}
	}
}

func Unhealthy(detail string) Failure {
	return Failure{
		Kind:   FailureUnhealthy,
		Detail: strings.TrimSpace(detail),
	}
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func compactDetail(err error, output string) string {
	parts := []string{}
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		parts = append(parts, strings.TrimSpace(err.Error()))
	}
	if strings.TrimSpace(output) != "" {
		parts = append(parts, strings.TrimSpace(output))
	}
	detail := strings.Join(parts, ": ")
	if len(detail) > 2000 {
		detail = detail[:1997] + "..."
	}
	return detail
}
