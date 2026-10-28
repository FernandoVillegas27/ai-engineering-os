package cursor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

func diagnoseFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDiagnoseProposalNativeLauncherInvokesNothing(t *testing.T) {
	runner := &fakeRunner{}
	unit := unitFixture(diagnoseFixtureRoot(t))
	unit.Task = "Validar timeout minimo de 5 segundos"

	report, err := DiagnoseProposal(context.Background(), DiagnoseOptions{Config: RunnerConfig{Runner: runner}, Unit: unit})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("diagnose must not run the agent by default, got %d invocations", len(runner.invocations))
	}
	if len(report.Problems) != 0 || !report.TaskInPrompt || !report.ContractInPrompt || report.SanitizedFiles != 1 {
		t.Fatalf("report=%#v", report)
	}
	if len(report.PromptSHA256) != 64 || report.PromptBytes == 0 {
		t.Fatalf("report=%#v", report)
	}
}

func TestDiagnoseProposalReportsMissingBinary(t *testing.T) {
	report, err := DiagnoseProposal(context.Background(), DiagnoseOptions{
		Config: RunnerConfig{Runner: lookFailRunner{&fakeRunner{}}},
		Unit:   unitFixture(diagnoseFixtureRoot(t)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Problems) == 0 || !strings.Contains(strings.Join(report.Problems, " "), "not found in PATH") {
		t.Fatalf("report=%#v", report)
	}
}

type lookFailRunner struct{ *fakeRunner }

func (lookFailRunner) LookPath(string) (string, error) { return "", errors.New("missing") }

func TestDiagnoseProposalValidatesResponseContract(t *testing.T) {
	unit := unitFixture(diagnoseFixtureRoot(t))
	unit.Task = "Actualizar valor"
	good := `{"type":"result","subtype":"success","is_error":false,"result":"{\"Summary\":\"Actualizar valor\",\"Files\":[{\"Path\":\"service.go\",\"Action\":\"modify\",\"Rationale\":\"Actualizar valor\"}],\"Patch\":\"\",\"Risks\":[],\"Verification\":[\"go test\"]}"}`
	bad := `{"type":"result","subtype":"success","is_error":false,"result":"Which task would you like me to work on?"}`

	for name, tc := range map[string]struct {
		response string
		wantOK   bool
	}{"valid": {good, true}, "task question": {bad, false}} {
		t.Run(name, func(t *testing.T) {
			report, err := DiagnoseProposal(context.Background(), DiagnoseOptions{Config: RunnerConfig{Runner: &fakeRunner{}}, Unit: unit, ResponseText: tc.response})
			if err != nil {
				t.Fatal(err)
			}
			if !report.ResponseChecked || report.ResponseValid != tc.wantOK || (len(report.Problems) == 0) != tc.wantOK {
				t.Fatalf("report=%#v", report)
			}
		})
	}
}

func TestDiagnoseProposalProbesRealProcessTransportOnWindows(t *testing.T) {
	_, agentCmd := windowsInstall(t)
	unit := unitFixture(diagnoseFixtureRoot(t))
	unit.Task = "Validar \"timeout\" & 100% | minimo"

	report, err := DiagnoseProposal(context.Background(), DiagnoseOptions{Config: RunnerConfig{Binary: agentCmd, Runner: commandmodel.OSRunner{}}, Unit: unit})
	if err != nil {
		t.Fatal(err)
	}
	if report.LaunchKind != launchWindowsNode || !report.TransportChecked || !report.TransportIntact || len(report.Problems) != 0 {
		t.Fatalf("report=%#v", report)
	}
}
