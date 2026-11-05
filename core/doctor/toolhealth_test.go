package doctor

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

func TestCheckExternalToolNormalizesFailureDetails(t *testing.T) {
	check := CheckExternalTool("context7", "Context7", toolhealth.Status{
		Provider: "context7",
		Available: true,
		Healthy: false,
		Failure: toolhealth.Failure{
			Kind: toolhealth.FailureTimeout,
			Retryable: true,
			Detail: "operation exceeded 5s",
		},
	})
	if check.Level != LevelWarn || !strings.Contains(check.Hint, "kind=timeout") ||
		!strings.Contains(check.Hint, "retryable=sí") {
		t.Fatalf("check=%#v", check)
	}
}

func TestCheckExternalToolHealthyUsesVersion(t *testing.T) {
	check := CheckExternalTool("engram", "Engram", toolhealth.Status{
		Provider: "engram", Available: true, Healthy: true, Version: "v3",
	})
	if check.Level != LevelOK || check.Message != "v3" {
		t.Fatalf("check=%#v", check)
	}
}
