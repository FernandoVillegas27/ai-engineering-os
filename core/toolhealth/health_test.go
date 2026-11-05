package toolhealth

import (
	"context"
	"errors"
	"os/exec"
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
