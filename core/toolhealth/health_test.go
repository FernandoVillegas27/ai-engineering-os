package toolhealth

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestClassifyExternalFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		out  string
		kind FailureKind
		retry bool
	}{
		{"missing", exec.ErrNotFound, "", FailureUnavailable, false},
		{"auth", errors.New("request failed"), "401 Unauthorized", FailureAuth, false},
		{"permission", errors.New("403 Forbidden"), "", FailurePermission, false},
		{"rate", errors.New("429 too many requests"), "", FailureRateLimit, true},
		{"transport", errors.New("connection refused"), "", FailureTransport, true},
		{"protocol", errors.New("invalid JSON response"), "", FailureProtocol, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err, tc.out)
			if got.Kind != tc.kind || got.Retryable != tc.retry {
				t.Fatalf("got=%#v", got)
			}
		})
	}
}

func TestRunEnforcesDeadlineWithoutAutomaticRetry(t *testing.T) {
	calls := 0
	_, failure, _ := Run(context.Background(), 20*time.Millisecond, func(ctx context.Context) (string, error) {
		calls++
		<-ctx.Done()
		return "", ctx.Err()
	})
	if failure.Kind != FailureTimeout || !failure.Retryable {
		t.Fatalf("failure=%#v", failure)
	}
	if calls != 1 {
		t.Fatalf("calls=%d want=1", calls)
	}
}


func TestExternalDiagnosticsRedactCredentialsOnSuccessAndFailure(t *testing.T) {
	raw := "Authorization: Bearer abc.def.ghi api_key=1234567890abcdef ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456 sk-abcdefghijklmnopqrstuvwx"
	if redacted := Redact(raw); strings.Contains(redacted, "abc.def.ghi") ||
		strings.Contains(redacted, "1234567890abcdef") ||
		strings.Contains(redacted, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456") ||
		strings.Contains(redacted, "sk-abcdefghijklmnopqrstuvwx") {
		t.Fatalf("redaction leaked secret: %q", redacted)
	}

	output, failure, _ := Run(context.Background(), time.Second, func(context.Context) (string, error) {
		return raw, nil
	})
	if failure.Kind != FailureNone || strings.Contains(output, "abc.def.ghi") {
		t.Fatalf("success output=%q failure=%#v", output, failure)
	}

	failure = Classify(errors.New("401 Unauthorized token=super-secret-value"), raw)
	if failure.Kind != FailureAuth {
		t.Fatalf("failure=%#v", failure)
	}
	if strings.Contains(failure.Detail, "super-secret-value") || strings.Contains(failure.Detail, "abc.def.ghi") {
		t.Fatalf("failure detail leaked secret: %q", failure.Detail)
	}
}

func TestUnhealthyRedactsPrivateKey(t *testing.T) {
	failure := Unhealthy("doctor failed\n-----BEGIN PRIVATE KEY-----\nSECRETKEYDATA\n-----END PRIVATE KEY-----")
	if strings.Contains(failure.Detail, "SECRETKEYDATA") || !strings.Contains(failure.Detail, "[REDACTED]") {
		t.Fatalf("failure=%#v", failure)
	}
}
