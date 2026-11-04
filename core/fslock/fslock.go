package fslock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Options struct {
	RetryInterval time.Duration
	StaleAfter    time.Duration
	Now           func() time.Time
}

type Lock struct {
	path  string
	token string
}

func Acquire(ctx context.Context, path string, options Options) (*Lock, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("lock path is required")
	}
	retry := options.RetryInterval
	if retry <= 0 {
		retry = 20 * time.Millisecond
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	if staleAfter < time.Second {
		staleAfter = time.Second
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	payload := fmt.Sprintf("%s\n%d\n%s\n", token, os.Getpid(), now().UTC().Format(time.RFC3339Nano))

	ticker := time.NewTicker(retry)
	defer ticker.Stop()

	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, writeErr := file.WriteString(payload); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("write lock: %w", writeErr)
			}
			if syncErr := file.Sync(); syncErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("sync lock: %w", syncErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("close lock: %w", closeErr)
			}
			return &Lock{path: path, token: token}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock: %w", err)
		}

		if reclaimed, reclaimErr := reclaimIfStale(path, staleAfter, now()); reclaimErr != nil {
			return nil, reclaimErr
		} else if reclaimed {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("acquire lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (l *Lock) Release() error {
	if l == nil || strings.TrimSpace(l.path) == "" || strings.TrimSpace(l.token) == "" {
		return nil
	}
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read lock for release: %w", err)
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 || lines[0] != l.token {
		// Ownership changed. Never remove a lock created by another process.
		return nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

func reclaimIfStale(path string, staleAfter time.Duration, now time.Time) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect lock: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errors.New("lock path must be a regular file")
	}
	if now.Sub(info.ModTime()) < staleAfter {
		return false, nil
	}

	// Re-stat immediately before removal. This narrows the window in which a
	// stale lock could have been replaced by a fresh owner.
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reinspect stale lock: %w", err)
	}
	if current.ModTime() != info.ModTime() || current.Size() != info.Size() {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("remove stale lock: %w", err)
	}
	return true, nil
}

func randomToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate lock token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
