package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

type Bus struct {
	root string
}

func NewBus(root string) (*Bus, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("message bus storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve message bus storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create message bus storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("message bus storage root must be a regular directory")
	}
	return &Bus{root: absolute}, nil
}

func (b *Bus) Append(ctx context.Context, value Message) (Message, error) {
	if b == nil {
		return Message{}, errors.New("message bus is required")
	}
	if err := Validate(value); err != nil {
		return Message{}, fmt.Errorf("validate message: %w", err)
	}
	if !safeToken(value.RunID) {
		return Message{}, errors.New("run id is unsafe for message storage")
	}

	runDir, err := b.ensureRunDir(value.RunID)
	if err != nil {
		return Message{}, err
	}
	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return Message{}, fmt.Errorf("acquire message append lock: %w", err)
	}
	defer lock.Release()

	messages, err := loadMessages(runDir)
	if err != nil {
		return Message{}, err
	}
	if value.ReplyTo != "" {
		found := false
		for _, existing := range messages {
			if existing.ID == value.ReplyTo {
				found = true
				if existing.RunID != value.RunID {
					return Message{}, errors.New("reply target belongs to another run")
				}
				break
			}
		}
		if !found {
			return Message{}, errors.New("reply target does not exist in this run")
		}
	}
	var sequence uint64 = 1
	if len(messages) > 0 {
		sequence = messages[len(messages)-1].Sequence + 1
	}
	value.Sequence = sequence

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Message{}, err
	}
	data = append(data, '\n')
	filename := fmt.Sprintf("%020d-%s.json", value.Sequence, value.ID)
	path := filepath.Join(runDir, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Message{}, fmt.Errorf("persist message: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Message{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Message{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return Message{}, err
	}
	return value, nil
}

func (b *Bus) List(runID string, includeExpired bool, now time.Time) ([]Message, error) {
	if b == nil {
		return nil, errors.New("message bus is required")
	}
	if !safeToken(runID) {
		return nil, errors.New("run id is unsafe for message storage")
	}
	runDir := filepath.Join(b.root, runID)
	info, err := os.Lstat(runDir)
	if os.IsNotExist(err) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("run message directory must be a regular directory")
	}
	values, err := loadMessages(runDir)
	if err != nil {
		return nil, err
	}
	if includeExpired {
		return values, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	filtered := make([]Message, 0, len(values))
	for _, value := range values {
		if !value.Expired(now) {
			filtered = append(filtered, value)
		}
	}
	return filtered, nil
}

func (b *Bus) ensureRunDir(runID string) (string, error) {
	runDir := filepath.Join(b.root, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("run message directory must be a regular directory")
	}
	return runDir, nil
}

func loadMessages(runDir string) ([]Message, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	var values []Message
	for _, entry := range entries {
		if entry.Name() == ".append.lock" || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("message entry is not a regular file: %s", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(runDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var value Message
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode message %s: %w", entry.Name(), err)
		}
		if err := Validate(value); err != nil {
			return nil, fmt.Errorf("validate message %s: %w", entry.Name(), err)
		}
		if filenameSequence(entry.Name()) != value.Sequence {
			return nil, fmt.Errorf("message filename sequence mismatch: %s", entry.Name())
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Sequence < values[j].Sequence })
	for i, value := range values {
		expected := uint64(i + 1)
		if value.Sequence != expected {
			return nil, fmt.Errorf("message sequence gap or duplicate: got %d want %d", value.Sequence, expected)
		}
	}
	return values, nil
}

func filenameSequence(name string) uint64 {
	index := strings.IndexByte(name, '-')
	if index <= 0 {
		return 0
	}
	value, _ := strconv.ParseUint(name[:index], 10, 64)
	return value
}


func safeToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
