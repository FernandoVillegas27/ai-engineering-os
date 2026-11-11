package toolapproval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrMutationAlreadyClaimed means the exact side effect has already been
// claimed. Claims are intentionally irreversible: after a crash the external
// provider may have accepted the request even if no audit completion exists.
var ErrMutationAlreadyClaimed = errors.New("external mutation already claimed")

// ClaimMutation atomically reserves one exact request for a run and task.
// The claim persists after process exit, including an interrupted mutation.
// Do not delete or reclaim it automatically: doing so could duplicate an
// already accepted external side effect.
func (s *Store) ClaimMutation(runID, taskID, toolID, operation, requestDigest string) error {
	if s == nil {
		return errors.New("tool approval store is required")
	}
	if !safeToken(runID) || !safeToken(taskID) || !safeToken(toolID) ||
		operation == "" || !validDigest(requestDigest) {
		return errors.New("unsafe or incomplete external mutation claim identity")
	}
	runDir := filepath.Join(s.root, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create mutation claim directory: %w", err)
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("mutation claim directory must be a regular directory")
	}

	// Digest the full operation identity, not only the provider request.
	// An exact request can be claimed only once per run/task/tool/operation.
	key := runID + "\x00" + taskID + "\x00" + toolID + "\x00" + operation + "\x00" + requestDigest
	hash := sha256.Sum256([]byte(key))
	name := "mutation-" + hex.EncodeToString(hash[:]) + ".claim"
	path := filepath.Join(runDir, name)
	// O_EXCL is the arbitration point across concurrent CLI processes.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ErrMutationAlreadyClaimed
	}
	if err != nil {
		return fmt.Errorf("create immutable mutation claim: %w", err)
	}
	// On a write/sync/close error, retain the claim and fail closed. An
	// operator must inspect external state before choosing another request.
	if _, err := file.WriteString(requestDigest + "\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("persist mutation claim: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync mutation claim: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close mutation claim: %w", err)
	}
	return nil
}
