package commandmodel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const processTreeHelperEnv = "AI_ENGINEERING_OS_PROCESS_TREE_HELPER"

func TestOSRunnerCancellationTerminatesDescendantProcess(t *testing.T) {
	if os.Getenv(processTreeHelperEnv) == "1" {
		t.Skip("helper subprocess")
	}

	marker := filepath.Join(t.TempDir(), "orphan-marker.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	runner := OSRunner{}
	_, _, err := runner.Run(ctx, Invocation{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestProcessTreeHelper",
			"--",
			"parent",
			marker,
		},
		Env: append(os.Environ(), processTreeHelperEnv+"=1"),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	// The child would write after 900 ms if it survived the cancelled parent.
	time.Sleep(1200 * time.Millisecond)
	if data, readErr := os.ReadFile(marker); readErr == nil {
		t.Fatalf("descendant survived cancellation and wrote marker: %q", data)
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("inspect orphan marker: %v", readErr)
	}
}

func TestProcessTreeHelper(t *testing.T) {
	if os.Getenv(processTreeHelperEnv) != "1" {
		return
	}
	args := helperArgs(os.Args)
	if len(args) != 2 {
		os.Exit(91)
	}
	mode, marker := args[0], args[1]
	switch mode {
	case "parent":
		child := exec.Command(
			os.Args[0],
			"-test.run=TestProcessTreeHelper",
			"--",
			"child",
			marker,
		)
		child.Env = append(os.Environ(), processTreeHelperEnv+"=1")
		if err := child.Start(); err != nil {
			os.Exit(92)
		}
		_ = child.Wait()
		os.Exit(0)
	case "child":
		time.Sleep(900 * time.Millisecond)
		if err := os.WriteFile(marker, []byte("orphan survived"), 0o600); err != nil {
			os.Exit(93)
		}
		os.Exit(0)
	default:
		os.Exit(94)
	}
}

func helperArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}
