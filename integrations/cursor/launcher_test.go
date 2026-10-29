package cursor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const argvProbeMarker = "argv-probe.marker"

// TestMain lets a copy of the test binary act as a fake Cursor runtime that echoes its argv.
func TestMain(m *testing.M) {
	if exe, err := os.Executable(); err == nil {
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(exe), argvProbeMarker)); statErr == nil {
			data, _ := json.Marshal(os.Args[1:])
			if len(os.Args) >= 3 && os.Args[1] == "-e" {
				last := os.Args[len(os.Args)-1]
				sum := sha256.Sum256([]byte(last))
				data, _ = json.Marshal(map[string]any{"count": len(os.Args) - 2, "bytes": len(last), "sha256": hex.EncodeToString(sum[:])})
			}
			os.Stdout.Write(data)
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

const hostilePrompt = "You are generating a proposal.\r\n\r\nTASK: <<<Validate \"timeout\" >= 5s & keep 100% | fine ^ !x>>>\n{\"Files\":[{\"Action\":\"create|modify\"}]}\nTask: ñandú\n"

func installFakeRuntime(t *testing.T, dir, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, argvProbeMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return target
}

func runProbe(t *testing.T, binary, prompt string) []string {
	t.Helper()
	out, err := runAgentWithFormat(context.Background(), RunnerConfig{Binary: binary, Runner: commandmodel.OSRunner{}}, t.TempDir(), "ask", prompt, "stream-json")
	if err != nil {
		t.Fatal(err)
	}
	var argv []string
	if err := json.Unmarshal([]byte(out), &argv); err != nil {
		t.Fatalf("probe output %q: %v", out, err)
	}
	return argv
}

func TestDirectExecutablePreservesMultilinePromptByteForByte(t *testing.T) {
	name := "agent"
	if runtime.GOOS == "windows" {
		name = "agent.exe"
	}
	binary := installFakeRuntime(t, t.TempDir(), name)

	argv := runProbe(t, binary, hostilePrompt)
	if got := argv[len(argv)-1]; got != hostilePrompt {
		t.Fatalf("prompt changed in transit:\n got=%q\nwant=%q", got, hostilePrompt)
	}
}

func windowsInstall(t *testing.T) (installDir string, agentCmd string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows batch launcher behavior")
	}
	installDir = t.TempDir()
	versionDir := filepath.Join(installDir, "versions", "2026.10.19-6bbf2c4")
	installFakeRuntime(t, versionDir, "node.exe")
	if err := os.WriteFile(filepath.Join(versionDir, "index.js"), []byte("// fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Mirrors the official wrapper: batch file forwarding %* to the runtime.
	agentCmd = filepath.Join(installDir, "agent.cmd")
	script := "@echo off\r\n\"%~dp0versions\\2026.10.19-6bbf2c4\\node.exe\" index.js %*\r\n"
	if err := os.WriteFile(agentCmd, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return installDir, agentCmd
}

func TestWindowsBatchLauncherTruncatesMultilinePrompt(t *testing.T) {
	_, agentCmd := windowsInstall(t)

	stdout, _, err := commandmodel.OSRunner{}.Run(context.Background(), commandmodel.Invocation{Command: agentCmd, Args: []string{"-p", hostilePrompt}, Dir: filepath.Dir(agentCmd)})
	if err != nil {
		t.Fatal(err)
	}
	var argv []string
	if err := json.Unmarshal([]byte(stdout), &argv); err != nil {
		t.Fatalf("probe output %q: %v", stdout, err)
	}
	if got := argv[len(argv)-1]; got == hostilePrompt {
		t.Fatal("expected cmd.exe to corrupt the prompt; the root-cause premise no longer holds")
	}
}

func TestWindowsBatchLauncherIsBypassedAndPromptArrivesIntact(t *testing.T) {
	_, agentCmd := windowsInstall(t)

	argv := runProbe(t, agentCmd, hostilePrompt)
	if got := argv[len(argv)-1]; got != hostilePrompt {
		t.Fatalf("prompt changed in transit:\n got=%q\nwant=%q", got, hostilePrompt)
	}
	if !strings.HasSuffix(argv[0], "index.js") {
		t.Fatalf("expected runtime entry point first, argv=%q", argv)
	}
}

func TestPlanAgentLaunchPicksNewestVersionAndKeepsUnixDirect(t *testing.T) {
	if got := planAgentLaunch("linux", "/usr/bin/agent"); got.Kind != launchDirect || got.Command != "/usr/bin/agent" {
		t.Fatalf("plan=%#v", got)
	}

	dir := t.TempDir()
	for _, version := range []string{"2026.9.30-aaaaaaa", "2026.10.19-bbbbbbb", "not-a-version"} {
		versionDir := filepath.Join(dir, "versions", version)
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"node.exe", "index.js"} {
			if err := os.WriteFile(filepath.Join(versionDir, file), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	got := planAgentLaunch("windows", filepath.Join(dir, "agent.cmd"))
	if got.Kind != launchWindowsNode || !strings.Contains(got.Command, "2026.10.19-bbbbbbb") {
		t.Fatalf("plan=%#v", got)
	}
}

func TestUnresolvedBatchLauncherRejectsMultilinePrompt(t *testing.T) {
	if got := planAgentLaunch("windows", filepath.Join(t.TempDir(), "agent.cmd")); got.Kind != launchBatchUnresolved {
		t.Fatalf("plan=%#v", got)
	}
	if !promptNeedsSafeTransport("a\nb") || promptNeedsSafeTransport("plain text") {
		t.Fatal("unexpected promptNeedsSafeTransport result")
	}
}
