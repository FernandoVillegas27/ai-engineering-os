package codex

import (
	"fmt"
	"time"
)

const (
	defaultRuntimeTimeout = 5 * time.Minute
	minimumRuntimeTimeout = 5 * time.Second
)

func resolveRuntimeTimeout(configured time.Duration) (time.Duration, error) {
	if configured == 0 {
		return defaultRuntimeTimeout, nil
	}
	if configured < minimumRuntimeTimeout {
		return 0, fmt.Errorf(
			"Codex runtime timeout must be zero or at least %s",
			minimumRuntimeTimeout,
		)
	}
	return configured, nil
}
